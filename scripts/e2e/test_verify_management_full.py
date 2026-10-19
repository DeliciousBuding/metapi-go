"""Offline instrument checks, not a live management acceptance result."""

import hashlib
import importlib.util
import json
from pathlib import Path

import pytest


spec = importlib.util.spec_from_file_location(
    "management_full", Path(__file__).with_name("verify-management-full.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def test_full_sequence_is_required_not_only_some_passing_steps():
    assert len(runner.CHECKS) == 21
    assert len(set(runner.CHECKS)) == 21
    result = runner.FailureAccumulator()
    assert result.exit_code() == 1
    for name in runner.CHECKS[:-1]:
        result.record_step(name, True)
    assert result.exit_code() == 1
    result.record_step(runner.CHECKS[-1], True)
    assert result.exit_code() == 0
    result.record_cleanup('failed cleanup', False)
    assert result.exit_code() == 1


def test_reordering_or_failure_cannot_pass():
    for names in (runner.CHECKS[::-1], runner.CHECKS + runner.CHECKS[:1]):
        result = runner.FailureAccumulator()
        for name in names:
            result.record_step(name, True)
        assert result.exit_code() == 1
    result = runner.FailureAccumulator()
    for name in runner.CHECKS:
        result.record_step(name, name != runner.CHECKS[10])
    assert result.exit_code() == 1


def test_production_origin_and_missing_opt_in_are_rejected(tmp_path):
    env = {
        'BASE_URL': 'http://127.0.0.1:4000', 'UPSTREAM_URL': 'http://127.0.0.1:3000',
        'AUTH_TOKEN': 'fixture-admin', 'NEWAPI_ROOT_USER': 'root',
        'NEWAPI_ROOT_PASSWORD': 'fixture-password', 'EXPECT_SERVER_COMMIT': 'a' * 40,
        'EVIDENCE_DIR': str(tmp_path / 'evidence'), 'SECRET_DIR': str(tmp_path / 'secrets'),
        'MANAGEMENT_DISPOSABLE': '1', 'MANAGEMENT_ALLOW_MUTATIONS': '1',
    }
    runner.load_config(env)
    for changes in (
        {'MANAGEMENT_ALLOW_MUTATIONS': '0'},
        {'UPSTREAM_URL': 'https://real.example.com'},
        # Deliberately fake userinfo exercises rejection, never used for a request.
        {'BASE_URL': 'http://user:secret@127.0.0.1:4000'},  # leak-guard-allow:LG-F4C6A155
        {'EXPECT_SERVER_COMMIT': 'abc'},
    ):
        with pytest.raises(runner.ConfigError):
            runner.load_config({**env, **changes})


@pytest.mark.parametrize("omit_revoke", [False, True], ids=["revoke", "omit_revoke"])
def test_exact_fingerprint_revoke_is_required_after_scoped_creation(monkeypatch, omit_revoke):
    api = runner.API('http://127.0.0.1:3000', 'fixture-jwt')
    old_token = 'nap_fixture-old-pat'
    old_ref = hashlib.sha256(old_token.encode()).hexdigest()
    removed = False
    calls = []
    proof_calls = []

    def call(method, path, body=None, allow_failure=False, extra_headers=None):
        nonlocal removed
        calls.append((method, path, body, extra_headers))
        api.status = 200
        if path == '/api/user/access_tokens' and method == 'GET':
            return {'data': {'items': [] if removed else [{'id': 7, 'token_ref': old_ref}]}}
        if path == '/api/user/access_tokens' and method == 'POST':
            if not extra_headers:
                api.status = 403
                return {'code': 'SECURITY_PROOF_REQUIRED'}
            return {'data': {'token': 'nap_fixture-new-pat'}}
        if path == '/api/user/access_tokens/7' and method == 'DELETE':
            removed = not omit_revoke
            return {'success': True}
        pytest.fail('unexpected offline operation')

    def proof(scope, password, context):
        proof_calls.append((scope, password, context))
        return 'fixture-proof'

    monkeypatch.setattr(runner, 'private', {'managementToken': old_token})
    monkeypatch.setattr(api, 'call', call)
    monkeypatch.setattr(api, 'proof', proof)
    if omit_revoke:
        with pytest.raises(AssertionError, match='revoked_pat_still_listed'):
            api.mint_pat('fixture-password')
    else:
        assert api.mint_pat('fixture-password') == 'nap_fixture-new-pat'
    assert ('access_token.revoke', 'fixture-password', {'token_id': 7}) in proof_calls
    assert ('DELETE', '/api/user/access_tokens/7', None,
            {'X-Security-Proof': 'fixture-proof'}) in calls


class _Probe:
    def __init__(self, status):
        self.status = status

    def call(self, method, path, allow_failure=False):
        pass


@pytest.mark.parametrize(("status", "now", "expected"), [
    (401, 901, True),
    (401, 899, False),
    (429, 901, False),
    (500, 901, False),
])
def test_only_expired_401_proves_natural_expiry(monkeypatch, status, now, expected):
    snapshot = {'token': 'fixture-jwt', 'uid': 7, 'sid': 'fixture-sid', 'expiresAt': 900}
    client = _Probe(status)
    monkeypatch.setattr(runner, 'cfg', {'upstream_url': 'http://127.0.0.1:3000'})
    monkeypatch.setattr(runner, 'API', lambda *args, **kwargs: client)
    monkeypatch.setattr(runner.time, 'time', lambda: now)
    result = runner.probe_expired_session(snapshot)
    assert result['expired'] is expected
    assert snapshot['token'] not in json.dumps(result)
