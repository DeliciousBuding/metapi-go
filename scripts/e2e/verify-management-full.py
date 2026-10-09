#!/usr/bin/env python3
"""Full 21-check New API management acceptance on disposable loopback instances.

No real model requests. Uses a fresh ordinary user, preserves natural JWT expiry
and upstream rate windows, and verifies cleanup. Secrets belong outside evidence.
"""
from pathlib import Path
import ipaddress, re, base64, datetime, hashlib, http.cookiejar, json, os, secrets, sys, time, urllib.error, urllib.parse, urllib.request
MANAGEMENT_WINDOW_SECONDS = 1201
CHECKS = (
  "create isolated ordinary NewAPI user",
  "password login persists and repeat login reuses account",
  "Metapi creates real finite upstream tokens",
  "metadata update does not return stored plaintext token",
  "metadata rename stays local as documented",
  "disabled local default moves to a usable token",
  "token deletion removes both local and upstream records",
  "PAT import discovers models with existing relay credentials",
  "fresh checkin grants exactly the configured upstream reward",
  "already checked upstream is success without another reward",
  "upstream disabled checkin is explicit skip",
  "ordinary login JWT expires naturally at the real upstream",
  "persisted PAT still manages the account after JWT expiry",
  "revoked PAT fails without password self-healing",
  "session rebind keeps credentials redacted",
  "credential replacement restores balance and checkin management",
  "disabled account skips management probes",
  "API-key connection is honestly proxy-only",
  "API-key connection cannot manage upstream tokens",
  "wrong password does not change account identities",
  "invalid management token persists no additional account"
)
REQUIRED_ENV = ('BASE_URL', 'AUTH_TOKEN', 'UPSTREAM_URL', 'NEWAPI_ROOT_USER', 'NEWAPI_ROOT_PASSWORD', 'EXPECT_SERVER_COMMIT', 'EVIDENCE_DIR', 'SECRET_DIR')

class ConfigError(RuntimeError):
    pass

def load_config(environ=None):
    env = os.environ if environ is None else environ
    if env.get('MANAGEMENT_DISPOSABLE') != '1' or env.get('MANAGEMENT_ALLOW_MUTATIONS') != '1':
        raise ConfigError('disposable mutation opt-in required')
    for field in ('BASE_URL', 'UPSTREAM_URL'):
        parsed = urllib.parse.urlsplit(env.get(field, ''))
        try:
            local = parsed.hostname == 'localhost' or ipaddress.ip_address(parsed.hostname).is_loopback
        except ValueError:
            local = False
        if not local or parsed.scheme not in ('http', 'https') or parsed.username or parsed.password or parsed.path not in ('', '/') or parsed.query or parsed.fragment:
            raise ConfigError('loopback origins required')
    if env.get('BASE_URL') == env.get('UPSTREAM_URL'):
        raise ConfigError('distinct origins required')
    if re.fullmatch(r'[0-9a-f]{40}', env.get('EXPECT_SERVER_COMMIT', '')) is None:
        raise ConfigError('full expected commit required')
    missing = [name for name in REQUIRED_ENV if not str(env.get(name, '')).strip()]
    if missing:
        raise ConfigError('missing required environment variables: ' + ', '.join(missing))
    evidence = Path(env['EVIDENCE_DIR']).resolve()
    secret = Path(env['SECRET_DIR']).resolve()
    if secret.is_relative_to(evidence) or evidence.is_relative_to(secret):
        raise ConfigError('evidence and secret directories must be disjoint')
    return {'base_url': str(env['BASE_URL']).strip().rstrip('/'), 'auth_token': str(env['AUTH_TOKEN']), 'upstream_url': str(env['UPSTREAM_URL']).strip().rstrip('/'), 'newapi_root_user': str(env['NEWAPI_ROOT_USER']), 'newapi_root_password': str(env['NEWAPI_ROOT_PASSWORD']), 'expect_server_commit': str(env['EXPECT_SERVER_COMMIT']).strip(), 'evidence_dir': Path(env['EVIDENCE_DIR']).expanduser().resolve(), 'secret_dir': Path(env['SECRET_DIR']).expanduser().resolve()}

class FailureAccumulator:

    def __init__(self): self.steps = []; self.cleanup = []
    def record_step(self, name, ok, **facts): self.steps.append({'name': name, 'status': 'pass' if ok else 'fail', **facts})
    def record_cleanup(self, name, ok, **facts): self.cleanup.append({'name': name, 'ok': bool(ok), **facts})

    def exit_code(self):
        passed = tuple(x['name'] for x in self.steps) == CHECKS and all((x['status'] == 'pass' for x in self.steps)) and all((x['ok'] for x in self.cleanup))
        return 0 if passed else 1
cfg = state = private = None
failures = FailureAccumulator()
owned_accounts = set()
root = user = m = None
uid = None
created_site = None
user_created = False
original_options = {}
main_before = None
secret_file = None

def write_secret():
    fd = os.open(secret_file, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 384)
    with os.fdopen(fd, 'w', encoding='utf-8') as handle: json.dump(private, handle, indent=2); handle.write('\n')
    secret_file.chmod(384)

def save():
    state['steps'] = failures.steps; state['cleanup'] = failures.cleanup
    path = cfg['evidence_dir'] / 'management-status.tmp'
    path.write_text(json.dumps(state, indent=2) + '\n', encoding='utf-8'); path.replace(cfg['evidence_dir'] / 'management-status.json'); write_secret()

def observe(name, condition, **facts):
    failures.record_step(name, condition, **facts); save()

def items(body):
    data = body.get('data', body) if isinstance(body, dict) else body
    return data.get('items', data) if isinstance(data, dict) else data

class API:

    def __init__(self, base, key=None):
        self.base = base
        self.key = key
        self.uid = None
        self.role = None
        self.status = None
        self.sid = None
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def call(self, method, path, body=None, allow_failure=False, extra_headers=None):
        state['lastOperation'] = {'method': method, 'path': path.split('?', 1)[0]}
        save()
        headers = {'Content-Type': 'application/json'}
        if self.key:
            headers['Authorization'] = 'Bearer ' + self.key
        if self.uid:
            headers['New-Api-User'] = str(self.uid)
        if self.sid:
            headers['X-Auth-Session'] = self.sid
        if extra_headers:
            headers.update(extra_headers)
        req = urllib.request.Request(self.base + path, data=json.dumps(body).encode() if body is not None else b'{}' if method in ('POST', 'PUT') else None, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=45) as response:
                self.status = response.status
                raw = response.read()
                data = json.loads(raw) if raw.strip() else {'_nonJson': True}
        except urllib.error.HTTPError as error:
            self.status = error.code
            raw = error.read()
            try:
                data = json.loads(raw)
            except ValueError:
                data = {'_nonJson': True}
        if not allow_failure:
            if not 200 <= self.status < 300:
                state['lastApiFailure'] = {'httpStatus': self.status, 'bodyKeys': sorted(data) if isinstance(data, dict) else [], 'mentionsRateLimit': any((x in str(data).lower() for x in ('429', 'rate limit', 'too many requests')))}
            assert 200 <= self.status < 300, 'unexpected_http_' + str(self.status)
            assert not isinstance(data, dict) or not data.get('_nonJson'), 'unexpected_non_json_response'
            assert not isinstance(data, dict) or data.get('success') is not False, 'upstream_declared_failure'
        return data

    def login(self, username, password):
        # A planned login must not carry the previous JWT, SID, or cookies.
        fresh = API(self.base)
        data = fresh.call('POST', '/api/user/login', {'username': username, 'password': password})
        session = data['data']
        identity = session['user']
        assert self.uid is None or identity['id'] == self.uid, 'login_identity_mismatch'
        assert self.role is None or identity['role'] == self.role, 'login_role_mismatch'
        key, sid = session['access_token'], session['session']['sid']
        assert key and sid, 'login_session_missing'
        self.key, self.uid, self.role, self.sid = key, identity['id'], identity['role'], sid
        self.opener, self.status = fresh.opener, fresh.status
        return data

    def proof(self, scope, password, context):
        methods = self.call('GET', '/api/verify/methods?scope=' + scope)['data']
        assert methods['scope'] == scope and methods.get('password_encryption_enabled') is False
        assert any(x['method'] == 'password' and x['available'] for x in methods['methods'])
        proof = self.call('POST', '/api/verify', {'method': 'password', 'scope': scope, 'password': password, 'context': context})['data']
        assert proof['scope'] == scope and proof['method'] == 'password' and proof['expires_at'] > time.time()
        return proof['proof_token']

    def mint_pat(self, password):
        # Capture the owned old PAT by its exact upstream fingerprint, not a mask/name.
        old_ref = hashlib.sha256(private['managementToken'].encode()).hexdigest()
        before = self.call('GET', '/api/user/access_tokens')['data']['items']
        matches = [x for x in before if x.get('token_ref') == old_ref]
        assert len(matches) == 1 and type(matches[0].get('id')) is int, 'owned_pat_not_found'
        scopes = ['api_key:read', 'api_key:reveal', 'api_key:write', 'profile:read', 'wallet:read', 'wallet:write']
        context = {'scopes': scopes, 'expires_at': 0}
        request = {'name': 'Metapi acceptance replacement', **context}
        answer = self.call('POST', '/api/user/access_tokens', request, allow_failure=True)
        assert self.status == 403 and answer.get('code') == 'SECURITY_PROOF_REQUIRED', 'missing_proof_not_rejected'
        proof = self.proof('access_token.generate', password, context)
        token = self.call('POST', '/api/user/access_tokens', request, extra_headers={'X-Security-Proof': proof})['data']['token']
        assert token.startswith('nap_') and token not in (self.key, proof, private['managementToken'])
        revoke = self.proof('access_token.revoke', password, {'token_id': matches[0]['id']})
        self.call('DELETE', '/api/user/access_tokens/' + str(matches[0]['id']), extra_headers={'X-Security-Proof': revoke})
        after = self.call('GET', '/api/user/access_tokens')['data']['items']
        assert all(x.get('token_ref') != old_ref for x in after), 'revoked_pat_still_listed'
        return token

def account_summary():
    return sorted(((x['id'], x.get('username'), x.get('status')) for x in m.call('GET', '/api/accounts')['accounts']))

def tokens(aid):
    return m.call('GET', '/api/account-tokens?accountId=' + str(aid))

def own_upstream_tokens():
    response = user.call('GET', '/api/token/?p=1&page_size=100')
    listed = items(response)
    data = response.get('data', response) if isinstance(response, dict) else response
    total = data.get('total') if isinstance(data, dict) else None
    # This two-token runner is not a pagination fixture. Never prove absence
    # or successful cleanup from a full page or a declared partial list.
    assert isinstance(listed, list) and len(listed) < 100 and (type(total) is int and total == len(listed)), 'incomplete_upstream_token_list'
    return listed

def session_jwt_expiry(token):
    try:
        parts = token.split('.')
        assert len(parts) == 3
        payload = json.loads(base64.urlsafe_b64decode(parts[1] + '=' * (-len(parts[1]) % 4)))
        expiry = payload['exp']
        assert isinstance(expiry, int) and not isinstance(expiry, bool) and expiry > 0
        return expiry
    except Exception:
        raise AssertionError('ordinary_login_did_not_return_an_expiring_jwt') from None

def probe_expired_session(snapshot):
    # A fresh HTTP client carries only the original JWT/SID, with no refreshed
    # cookie jar that could hide expiration. This expected negative is read-only.
    probe = API(cfg['upstream_url'], snapshot['token'])
    probe.uid, probe.sid = snapshot['uid'], snapshot['sid']
    probe.call('GET', '/api/user/self', allow_failure=True)
    checked = time.time()
    return {'expired': checked >= snapshot['expiresAt'] and probe.status == 401,
            'httpStatus': probe.status, 'jwtExpiresAt': snapshot['expiresAt'], 'checkedAt': checked}

def wait_for_management_window():
    pacing = {'reason': 'separate critical-management batches without changing upstream limits',
              'seconds': MANAGEMENT_WINDOW_SECONDS, 'phase': 'waiting',
              'runnerLoginAttempts': {'root': 0, 'ordinaryUser': 0}}
    state['rateLimitPacing'] = pacing
    save()
    started = time.monotonic()
    while time.monotonic() - started < MANAGEMENT_WINDOW_SECONDS:
        time.sleep(min(30, MANAGEMENT_WINDOW_SECONDS - (time.monotonic() - started)))
        pacing['waitedSeconds'] = round(time.monotonic() - started, 3)
        save()
    pacing.update(phase='root-login', waitedSeconds=round(time.monotonic() - started, 3))
    pacing['runnerLoginAttempts']['root'] = 1
    save()
    root.login(cfg['newapi_root_user'], cfg['newapi_root_password'])
    assert root.uid == 1, 'newapi_root_identity_mismatch'
    pacing['phase'] = 'ordinary-user-login'
    pacing['runnerLoginAttempts']['ordinaryUser'] = 1
    save()
    login = user.login(private['username'], private['password'])
    assert user.uid == uid and login['data']['user']['role'] == 1, 'ordinary_user_role_or_identity_mismatch'
    pacing['phase'] = 'ready'
    save()

def checkin(aid):
    return m.call('POST', '/api/checkin/trigger/' + str(aid), allow_failure=True)

def quota():
    return user.call('GET', '/api/user/self')['data']['quota']

def options():
    return {x['key']: x['value'] for x in root.call('GET', '/api/option/')['data']}

def find_created_user_id():
    data = items(root.call('GET', '/api/user/search?keyword=' + urllib.parse.quote(private['username'], safe='')))
    return next((x['id'] for x in data if x.get('username') == private['username']), None)

def run(config):
    global cfg, state, private, failures, owned_accounts, root, user, m, uid, created_site, user_created, original_options, main_before, secret_file
    cfg = config
    cfg['evidence_dir'].mkdir(parents=True, exist_ok=True)
    cfg['secret_dir'].mkdir(parents=True, exist_ok=True)
    cfg['secret_dir'].chmod(448)
    run_id = secrets.token_hex(8)
    state = {'status': 'running', 'runId': run_id, 'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'steps': [], 'cleanup': [], 'runnerSha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    private = {'username': 'ma' + secrets.token_hex(5), 'password': secrets.token_urlsafe(18), 'siteName': 'management-acceptance-' + run_id}
    secret_file = cfg['secret_dir'] / ('management-' + run_id + '.json')
    failures = FailureAccumulator()
    owned_accounts = set()
    root = user = m = None
    uid = None
    created_site = None
    user_created = False
    original_options = {}
    main_before = None
    m = API(cfg['base_url'], cfg['auth_token'])
    save()
    try:
        about = m.call('GET', '/api/about')
        state['preconditions'] = {'expectedServerCommit': cfg['expect_server_commit'], 'aboutCommit': about.get('commit'), 'aboutCommitMatches': about.get('commit') == cfg['expect_server_commit']}
        assert state['preconditions']['aboutCommitMatches'], 'server_commit_mismatch'
        sites = items(m.call('GET', '/api/sites'))
        accounts = account_summary()
        state['preconditions']['metapiSitesEmpty'] = sites == []
        state['preconditions']['metapiAccountsEmpty'] = accounts == []
        assert sites == [], 'metapi_sites_not_empty'
        assert accounts == [], 'metapi_accounts_not_empty'
        main_before = accounts
        site = m.call('POST', '/api/sites', {'name': private['siteName'], 'url': cfg['upstream_url'], 'platform': 'new-api'})
        created_site = site['id']
        private['siteId'] = created_site
        save()
        root = API(cfg['upstream_url'])
        root.login(cfg['newapi_root_user'], cfg['newapi_root_password'])
        assert root.uid == 1, 'newapi_root_identity_mismatch'
        original_options = {k: v for k, v in options().items() if k.startswith('checkin_setting.')}
        assert original_options.get('checkin_setting.enabled') == 'true', 'checkin_setting_enabled_not_true'
        assert find_created_user_id() is None, 'user_name_collision'
        user_created = True
        root.call('POST', '/api/user/', {'username': private['username'], 'password': private['password'], 'display_name': 'Metapi acceptance', 'role': 1})
        user_created = True
        user = API(root.base)
        login = user.login(private['username'], private['password'])
        uid = user.uid
        private['userId'] = uid
        state['ordinaryUserRole'] = login['data']['user']['role']
        save()
        assert uid > 1 and login['data']['user']['role'] == 1, 'ordinary_user_role_mismatch'
        observe('create isolated ordinary NewAPI user', True, userId=uid)
        body = {'siteId': site['id'], 'username': private['username'], 'password': private['password']}
        logged = m.call('POST', '/api/accounts/login', body)
        aid = logged['account']['id']
        owned_accounts.add(aid)
        save()
        duplicate = m.call('POST', '/api/accounts/login', body)
        private['managementToken'] = duplicate['account']['accessToken']
        save()
        observe('password login persists and repeat login reuses account', duplicate['account']['id'] == aid and duplicate.get('reusedAccount') is True, accountId=aid)
        expiry = int(time.time()) + 14400
        for label in ('first', 'second'):
            m.call('POST', '/api/account-tokens', {'accountId': aid, 'name': private['username'] + '-' + label, 'group': 'default', 'unlimitedQuota': False, 'remainQuota': 2000000, 'expiredTime': expiry})
        local = tokens(aid)
        up = own_upstream_tokens()
        first = next((t for t in local if t['name'].endswith('-first')))
        second = next((t for t in local if t['name'].endswith('-second')))
        own = [t for t in up if t['name'] in (private['username'] + '-first', private['username'] + '-second')]
        observe('Metapi creates real finite upstream tokens', len(own) == 2 and all((t['remain_quota'] == 2000000 and (not t['unlimited_quota']) and (t['expired_time'] == expiry) for t in own)), localTokenCount=len(local), upstreamTokenCount=len(own))
        first_value = m.call('GET', f"/api/account-tokens/{first['id']}/value")['token']
        private['relayToken'] = first_value
        save()
        updated = m.call('PUT', f"/api/account-tokens/{first['id']}", {'name': private['username'] + '-local-name'})
        observe('metadata update does not return stored plaintext token', 'token' not in updated.get('token', {}), plaintextFieldReturned='token' in updated.get('token', {}), maskedFieldReturned='tokenMasked' in updated.get('token', {}))
        up = own_upstream_tokens()
        observe('metadata rename stays local as documented', any((t['name'] == private['username'] + '-first' for t in up)) and next((t for t in tokens(aid) if t['id'] == first['id']))['name'] == private['username'] + '-local-name')
        m.call('POST', f"/api/account-tokens/{first['id']}/default")
        m.call('PUT', f"/api/account-tokens/{first['id']}", {'enabled': False})
        local = tokens(aid)
        observe('disabled local default moves to a usable token', not next((t for t in local if t['id'] == first['id']))['isDefault'] and next((t for t in local if t['id'] == second['id']))['isDefault'])
        m.call('PUT', f"/api/account-tokens/{first['id']}", {'enabled': True})
        m.call('POST', f"/api/account-tokens/{first['id']}/default")
        second_up = next((t for t in own_upstream_tokens() if t['name'] == private['username'] + '-second'))['id']
        m.call('DELETE', f"/api/account-tokens/{second['id']}")
        local_gone = all((t['id'] != second['id'] for t in tokens(aid)))
        up_gone = all((t['id'] != second_up for t in own_upstream_tokens()))
        observe('token deletion removes both local and upstream records', local_gone and up_gone, localAbsent=local_gone, upstreamAbsent=up_gone, upstreamTokenId=second_up)
        m.call('DELETE', f'/api/accounts/{aid}')
        owned_accounts.remove(aid)
        pat = private['managementToken']
        verify = m.call('POST', '/api/accounts/verify-token', {'siteId': site['id'], 'accessToken': pat, 'credentialMode': 'session', 'platformUserId': uid})
        assert verify.get('tokenType') == 'session'
        imported = m.call('POST', '/api/accounts', {'siteId': site['id'], 'username': private['username'], 'accessToken': pat, 'credentialMode': 'session', 'platformUserId': uid, 'checkinEnabled': True})
        aid = imported['id']
        owned_accounts.add(aid)
        private['accountId'] = aid
        save()
        models = m.call('GET', f'/api/accounts/{aid}/models')
        observe('PAT import discovers models with existing relay credentials', models['totalCount'] > 0, tokenType=verify['tokenType'], modelCount=models['totalCount'])
        m.call('POST', f'/api/accounts/{aid}/balance')
        before = quota()
        info = user.call('GET', '/api/user/checkin')['data']
        assert info['min_quota'] == info['max_quota'] == 1000
        fresh = checkin(aid)
        after = quota()
        observe('fresh checkin grants exactly the configured upstream reward', fresh.get('success') is True and fresh.get('status') == 'success' and (fresh.get('skipped') is False) and (after - before == 1000), outcome=fresh.get('status'), quotaDelta=after - before)
        repeated = checkin(aid)
        after_repeat = quota()
        observe('already checked upstream is success without another reward', repeated.get('success') is True and repeated.get('status') == 'success' and (repeated.get('skipped') is False) and (after_repeat == after), outcome=repeated.get('status'), skipped=repeated.get('skipped'), quotaDelta=after_repeat - after)
        root.call('PUT', '/api/option/', {'key': 'checkin_setting.enabled', 'value': 'false'})
        try:
            disabled = checkin(aid)
            observe('upstream disabled checkin is explicit skip', disabled.get('status') == 'skipped' and disabled.get('skipped') is True, outcome=disabled.get('status'))
        finally:
            root.call('PUT', '/api/option/', {'key': 'checkin_setting.enabled', 'value': original_options['checkin_setting.enabled']})
        old_session = {'token': user.key, 'uid': user.uid, 'sid': user.sid, 'expiresAt': session_jwt_expiry(user.key)}
        old_identity = user.call('GET', '/api/user/self')['data']
        assert old_identity['id'] == uid and time.time() < old_session['expiresAt'], 'original_user_jwt_not_valid_before_window'
        wait_for_management_window()
        expiry_facts = probe_expired_session(old_session)
        state['naturalJwtExpiry'] = expiry_facts
        observe('ordinary login JWT expires naturally at the real upstream', expiry_facts['expired'], **{k: v for k, v in expiry_facts.items() if k != 'expired'})
        durable_balance = m.call('POST', f'/api/accounts/{aid}/balance')
        observe('persisted PAT still manages the account after JWT expiry', durable_balance.get('balance', 0) > 0 and durable_balance.get('skipped') is not True, balance=durable_balance.get('balance'))
        newer = user.mint_pat(private['password'])
        private['managementToken'] = newer
        save()
        expired = checkin(aid)
        observe('revoked PAT fails without password self-healing', expired.get('status') == 'failed' and expired.get('success') is False, outcome=expired.get('status'))
        rebound = m.call('POST', f'/api/accounts/{aid}/rebind-session', {'accessToken': newer})
        observe('session rebind keeps credentials redacted', newer not in json.dumps(rebound) and first_value not in json.dumps(rebound))
        restored = m.call('POST', f'/api/accounts/{aid}/balance')
        restored_checkin = checkin(aid)
        observe('credential replacement restores balance and checkin management', restored.get('balance', 0) > 0 and restored_checkin.get('status') == 'success', outcome=restored_checkin.get('status'))
        m.call('PUT', f'/api/accounts/{aid}', {'status': 'disabled'})
        disabled_balance = m.call('POST', f'/api/accounts/{aid}/balance')
        disabled_account = checkin(aid)
        observe('disabled account skips management probes', disabled_balance.get('skipped') is True and disabled_balance.get('reason') == 'account_disabled' and (disabled_account.get('status') == 'skipped') and (disabled_account.get('skipped') is True))
        m.call('PUT', f'/api/accounts/{aid}', {'status': 'active'})
        proxy_verify = m.call('POST', '/api/accounts/verify-token', {'siteId': site['id'], 'accessToken': first_value, 'credentialMode': 'apikey'})
        proxy_account = m.call('POST', '/api/accounts', {'siteId': site['id'], 'username': private['username'] + '-key', 'accessToken': first_value, 'credentialMode': 'apikey', 'checkinEnabled': False})
        proxy_id = proxy_account['id']
        owned_accounts.add(proxy_id)
        save()
        proxy_balance = m.call('POST', f'/api/accounts/{proxy_id}/balance')
        proxy_checkin = checkin(proxy_id)
        observe('API-key connection is honestly proxy-only', proxy_verify.get('tokenType') == 'apikey' and proxy_balance.get('reason') == 'proxy_only' and (proxy_balance.get('skipped') is True) and (proxy_checkin.get('status') == 'skipped'))
        m.call('POST', '/api/account-tokens', {'accountId': proxy_id, 'name': 'must-not-create'}, allow_failure=True)
        observe('API-key connection cannot manage upstream tokens', m.status == 400, httpStatus=m.status)
        before_negative = account_summary()
        failed = m.call('POST', '/api/accounts/login', {'siteId': site['id'], 'username': private['username'], 'password': 'wrong-acceptance-password'}, allow_failure=True)
        http = m.status
        observe('wrong password does not change account identities', http == 401 and '429' not in json.dumps(failed) and (account_summary() == before_negative), httpStatus=http)
        failed = m.call('POST', '/api/accounts/verify-token', {'siteId': site['id'], 'accessToken': 'invalid-acceptance-token', 'credentialMode': 'session', 'platformUserId': uid}, allow_failure=True)
        http = m.status
        observe('invalid management token persists no additional account', http == 400 and account_summary() == before_negative, httpStatus=http)
    except (Exception, KeyboardInterrupt) as error:
        state['failedOperation'] = dict(state.get('lastOperation', {}))
        state['fatal'] = {'errorType': type(error).__name__, 'detail': str(error) if isinstance(error, AssertionError) else 'operation failed; inspect lastOperation'}
    finally:
        if root and root.uid == 1 and original_options:
            try:
                current = options()
                for key, value in original_options.items():
                    if current.get(key) != value:
                        root.call('PUT', '/api/option/', {'key': key, 'value': value})
                restored = {k: v for k, v in options().items() if k.startswith('checkin_setting.')}
                failures.record_cleanup('restore original checkin options', restored == original_options)
            except Exception as error:
                failures.record_cleanup('restore original checkin options', False, errorType=type(error).__name__)
        cleanup_uid = uid
        if root and user_created and (not cleanup_uid):
            try:
                cleanup_uid = find_created_user_id()
                if not cleanup_uid:
                    failures.record_cleanup('locate created NewAPI user', False, userId=None)
            except Exception as error:
                failures.record_cleanup('locate created NewAPI user', False, errorType=type(error).__name__)
        if user and cleanup_uid and (cleanup_uid > 1):
            try:
                for token in own_upstream_tokens():
                    user.call('DELETE', '/api/token/' + str(token['id']))
                failures.record_cleanup('remove only new user upstream tokens', not own_upstream_tokens())
            except Exception as error:
                failures.record_cleanup('remove only new user upstream tokens', False, errorType=type(error).__name__)
        for aid in sorted(owned_accounts):
            try:
                m.call('DELETE', '/api/accounts/' + str(aid))
                failures.record_cleanup('remove owned test account', True, accountId=aid)
            except Exception as error:
                failures.record_cleanup('remove owned test account', False, accountId=aid, errorType=type(error).__name__)
        if root and cleanup_uid and (cleanup_uid > 1):
            try:
                root.call('DELETE', '/api/user/' + str(cleanup_uid))
                failures.record_cleanup('remove created NewAPI user', find_created_user_id() is None, userId=cleanup_uid)
            except Exception as error:
                failures.record_cleanup('remove created NewAPI user', False, errorType=type(error).__name__)
        if m and created_site:
            try:
                m.call('DELETE', '/api/sites/' + str(created_site))
                failures.record_cleanup('remove owned Metapi site', True, siteId=created_site)
            except Exception as error:
                failures.record_cleanup('remove owned Metapi site', False, siteId=created_site, errorType=type(error).__name__)
        if main_before is not None:
            try:
                failures.record_cleanup('preserve pre-run Metapi account identities', account_summary() == main_before)
            except Exception as error:
                failures.record_cleanup('preserve pre-run Metapi account identities', False, errorType=type(error).__name__)
        state['status'] = 'pass' if not state.get('fatal') and failures.exit_code() == 0 else 'fail'
        state['expectedSteps'] = len(CHECKS)
        state['missingSteps'] = [name for name in CHECKS if name not in {x['name'] for x in failures.steps}]
        state['finishedAt'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        save()
    print('management acceptance: ' + state['status'] + ' (' + str(len(state['steps'])) + ' steps)')
    return 0 if state['status'] == 'pass' else 1

def main():
    if sys.argv[1:] == ['--help']:
        print(__doc__)
        print('Required env: ' + ' '.join(REQUIRED_ENV) + ' MANAGEMENT_DISPOSABLE=1 MANAGEMENT_ALLOW_MUTATIONS=1')
        return 0
    if sys.argv[1:] or not __debug__:
        print('unsupported arguments or optimized Python mode', file=sys.stderr)
        return 2
    try:
        config = load_config()
    except ConfigError as error:
        print(str(error), file=sys.stderr)
        return 2
    return run(config)
if __name__ == '__main__':
    raise SystemExit(main())
