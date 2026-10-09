"""Offline instrument checks, not a live management acceptance result."""
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("management_full", Path(__file__).with_name("verify-management-full.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class FullManagementInstrumentTests(unittest.TestCase):
    def test_full_sequence_is_required_not_only_some_passing_steps(self):
        self.assertEqual(21, len(runner.CHECKS))
        self.assertEqual(21, len(set(runner.CHECKS)))
        result = runner.FailureAccumulator()
        self.assertEqual(1, result.exit_code())
        for name in runner.CHECKS[:-1]:
            result.record_step(name, True)
        self.assertEqual(1, result.exit_code())
        result.record_step(runner.CHECKS[-1], True)
        self.assertEqual(0, result.exit_code())
        result.record_cleanup('failed cleanup', False)
        self.assertEqual(1, result.exit_code())

    def test_reordering_or_failure_cannot_pass(self):
        for names in (runner.CHECKS[::-1], runner.CHECKS + runner.CHECKS[:1]):
            result = runner.FailureAccumulator()
            for name in names:
                result.record_step(name, True)
            self.assertEqual(1, result.exit_code())
        result = runner.FailureAccumulator()
        for name in runner.CHECKS:
            result.record_step(name, name != runner.CHECKS[10])
        self.assertEqual(1, result.exit_code())

    def test_production_origin_and_missing_opt_in_are_rejected(self):
        env = {
            'BASE_URL': 'http://127.0.0.1:4000', 'UPSTREAM_URL': 'http://127.0.0.1:3000',
            'AUTH_TOKEN': 'fixture-admin', 'NEWAPI_ROOT_USER': 'root',
            'NEWAPI_ROOT_PASSWORD': 'fixture-password', 'EXPECT_SERVER_COMMIT': 'a' * 40,
            'EVIDENCE_DIR': 'fixture-evidence', 'SECRET_DIR': 'fixture-secrets',
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
            with self.assertRaises(runner.ConfigError):
                runner.load_config({**env, **changes})

    def test_exact_fingerprint_revoke_is_required_after_scoped_creation(self):
        for omit_revoke in (False, True):
            api = runner.API('http://127.0.0.1:3000', 'fixture-jwt')
            old_token = 'nap_fixture-old-pat'
            old_ref = hashlib.sha256(old_token.encode()).hexdigest()
            removed = False
            calls = []

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
                self.fail('unexpected offline operation')

            with mock.patch.object(runner, 'private', {'managementToken': old_token}), mock.patch.object(api, 'call', side_effect=call), mock.patch.object(api, 'proof', return_value='fixture-proof') as proof:
                if omit_revoke:
                    with self.assertRaisesRegex(AssertionError, 'revoked_pat_still_listed'):
                        api.mint_pat('fixture-password')
                else:
                    self.assertEqual('nap_fixture-new-pat', api.mint_pat('fixture-password'))
                proof.assert_any_call('access_token.revoke', 'fixture-password', {'token_id': 7})
            self.assertIn(('DELETE', '/api/user/access_tokens/7', None, {'X-Security-Proof': 'fixture-proof'}), calls)

    def test_only_expired_401_proves_natural_expiry(self):
        snapshot = {'token': 'fixture-jwt', 'uid': 7, 'sid': 'fixture-sid', 'expiresAt': 900}
        for status, now, expected in ((401, 901, True), (401, 899, False), (429, 901, False), (500, 901, False)):
            client = mock.Mock(status=status)
            with mock.patch.object(runner, 'cfg', {'upstream_url': 'http://127.0.0.1:3000'}), mock.patch.object(runner, 'API', return_value=client), mock.patch.object(runner.time, 'time', return_value=now):
                result = runner.probe_expired_session(snapshot)
            self.assertEqual(expected, result['expired'])
            self.assertNotIn(snapshot['token'], json.dumps(result))


if __name__ == '__main__':
    unittest.main()
