#!/usr/bin/env python3
"""Opt-in live NewAPI authentication-boundary acceptance.

Starts fresh, disposable NewAPI and Metapi instances. Plaintext scenarios each
receive a fresh NewAPI instance so the harness cannot consume the upstream's
login rate-limit window. It never changes an existing testbed, rate limit, or
production service. The only retained evidence is a sanitized JSON report;
temporary SQLite databases and process logs are deleted on exit.

Required environment:
  METAPI_BINARY, METAPI_EXPECT_COMMIT, NEWAPI_BINARY, EVIDENCE_DIR

Optional:
  METAPI_EXPECT_SHA256, NEWAPI_EXPECT_SHA256,
  AUTH_PROOF_EXPIRY_WAIT=0|1 (default 1), RUN_PREFIX,
  KEEP_WORK_ON_FAILURE=0|1

The runner uses generated users and passwords. It does not print credentials,
tokens, cookies, proof values, or upstream response bodies. Exit 0 means every
required live check passed; 1 means a check or cleanup failed; 2 is config error.
"""

import base64
import contextlib
import datetime as dt
import hashlib
import http.client
import hmac
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import struct
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

MAX_BODY = 1024 * 1024
MANUAL_PAT = "complete verification in New API and import a durable dashboard PAT manually"


REPORT = None


class CheckFailed(Exception):
    def __init__(self, code):
        super().__init__()
        self.code = code


def require(ok, code):
    if not ok:
        error = CheckFailed(code)
        if REPORT is not None:
            error.lastOperation = dict(REPORT.state.get("lastOperation", {}))
        raise error


def now_iso():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def totp(secret, at=None):
    key = base64.b32decode(secret.upper() + "=" * (-len(secret) % 8))
    counter = int((time.time() if at is None else at) // 30)
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    value = (struct.unpack(">I", digest[offset:offset + 4])[0] & 0x7FFFFFFF) % 1000000
    return f"{value:06d}"


def body_message(body):
    if not isinstance(body, dict):
        return ""
    for key in ("message", "error", "detail"):
        value = body.get(key)
        if isinstance(value, str):
            return value
    return ""


def body_code(body):
    if not isinstance(body, dict):
        return "non_json"
    code = body.get("code")
    return code if isinstance(code, str) and code else "none"


def body_keys(body):
    return sorted(body) if isinstance(body, dict) else []


def count_newapi_login_requests(path):
    try:
        lines = Path(path).read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return 0
    return sum(1 for line in lines if line.rstrip().endswith("POST /api/user/login"))


class API:
    def __init__(self, base, key=None, uid=None, sid=None):
        self.base = base.rstrip("/")
        self.key = key
        self.uid = uid
        self.sid = sid
        self.last = {"method": "", "path": "", "status": 0, "bodyKeys": []}
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def call(self, method, path, body=None, headers=None, timeout=20):
        request_headers = {"Content-Type": "application/json", "Accept": "application/json"}
        if self.key:
            request_headers["Authorization"] = "Bearer " + self.key
        if self.uid:
            request_headers["New-Api-User"] = str(self.uid)
        if self.sid:
            request_headers["X-Auth-Session"] = self.sid
        if headers:
            request_headers.update(headers)
        raw = None
        if body is not None:
            raw = json.dumps(body, separators=(",", ":"), allow_nan=False).encode("utf-8")
        request = urllib.request.Request(self.base + path, data=raw, headers=request_headers, method=method)
        status = 0
        parsed = {}
        try:
            try:
                response = self.opener.open(request, timeout=timeout)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                status = response.status
                payload = response.read(MAX_BODY + 1)
                require(len(payload) <= MAX_BODY, "response_too_large")
                if payload.strip():
                    try:
                        parsed = json.loads(payload)
                    except ValueError:
                        parsed = {"_nonJson": True}
        except CheckFailed:
            raise
        except (urllib.error.URLError, OSError, TimeoutError, http.client.HTTPException):
            parsed = {"_transport": "transport_error"}
        self.last = {"method": method, "path": path.split("?", 1)[0], "status": status, "bodyKeys": body_keys(parsed)}
        if REPORT is not None:
            REPORT.state["lastOperation"] = dict(self.last)
            REPORT.write()
        return status, parsed

    def ok(self, method, path, body=None, headers=None):
        status, parsed = self.call(method, path, body=body, headers=headers)
        require(status == 200, f"http_{status}")
        require(not isinstance(parsed, dict) or parsed.get("success") is not False, "upstream_declared_failure")
        return parsed

    def login(self, username, password):
        status, parsed = self.call("POST", "/api/user/login", {"username": username, "password": password})
        require(status == 200, f"login_http_{status}")
        require(isinstance(parsed, dict) and parsed.get("success") is True, "login_failed")
        data = parsed.get("data") or {}
        require(not data.get("require_verification"), "login_verification_required")
        session = data.get("session") or {}
        user = data.get("user") or {}
        token = data.get("access_token")
        uid = user.get("id")
        sid = session.get("sid")
        require(isinstance(token, str) and token, "login_no_access_token")
        require(isinstance(uid, int) and uid > 0, "login_no_user_id")
        require(isinstance(sid, str) and sid, "login_no_session_id")
        self.key, self.uid, self.sid = token, uid, sid
        return data

    def encrypted_login(self, username, password, work_dir):
        status, key_body = self.call("GET", "/api/user/login/encryption-key")
        require(status == 200 and key_body.get("success") is True, "encryption_key_unavailable")
        key_data = key_body.get("data") or {}
        require(key_data.get("enabled") is True, "encryption_not_advertised")
        kid, public_key = key_data.get("kid"), key_data.get("public_key")
        require(isinstance(kid, str) and kid and isinstance(public_key, str) and public_key, "encryption_key_invalid")
        pem = Path(work_dir) / ("password-public-" + secrets.token_hex(4) + ".pem")
        pem.write_text(public_key, encoding="utf-8")
        try:
            encrypted = subprocess.run(
                ["openssl", "pkeyutl", "-encrypt", "-pubin", "-inkey", str(pem),
                 "-pkeyopt", "rsa_padding_mode:oaep", "-pkeyopt", "rsa_oaep_md:sha256"],
                input=password.encode("utf-8"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=True,
            ).stdout
        finally:
            pem.unlink(missing_ok=True)
        status, parsed = self.call("POST", "/api/user/login", {
            "username": username,
            "password_encrypted": base64.b64encode(encrypted).decode("ascii"),
            "encryption_key_id": kid,
        })
        require(status == 200 and isinstance(parsed, dict) and parsed.get("success") is True, "encrypted_login_failed")
        data = parsed.get("data") or {}
        session = data.get("session") or {}
        user = data.get("user") or {}
        token, uid, sid = data.get("access_token"), user.get("id"), session.get("sid")
        require(isinstance(token, str) and token and isinstance(uid, int) and uid > 0 and isinstance(sid, str) and sid,
                "encrypted_login_incomplete")
        self.key, self.uid, self.sid = token, uid, sid
        return data

    def token_status(self):
        status, parsed = self.call("GET", "/api/user/token/status")
        require(status == 200 and parsed.get("success") is True, "token_status_failed")
        return parsed.get("data") or {}

    def proof(self, scope, password, code=None):
        status, parsed = self.call("GET", "/api/verify/methods?scope=" + urllib.parse.quote(scope, safe=""))
        require(status == 200 and parsed.get("success") is True, "verify_methods_failed")
        data = parsed.get("data") or {}
        require(data.get("scope") == scope, "verify_scope_mismatch")
        methods = data.get("methods") or []
        chosen = "2fa" if code is not None else "password"
        require(any(isinstance(x, dict) and x.get("method") == chosen and x.get("available") is True for x in methods),
                "verification_method_unavailable")
        body = {"method": chosen, "scope": scope}
        if code is not None:
            body["code"] = code
        else:
            body["password"] = password
        status, parsed = self.call("POST", "/api/verify", body)
        require(status == 200 and parsed.get("success") is True, "verify_failed")
        data = parsed.get("data") or {}
        proof = data.get("proof_token")
        expires = data.get("expires_at")
        require(isinstance(proof, str) and proof and isinstance(expires, (int, float)) and expires > time.time(),
                "proof_invalid")
        return proof, int(expires)


class Report:
    def __init__(self, path, prefix):
        global REPORT
        REPORT = self
        self.path = Path(path)
        self.state = {
            "status": "running",
            "runPrefix": prefix,
            "startedAt": now_iso(),
            "checks": [],
            "cleanup": [],
            "lastOperation": {},
        }
        self.write()

    def write(self):
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self.state, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        tmp.replace(self.path)

    def check(self, name, ok, code="ok", **facts):
        row = {"name": name, "status": "pass" if ok else "fail", "code": code, **facts}
        self.state["checks"].append(row)
        self.write()
        return row

    def cleanup(self, name, ok, **facts):
        self.state["cleanup"].append({"name": name, "ok": bool(ok), **facts})
        self.write()


class Process:
    def __init__(self, name, command, cwd, env, log_path):
        self.name = name
        self.log_path = Path(log_path)
        self.log = self.log_path.open("wb")
        self.process = subprocess.Popen(command, cwd=cwd, env=env, stdout=self.log, stderr=subprocess.STDOUT,
                                        start_new_session=True)
        self.stopped = False

    def stop(self):
        if self.stopped:
            return
        try:
            if self.process.poll() is None:
                with contextlib.suppress(ProcessLookupError):
                    os.killpg(self.process.pid, signal.SIGTERM)
                try:
                    self.process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(self.process.pid, signal.SIGKILL)
                    self.process.wait(timeout=10)
        finally:
            self.log.close()
            self.stopped = True


def minimal_env(extra):
    env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG", "TZ") if k in os.environ}
    env.update({"NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost", "GOMAXPROCS": "1"})
    env.update(extra)
    return env


def wait_for(api, path, predicate, timeout=45):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        status, parsed = api.call("GET", path)
        last = {"status": status, "bodyKeys": body_keys(parsed)}
        if status == 200 and predicate(parsed):
            return parsed
        time.sleep(0.25)
    raise CheckFailed("process_not_ready")


def initialize_newapi(base):
    username = "root"
    password = secrets.token_urlsafe(12)
    status, body = API(base).call("POST", "/api/setup", {
        "username": username,
        "password": password,
        "confirmPassword": password,
        "SelfUseModeEnabled": True,
        "DemoSiteEnabled": False,
    })
    require(status == 200 and isinstance(body, dict) and body.get("success") is True, "newapi_setup_failed")
    return username, password

def start_newapi(binary, data_dir, encrypted):
    data_dir = Path(data_dir)
    (data_dir / "logs").mkdir(parents=True, exist_ok=True)
    port = free_port()
    base = f"http://127.0.0.1:{port}"
    env = minimal_env({
        "PORT": str(port),
        "SQLITE_PATH": str(data_dir / "one-api.db"),
        "SESSION_SECRET": secrets.token_urlsafe(32),
        "CRYPTO_SECRET": secrets.token_urlsafe(32),
        "PASSWORD_LOGIN_ENCRYPTION_ENABLED": "true" if encrypted else "false",
        "GIN_MODE": "release",
        "BATCH_UPDATE_ENABLED": "false",
    })
    proc = Process("newapi-" + ("encrypted" if encrypted else "plain"), [binary, "--log-dir", str(data_dir / "logs")],
                   data_dir, env, data_dir / "process.log")
    try:
        wait_for(API(base), "/api/status", lambda x: isinstance(x, dict) and x.get("success") is True)
    except Exception:
        proc.stop()
        raise
    return base, proc


def start_upstream(binary, data_dir, encrypted, work_dir):
    base, proc = start_newapi(binary, data_dir, encrypted)
    try:
        username, password = initialize_newapi(base)
        root = API(base)
        if encrypted:
            root.encrypted_login(username, password, work_dir)
        else:
            root.login(username, password)
        require(root.uid == 1, "upstream_root_identity_mismatch")
    except Exception:
        proc.stop()
        raise
    return base, root, proc


def start_metapi(binary, data_dir, expected_commit):
    data_dir = Path(data_dir)
    data_dir.mkdir(parents=True, exist_ok=True)
    port = free_port()
    base = f"http://127.0.0.1:{port}"
    admin_token = secrets.token_urlsafe(32)
    env = minimal_env({
        "HOST": "127.0.0.1",
        "PORT": str(port),
        "BASE_URL": base,
        "DATA_DIR": str(data_dir),
        "AUTH_TOKEN": admin_token,
        "PROXY_TOKEN": secrets.token_urlsafe(32),
    })
    proc = Process("metapi", [binary], data_dir, env, data_dir / "process.log")
    api = API(base, key=admin_token)
    try:
        wait_for(api, "/ready", lambda x: isinstance(x, dict) and x.get("status") == "ok")
        about = api.ok("GET", "/api/about")
        require(about.get("commit") == expected_commit, "metapi_commit_mismatch")
    except Exception:
        proc.stop()
        raise
    return base, admin_token, proc


def create_upstream_user(root, prefix, role=1, encrypted=False, work_dir=None):
    username = "u" + secrets.token_hex(6)
    password = secrets.token_urlsafe(12)
    root.ok("POST", "/api/user/", {"username": username, "password": password, "display_name": "Metapi boundary", "role": role})
    user = API(root.base)
    if encrypted:
        require(work_dir is not None, "encrypted_user_missing_work_dir")
        data = user.encrypted_login(username, password, work_dir)
    else:
        data = user.login(username, password)
    require((data.get("user") or {}).get("role") == role, "ordinary_user_role_mismatch")
    return username, password, user


def delete_upstream_user(root, uid):
    if uid and uid > 1:
        root.call("DELETE", f"/api/user/{uid}")


def metapi_sites(metapi):
    body = metapi.ok("GET", "/api/sites")
    data = body.get("sites", body.get("data", []))
    return data if isinstance(data, list) else []


def metapi_accounts(metapi):
    body = metapi.ok("GET", "/api/accounts")
    data = body.get("accounts", body.get("data", []))
    return data if isinstance(data, list) else []


def create_metapi_site(metapi, name, upstream):
    body = metapi.ok("POST", "/api/sites", {"name": name, "url": upstream, "platform": "new-api"})
    return body.get("id") or (body.get("site") or {}).get("id")


def run_scenario_mfa(ctx):
    root = ctx["plain_root"]
    metapi = ctx["metapi"]
    username, password, user = create_upstream_user(root, ctx["prefix"] + "mfa")
    site_id = None
    try:
        # Enroll 2FA through NewAPI's own public API. The password proof is
        # consumed before the session rotates; use the rotated session after.
        setup_proof, _ = user.proof("2fa.setup", password)
        status, setup = user.call("POST", "/api/user/2fa/setup", headers={"X-Security-Proof": setup_proof})
        require(status == 200 and setup.get("success") is True, "twofa_setup_failed")
        setup_data = setup.get("data") or {}
        secret = setup_data.get("secret")
        flow_token = setup_data.get("flow_token")
        require(isinstance(secret, str) and secret and isinstance(flow_token, str) and flow_token, "twofa_setup_incomplete")
        status, enabled = user.call("POST", "/api/user/2fa/enable", {"code": totp(secret), "flow_token": flow_token})
        require(status == 200 and enabled.get("success") is True, "twofa_enable_failed")
        data = enabled.get("data") or {}
        user.key = data.get("access_token") or user.key
        user.sid = (data.get("session") or {}).get("sid") or user.sid
        status, status_body = user.call("GET", "/api/user/2fa/status")
        require(status == 200 and (status_body.get("data") or {}).get("enabled") is True, "twofa_status_not_enabled")

        site_id = create_metapi_site(metapi, ctx["prefix"] + "mfa-site", ctx["plain_base"])
        require(isinstance(site_id, int) and site_id > 0, "metapi_site_create_failed")
        before = len(metapi_accounts(metapi))
        status, body = metapi.call("POST", "/api/accounts/login", {
            "siteId": site_id, "username": username, "password": password,
        })
        message = body_message(body).lower()
        ctx["report"].check(
            "mfa_product_login_fails_closed", status in (400, 401) and "mfa" in message and "pat" in message,
            code="ok" if status in (400, 401) else f"http_{status}",
            httpStatus=status,
            messageNamesMFA="mfa" in message,
            messageNamesManualPAT="pat" in message,
            accountCountUnchanged=(len(metapi_accounts(metapi)) == before),
        )
        token_status = user.token_status()
        ctx["report"].check("mfa_no_durable_pat_created", token_status.get("exists") is False,
                            code="ok" if token_status.get("exists") is False else "pat_exists",
                            tokenExists=bool(token_status.get("exists")))
    finally:
        if site_id:
            metapi.call("DELETE", f"/api/sites/{site_id}")
        delete_upstream_user(root, user.uid)


def scenario_proof_matrix(ctx, wait_expiry):
    root = ctx["plain_root"]
    user_api = API(ctx["plain_base"])
    username, password, user = create_upstream_user(root, ctx["prefix"] + "proof")
    try:
        initial = user.token_status()
        require(initial.get("exists") is False, "proof_initial_token_exists")

        status, body = user.call("GET", "/api/user/token")
        ctx["report"].check("proof_missing_rejected", status == 403 and body_code(body) == "SECURITY_PROOF_REQUIRED",
                            code=body_code(body), httpStatus=status,
                            tokenUnchanged=(user.token_status().get("exists") is False))

        revoke_proof, _ = user.proof("access_token.revoke", password)
        status, body = user.call("GET", "/api/user/token", headers={"X-Security-Proof": revoke_proof})
        ctx["report"].check("proof_wrong_scope_rejected", status == 403 and body_code(body) == "SECURITY_PROOF_SCOPE_MISMATCH",
                            code=body_code(body), httpStatus=status,
                            tokenUnchanged=(user.token_status().get("exists") is False))

        status, body = user.call("POST", "/api/verify", {
            "method": "password", "scope": "access_token.generate", "password": password + "-wrong",
        })
        ctx["report"].check("proof_wrong_password_rejected",
                            status == 200 and isinstance(body, dict) and body.get("success") is False and body_code(body) == "SECURITY_VERIFICATION_FAILED",
                            code=body_code(body), httpStatus=status,
                            tokenUnchanged=(user.token_status().get("exists") is False))

        proof, _ = user.proof("access_token.generate", password)
        status, generated = user.call("GET", "/api/user/token", headers={"X-Security-Proof": proof})
        token = generated.get("data") if isinstance(generated, dict) else None
        require(status == 200 and isinstance(generated, dict) and generated.get("success") is True and isinstance(token, str) and token, "valid_proof_failed")
        first_ref = user.token_status().get("token_ref")
        require(isinstance(first_ref, str) and first_ref, "token_ref_missing_after_generate")

        status, body = user.call("GET", "/api/user/token", headers={"X-Security-Proof": proof})
        second_ref = user.token_status().get("token_ref")
        ctx["report"].check("proof_reuse_rejected", status == 403 and body_code(body) in ("SECURITY_PROOF_INVALID", "SECURITY_PROOF_CONSUMED"),
                            code=body_code(body), httpStatus=status,
                            tokenRefUnchanged=(second_ref == first_ref))

        if wait_expiry:
            proof, expires = user.proof("access_token.generate", password)
            wait = max(1, expires - int(time.time()) + 2)
            ctx["report"].check("proof_expiry_clock_observed", wait <= 301, code="ok" if wait <= 301 else "ttl_out_of_contract",
                                waitSeconds=wait)
            time.sleep(wait)
            status, body = user.call("GET", "/api/user/token", headers={"X-Security-Proof": proof})
            final_ref = user.token_status().get("token_ref")
            ctx["report"].check("proof_expired_rejected", status == 403 and body_code(body) in ("SECURITY_PROOF_EXPIRED", "SECURITY_PROOF_INVALID"),
                                code=body_code(body), httpStatus=status,
                                tokenRefUnchanged=(final_ref == first_ref))
        else:
            ctx["report"].check("proof_expiry_live_deferred", True, code="explicit_skip", required=False)
    finally:
        delete_upstream_user(root, user.uid)


def scenario_auto_relogin(ctx):
    root = ctx["plain_root"]
    metapi = ctx["metapi"]
    username, password, user = create_upstream_user(root, ctx["prefix"] + "relogin")
    account_id = None
    site_id = None
    try:
        site_id = create_metapi_site(metapi, ctx["prefix"] + "relogin-site", ctx["plain_base"])
        require(isinstance(site_id, int) and site_id > 0, "relogin_site_create_failed")
        logged = metapi.ok("POST", "/api/accounts/login", {"siteId": site_id, "username": username, "password": password})
        account_id = (logged.get("account") or {}).get("id")
        require(isinstance(account_id, int) and account_id > 0, "relogin_account_create_failed")
        metapi.ok("PUT", f"/api/accounts/{account_id}", {"platformUserId": user.uid})
        ctx["report"].check("relogin_platform_user_id_stored", True, code="ok", platformUserIdKnown=True)
        before = user.token_status()
        require(before.get("exists") is True and isinstance(before.get("token_ref"), str), "relogin_initial_pat_missing")

        revoke_proof, _ = user.proof("access_token.revoke", password)
        status, body = user.call("DELETE", "/api/user/token", headers={"X-Security-Proof": revoke_proof})
        require(status == 200 and body.get("success") is True, "relogin_pat_revoke_failed")
        require(user.token_status().get("exists") is False, "relogin_pat_still_exists")

        login_before = count_newapi_login_requests(ctx["newapi_log"])
        status, _ = metapi.call("POST", f"/api/accounts/{account_id}/balance")
        require(status == 200, f"balance_refresh_http_{status}")
        after = user.token_status()
        require(after.get("exists") is True and after.get("token_ref") != before.get("token_ref"), "relogin_pat_not_replaced")

        deadline = time.monotonic() + 5
        login_count = 0
        while time.monotonic() < deadline:
            login_count = count_newapi_login_requests(ctx["newapi_log"]) - login_before
            if login_count >= 1:
                break
            time.sleep(0.1)
        ctx["report"].check("auto_relogin_count_exactly_one", login_count == 1, code="ok" if login_count == 1 else "unexpected_login_count",
                            productInitiatedLoginCount=login_count,
                            loginEvidenceSource="process_log",
                            tokenReplaced=True)
    finally:
        if account_id:
            metapi.call("DELETE", f"/api/accounts/{account_id}")
        if site_id:
            metapi.call("DELETE", f"/api/sites/{site_id}")
        delete_upstream_user(root, user.uid)


def scenario_encrypted(ctx):
    root = ctx["encrypted_root"]
    metapi = ctx["metapi"]
    username, password, user = create_upstream_user(root, ctx["prefix"] + "enc", encrypted=True, work_dir=ctx["work"])
    site_id = None
    try:
        # Prove the isolated upstream really supports the encrypted ceremony.
        direct = API(ctx["encrypted_base"])
        direct.encrypted_login(username, password, ctx["work"])
        ctx["report"].check("encrypted_upstream_variant_live", True, code="ok", directEncryptedLogin=True)

        site_id = create_metapi_site(metapi, ctx["prefix"] + "enc-site", ctx["encrypted_base"])
        require(isinstance(site_id, int) and site_id > 0, "encrypted_site_create_failed")
        before = len(metapi_accounts(metapi))
        status, body = metapi.call("POST", "/api/accounts/login", {
            "siteId": site_id, "username": username, "password": password,
        })
        message = body_message(body).lower()
        ctx["report"].check("encrypted_product_login_explicit_failure",
                            status in (400, 401) and "encrypted-password" in message and "pat" in message,
                            code="ok" if status in (400, 401) else f"http_{status}",
                            httpStatus=status,
                            messageNamesEncryptedVariant="encrypted-password" in message,
                            messageNamesManualPAT="pat" in message,
                            accountCountUnchanged=(len(metapi_accounts(metapi)) == before))
        token_status = user.token_status()
        ctx["report"].check("encrypted_no_durable_pat_created", token_status.get("exists") is False,
                            code="ok" if token_status.get("exists") is False else "pat_exists",
                            tokenExists=bool(token_status.get("exists")))
    finally:
        if site_id:
            metapi.call("DELETE", f"/api/sites/{site_id}")
        delete_upstream_user(root, user.uid)


def run_upstream_scenario(cfg, work_root, processes, metapi, report, name, encrypted, scenario):
    base, root, proc = start_upstream(
        cfg["newapi_binary"], work_root / ("newapi-" + name), encrypted, work_root
    )
    processes.append(proc)
    try:
        ctx = {
            "report": report, "work": work_root, "prefix": cfg["prefix"],
            "plain_base": base, "encrypted_base": base,
            "plain_root": root, "encrypted_root": root,
            "newapi_log": str(proc.log_path),
            "metapi": metapi,
        }
        scenario(ctx)
    finally:
        proc.stop()


def load_config():
    required = ("METAPI_BINARY", "METAPI_EXPECT_COMMIT", "NEWAPI_BINARY", "EVIDENCE_DIR")
    missing = [key for key in required if not os.environ.get(key)]
    if missing:
        raise CheckFailed("missing_environment")
    evidence = Path(os.environ["EVIDENCE_DIR"]).expanduser().resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    return {
        "metapi_binary": os.environ["METAPI_BINARY"],
        "metapi_commit": os.environ["METAPI_EXPECT_COMMIT"],
        "metapi_sha": os.environ.get("METAPI_EXPECT_SHA256", ""),
        "newapi_binary": os.environ["NEWAPI_BINARY"],
        "newapi_sha": os.environ.get("NEWAPI_EXPECT_SHA256", ""),
        "evidence": evidence,
        "wait_expiry": os.environ.get("AUTH_PROOF_EXPIRY_WAIT", "1") != "0",
        "prefix": os.environ.get("RUN_PREFIX", "auth-" + secrets.token_hex(4) + "-"),
    }


def main():
    try:
        cfg = load_config()
    except CheckFailed:
        print("missing required environment: METAPI_BINARY METAPI_EXPECT_COMMIT NEWAPI_BINARY EVIDENCE_DIR", file=sys.stderr)
        return 2

    report_path = cfg["evidence"] / "auth-boundaries-status.json"
    report = Report(report_path, cfg["prefix"])
    work_root = Path(tempfile.mkdtemp(prefix="auth-boundaries-", dir=cfg["evidence"]))
    processes = []
    exit_code = 1
    try:
        if cfg["metapi_sha"]:
            require(sha256_file(cfg["metapi_binary"]) == cfg["metapi_sha"], "metapi_binary_sha_mismatch")
        if cfg["newapi_sha"]:
            require(sha256_file(cfg["newapi_binary"]) == cfg["newapi_sha"], "newapi_binary_sha_mismatch")

        metapi_base, metapi_token, metapi_proc = start_metapi(cfg["metapi_binary"], work_root / "metapi", cfg["metapi_commit"])
        processes.append(metapi_proc)
        metapi = API(metapi_base, key=metapi_token)

        run_upstream_scenario(cfg, work_root, processes, metapi, report, "mfa", False, run_scenario_mfa)
        run_upstream_scenario(cfg, work_root, processes, metapi, report, "proof", False,
                              lambda ctx: scenario_proof_matrix(ctx, cfg["wait_expiry"]))
        run_upstream_scenario(cfg, work_root, processes, metapi, report, "relogin", False, scenario_auto_relogin)
        run_upstream_scenario(cfg, work_root, processes, metapi, report, "encrypted", True, scenario_encrypted)
        report.state["freshUpstreamPerScenario"] = True

        report.state["status"] = "pass" if all(x["status"] == "pass" for x in report.state["checks"]) else "fail"
        report.state["finishedAt"] = now_iso()
        report.state["binary"] = {
            "metapiSha256": sha256_file(cfg["metapi_binary"]),
            "newapiSha256": sha256_file(cfg["newapi_binary"]),
            "metapiCommit": cfg["metapi_commit"],
        }
        exit_code = 0 if report.state["status"] == "pass" else 1
    except (CheckFailed, subprocess.SubprocessError, OSError) as error:
        report.state["status"] = "fail"
        report.state["error"] = {"type": type(error).__name__, "code": getattr(error, "code", "process_error")}
        report.state["lastOperation"] = getattr(error, "lastOperation", report.state.get("lastOperation", {}))
        report.state["finishedAt"] = now_iso()
        print("auth boundaries: FAIL (" + report.state["error"]["code"] + ")", file=sys.stderr)
        exit_code = 1
    finally:
        cleanup_ok = True
        for proc in reversed(processes):
            try:
                proc.stop()
            except Exception:
                cleanup_ok = False
        if processes and not all(proc.process.poll() is not None for proc in processes):
            cleanup_ok = False
        report.cleanup("disposable process tree stopped", cleanup_ok)
        if os.environ.get("KEEP_WORK_ON_FAILURE") == "1" and report.state.get("status") == "fail":
            report.state["workDir"] = str(work_root)
        else:
            shutil.rmtree(work_root, ignore_errors=True)
            if work_root.exists():
                cleanup_ok = False
        report.state["cleanupComplete"] = cleanup_ok
        if not cleanup_ok:
            report.state["status"] = "fail"
            exit_code = 1
        report.write()
    return exit_code

if __name__ == "__main__":
    raise SystemExit(main())
