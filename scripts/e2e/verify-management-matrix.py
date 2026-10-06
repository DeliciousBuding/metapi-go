#!/usr/bin/env python3
"""Opt-in New API management journey against a disposable Metapi process.

This is an eight-check vertical slice, not the lost historical 21-step matrix.
It makes no model requests. It leaves its Metapi site/account in the disposable
database for inspection and only deletes the upstream token it created.
"""

import ipaddress
import json
import os
import re
import secrets
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


CHECKS = (
    "empty_disposable_preflight",
    "site_created",
    "account_login",
    "account_inventory",
    "finite_upstream_token_created",
    "upstream_token_observed",
    "token_revealed_and_local_rename",
    "created_token_deleted_on_both_sides",
)
MAX_REQUESTS = 40
MAX_SECONDS = 300
MAX_BODY = 1024 * 1024


class Failure(Exception):
    def __init__(self, code):
        self.code = code
        super().__init__()


def require(condition, code):
    if not condition:
        raise Failure(code)


def positive_id(value):
    require(type(value) is int and 0 < value <= 2**53 - 1, "invalid_id")
    return value


def object_value(value):
    require(isinstance(value, dict), "invalid_response_shape")
    return value


def origin(value):
    require(isinstance(value, str) and value.isascii() and len(value) <= 2048
            and not any(c in value for c in "\\%?#") and all(ord(c) > 32 for c in value),
            "invalid_origin")
    try:
        parsed = urlsplit(value)
        port = parsed.port
        require(parsed.scheme in ("http", "https") and parsed.hostname
                and parsed.username is None and parsed.password is None
                and parsed.path in ("", "/") and (port is None or 0 < port < 65536),
                "invalid_origin")
    except ValueError:
        raise Failure("invalid_origin") from None
    try:
        local = ipaddress.ip_address(parsed.hostname).is_loopback
    except ValueError:
        local = parsed.hostname == "localhost"
    require(local, "loopback_origin_required")
    return value.rstrip("/")


def config(env):
    required = ("METAPI_BASE_URL", "METAPI_AUTH_TOKEN", "METAPI_EXPECT_COMMIT",
                "NEWAPI_BASE_URL", "NEWAPI_USERNAME", "NEWAPI_PASSWORD")
    require(env.get("MANAGEMENT_DISPOSABLE") == "1"
            and env.get("MANAGEMENT_ALLOW_MUTATIONS") == "1", "disposable_opt_in_required")
    require(all(env.get(key) for key in required), "required_environment_missing")
    require(re.fullmatch(r"[0-9a-f]{40}", env["METAPI_EXPECT_COMMIT"]) is not None,
            "invalid_expected_commit")
    metapi, newapi = origin(env["METAPI_BASE_URL"]), origin(env["NEWAPI_BASE_URL"])
    require(metapi != newapi, "distinct_origins_required")
    return {"metapi": metapi, "newapi": newapi, "admin": env["METAPI_AUTH_TOKEN"],
            "commit": env["METAPI_EXPECT_COMMIT"], "username": env["NEWAPI_USERNAME"],
            "password": env["NEWAPI_PASSWORD"]}


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, newurl):
        raise Failure("redirect_refused")


class HTTP:
    def __init__(self):
        self.opener = build_opener(ProxyHandler({}), NoRedirect())
        self.count = 0
        self.started = time.monotonic()
        self.last = {"method": "", "path": "", "status": 0}

    def call(self, base, method, path, headers=None, payload=None):
        self.count += 1
        self.last = {"method": method, "path": path.split("?", 1)[0], "status": 0}
        require(self.count <= MAX_REQUESTS, "request_budget_exceeded")
        remaining = MAX_SECONDS - (time.monotonic() - self.started)
        require(remaining > 0, "run_deadline_exceeded")
        data = None if payload is None else json.dumps(payload, allow_nan=False).encode("utf-8")
        request = Request(base + path, data=data,
                          headers={"Content-Type": "application/json", **(headers or {})}, method=method)
        try:
            try:
                response = self.opener.open(request, timeout=min(20, remaining))
            except HTTPError as error:
                response = error
            with response:
                self.last["status"] = response.status
                require(not 300 <= response.status < 400, "redirect_refused")
                raw = response.read(MAX_BODY + 1)
                require(len(raw) <= MAX_BODY, "response_too_large")
                return response.status, json.loads(raw)
        except (URLError, OSError, TimeoutError):
            raise Failure("transport_error") from None
        except (UnicodeError, ValueError, RecursionError):
            raise Failure("invalid_json_response") from None


def expected(response, status=200, *, upstream=False):
    code, body = response
    require(code == status, "unexpected_http_status")
    if status == 200:
        require(isinstance(body, (dict, list)), "invalid_response_shape")
        if isinstance(body, dict):
            require(body.get("success") is not False and not body.get("error"), "api_rejected")
        if upstream:
            require(object_value(body).get("success") is True, "upstream_declared_failure")
    return body


class Report:
    def __init__(self, emit):
        self.emit = emit
        self.completed = []
        self.label = "management-" + secrets.token_hex(6)
        self.objects = {}

    def mutation(self, name):
        self.emit({"event": "mutation", "name": name, "runLabel": self.label,
                   "state": "may_exist"})

    def check(self, name, condition):
        require(name in CHECKS and name not in self.completed, "invalid_check_sequence")
        require(condition, "assertion_failed_" + name)
        self.completed.append(name)
        self.emit({"event": "check", "name": name, "status": "PASS"})

    def finish(self, code, http):
        complete = bool(CHECKS) and len(set(CHECKS)) == len(CHECKS) and tuple(self.completed) == CHECKS
        status = "PASS" if complete and code is None else "FAIL"
        self.emit({"event": "summary", "status": status, "code": code or ("ok" if complete else "incomplete_matrix"),
                   "completedChecks": len(self.completed), "expectedChecks": len(CHECKS),
                   "missingChecks": [name for name in CHECKS if name not in self.completed],
                   "directRequests": http.count,
                   "lastOperation": http.last,
                   "runLabel": self.label, "objects": self.objects,
                   "upstreamProcessIdentityVerified": False})
        return 0 if status == "PASS" else 1


def upstream_tokens(http, cfg, auth):
    body = expected(http.call(cfg["newapi"], "GET", "/api/token/?p=1&page_size=100", auth), upstream=True)
    data = body.get("data")
    page = data.get("items") if isinstance(data, dict) else data
    total = data.get("total") if isinstance(data, dict) else body.get("total")
    require(isinstance(page, list) and len(page) < 100 and type(total) is int
            and total == len(page), "incomplete_upstream_inventory")
    return page


def journey(cfg, http, report):
    admin = {"Authorization": "Bearer " + cfg["admin"]}
    metapi, newapi = cfg["metapi"], cfg["newapi"]
    about = expected(http.call(metapi, "GET", "/api/about", admin))
    require(about.get("commit") == cfg["commit"], "server_commit_mismatch")
    expected(http.call(metapi, "GET", "/api/sites"), 401)
    sites = http.call(metapi, "GET", "/api/sites", admin)
    accounts = http.call(metapi, "GET", "/api/accounts", admin)
    require(sites[0] == accounts[0] == 200 and sites[1] == []
            and object_value(accounts[1]).get("accounts") == [], "metapi_not_empty")
    login = expected(http.call(newapi, "POST", "/api/user/login", payload={
        "username": cfg["username"], "password": cfg["password"]}), upstream=True)
    session = object_value(login.get("data"))
    identity = object_value(session.get("user"))
    uid = positive_id(identity.get("id"))
    require(identity.get("role") == 1, "ordinary_user_required")
    sid = object_value(session.get("session")).get("sid")
    jwt = session.get("access_token")
    require(isinstance(sid, str) and sid and isinstance(jwt, str) and jwt, "upstream_login_incomplete")
    upstream_auth = {"Authorization": "Bearer " + jwt, "New-Api-User": str(uid), "X-Auth-Session": sid}
    require(upstream_tokens(http, cfg, upstream_auth) == [], "upstream_user_tokens_not_empty")
    report.check("empty_disposable_preflight", True)

    report.mutation("create_site")
    site = expected(http.call(metapi, "POST", "/api/sites", admin, {
        "name": report.label, "url": newapi, "platform": "new-api"}))
    site_id = positive_id(site.get("id"))
    report.objects["siteId"] = site_id
    report.check("site_created", True)

    report.mutation("login_account")
    logged = expected(http.call(metapi, "POST", "/api/accounts/login", admin, {
        "siteId": site_id, "username": cfg["username"], "password": cfg["password"]}))
    account = object_value(logged.get("account"))
    aid = positive_id(account.get("id"))
    report.objects["accountId"] = aid
    report.check("account_login", logged.get("success") is True and logged.get("reusedAccount") is False
                 and account.get("siteId") == site_id and isinstance(account.get("accessToken"), str)
                 and bool(account["accessToken"]))
    listed = expected(http.call(metapi, "GET", "/api/accounts", admin)).get("accounts")
    report.check("account_inventory", isinstance(listed, list) and len(listed) == 1
                 and listed[0].get("id") == aid and listed[0].get("username") == cfg["username"])

    token_name = report.label + "-finite"
    report.mutation("create_finite_token")
    created = expected(http.call(metapi, "POST", "/api/account-tokens", admin, {
        "accountId": aid, "name": token_name, "group": "default", "unlimitedQuota": False,
        "remainQuota": 100000, "expiredTime": int(time.time()) + 3600}))
    local = expected(http.call(metapi, "GET", "/api/account-tokens?accountId=" + str(aid), admin))
    require(isinstance(local, list), "invalid_local_inventory")
    matches = [row for row in local if isinstance(row, dict) and row.get("name") == token_name]
    require(len(matches) == 1, "created_token_not_in_local_inventory")
    token_id = positive_id(matches[0].get("id"))
    report.objects["tokenId"] = token_id
    report.check("finite_upstream_token_created", created.get("success") is True
                 and created.get("synced") is True and matches[0].get("accountId") == aid)

    upstream = upstream_tokens(http, cfg, upstream_auth)
    report.check("upstream_token_observed", len(upstream) == 1
                 and isinstance(upstream[0], dict) and upstream[0].get("name") == token_name
                 and upstream[0].get("unlimited_quota") is False
                 and upstream[0].get("remain_quota") == 100000)

    revealed = expected(http.call(metapi, "GET", "/api/account-tokens/" + str(token_id) + "/value", admin))
    value = revealed.get("token")
    require(isinstance(value, str) and len(value) > 8 and "****" not in value,
            "token_value_not_revealed")
    report.mutation("rename_local_token")
    renamed = expected(http.call(metapi, "PUT", "/api/account-tokens/" + str(token_id), admin, {
        "name": report.label + "-local"}))
    after = expected(http.call(metapi, "GET", "/api/account-tokens?accountId=" + str(aid), admin))
    report.check("token_revealed_and_local_rename", "token" not in object_value(renamed.get("token"))
                 and isinstance(after, list) and len(after) == 1
                 and after[0].get("name") == report.label + "-local"
                 and upstream_tokens(http, cfg, upstream_auth)[0].get("name") == token_name)

    report.mutation("delete_created_token")
    deleted = expected(http.call(metapi, "DELETE", "/api/account-tokens/" + str(token_id), admin))
    after = expected(http.call(metapi, "GET", "/api/account-tokens?accountId=" + str(aid), admin))
    report.check("created_token_deleted_on_both_sides", deleted.get("success") is True
                 and after == [] and upstream_tokens(http, cfg, upstream_auth) == [])


def run(cfg, http, emit):
    report = Report(emit)
    try:
        require(CHECKS and len(set(CHECKS)) == len(CHECKS), "invalid_matrix_definition")
        journey(cfg, http, report)
        return report.finish(None, http)
    except Failure as error:
        return report.finish(error.code, http)
    except Exception:
        # Exceptions can contain request URLs, response bodies or credentials.
        return report.finish("unexpected_error", http)


def main():
    if sys.argv[1:] == ["--help"]:
        print(__doc__)
        print("Required env: METAPI_BASE_URL METAPI_AUTH_TOKEN METAPI_EXPECT_COMMIT "
              "NEWAPI_BASE_URL NEWAPI_USERNAME NEWAPI_PASSWORD "
              "MANAGEMENT_DISPOSABLE=1 MANAGEMENT_ALLOW_MUTATIONS=1")
        return 0
    if len(sys.argv) != 1:
        print('{"status":"FAIL","code":"unknown_arguments"}')
        return 2
    try:
        cfg = config(os.environ)
    except Failure as error:
        print(json.dumps({"status": "FAIL", "code": error.code}))
        return 2
    return run(cfg, HTTP(), lambda row: print(json.dumps(row, separators=(",", ":")), flush=True))


if __name__ == "__main__":
    raise SystemExit(main())
