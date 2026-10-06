"""Offline instrument tests; they do not prove a live New API journey."""

import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location(
    "management_matrix", Path(__file__).with_name("verify-management-matrix.py"))
matrix = importlib.util.module_from_spec(spec)
spec.loader.exec_module(matrix)

CONFIG = {
    "metapi": "http://127.0.0.1:4000",
    "newapi": "http://127.0.0.1:3000",
    "admin": "fixture-admin",
    "commit": "a" * 40,
    "username": "fixture-user",
    "password": "fixture-password",
}


class FixtureHTTP:
    def __init__(self, *, omit_upstream_create=False, omit_upstream_delete=False,
                 partial_inventory=False, leaky_login_failure=False):
        self.count = 0
        self.omit_upstream_create = omit_upstream_create
        self.omit_upstream_delete = omit_upstream_delete
        self.partial_inventory = partial_inventory
        self.leaky_login_failure = leaky_login_failure
        self.site = None
        self.account = None
        self.local = []
        self.upstream = []
        self.paths = []
        self.last = {"method": "", "path": "", "status": 0}

    def call(self, base, method, path, headers=None, payload=None):
        self.count += 1
        self.paths.append((base, method, path))
        self.last = {"method": method, "path": path.split("?", 1)[0], "status": 200}
        if base == CONFIG["newapi"]:
            if (method, path) == ("POST", "/api/user/login"):
                if self.leaky_login_failure:
                    self.last["status"] = 401
                    return 401, {"error": "fixture-password fixture-session"}
                return 200, {"success": True, "data": {"user": {"id": 7, "role": 1},
                             "access_token": "fixture-session", "session": {"sid": "fixture-sid"}}}
            if (method, path) == ("GET", "/api/token/?p=1&page_size=100"):
                if headers.get("New-Api-User") != "7" or headers.get("X-Auth-Session") != "fixture-sid":
                    return 401, {"success": False}
                total = len(self.upstream) + (1 if self.partial_inventory else 0)
                return 200, {"success": True, "data": {"items": list(self.upstream), "total": total}}
        if base != CONFIG["metapi"]:
            raise AssertionError("unexpected origin")
        if (method, path) == ("GET", "/api/about"):
            return 200, {"commit": CONFIG["commit"]}
        if not headers or headers.get("Authorization") != "Bearer " + CONFIG["admin"]:
            return 401, {"error": "unauthorized"}
        if (method, path) == ("GET", "/api/sites"):
            return 200, [] if self.site is None else [self.site]
        if (method, path) == ("GET", "/api/accounts"):
            return 200, {"accounts": [] if self.account is None else [self.account]}
        if (method, path) == ("POST", "/api/sites"):
            self.site = {"id": 1, "name": payload["name"]}
            return 200, self.site
        if (method, path) == ("POST", "/api/accounts/login"):
            self.account = {"id": 2, "siteId": 1, "username": CONFIG["username"],
                            "accessToken": "fixture-management-pat"}
            return 200, {"success": True, "reusedAccount": False, "account": self.account}
        if (method, path) == ("POST", "/api/account-tokens"):
            self.local = [{"id": 3, "accountId": 2, "name": payload["name"]}]
            if not self.omit_upstream_create:
                self.upstream = [{"id": 4, "name": payload["name"],
                                  "unlimited_quota": False, "remain_quota": 100000}]
            return 200, {"success": True, "synced": True}
        if (method, path) == ("GET", "/api/account-tokens?accountId=2"):
            return 200, list(self.local)
        if (method, path) == ("GET", "/api/account-tokens/3/value"):
            return 200, {"token": "sk-fixture-value"}
        if (method, path) == ("PUT", "/api/account-tokens/3"):
            self.local[0]["name"] = payload["name"]
            return 200, {"token": self.local[0]}
        if (method, path) == ("DELETE", "/api/account-tokens/3"):
            self.local = []
            if not self.omit_upstream_delete:
                self.upstream = []
            return 200, {"success": True}
        raise AssertionError("unexpected request")


class ManagementMatrixInstrumentTest(unittest.TestCase):
    def run_fixture(self, **kwargs):
        events = []
        http = FixtureHTTP(**kwargs)
        rc = matrix.run(CONFIG, http, events.append)
        return rc, events, http

    def test_complete_journey_requires_all_eight_real_operations(self):
        rc, events, http = self.run_fixture()
        summary = events[-1]
        self.assertEqual(rc, 0)
        self.assertEqual(summary["status"], "PASS")
        self.assertEqual(summary["completedChecks"], len(matrix.CHECKS))
        self.assertEqual([e["name"] for e in events if e["event"] == "check"], list(matrix.CHECKS))
        self.assertGreater(http.count, len(matrix.CHECKS))
        self.assertIn((CONFIG["newapi"], "GET", "/api/token/?p=1&page_size=100"), http.paths)
        self.assertEqual(http.local, [])
        self.assertEqual(http.upstream, [])
        self.assertFalse(any("fixture-password" in str(event) or "fixture-session" in str(event)
                             for event in events))

    def test_local_success_without_upstream_token_is_red(self):
        rc, events, _ = self.run_fixture(omit_upstream_create=True)
        self.assertEqual(rc, 1)
        self.assertEqual(events[-1]["code"], "assertion_failed_upstream_token_observed")
        self.assertGreater(len(events[-1]["missingChecks"]), 0)

    def test_local_delete_without_upstream_delete_is_red(self):
        rc, events, _ = self.run_fixture(omit_upstream_delete=True)
        self.assertEqual(rc, 1)
        self.assertEqual(events[-1]["code"], "assertion_failed_created_token_deleted_on_both_sides")

    def test_partial_upstream_inventory_does_not_prove_absence(self):
        rc, events, http = self.run_fixture(partial_inventory=True)
        self.assertEqual(rc, 1)
        self.assertEqual(events[-1]["code"], "incomplete_upstream_inventory")
        self.assertIsNone(http.site)

    def test_upstream_error_body_is_never_reported(self):
        rc, events, _ = self.run_fixture(leaky_login_failure=True)
        self.assertEqual(rc, 1)
        self.assertEqual(events[-1]["code"], "unexpected_http_status")
        self.assertNotIn("fixture-password", str(events))
        self.assertNotIn("fixture-session", str(events))

    def test_zero_check_matrix_and_missing_opt_in_are_red(self):
        events = []
        report = matrix.Report(events.append)
        self.assertEqual(report.finish(None, FixtureHTTP()), 1)
        self.assertEqual(events[-1]["status"], "FAIL")
        with self.assertRaises(matrix.Failure) as err:
            matrix.config({"MANAGEMENT_DISPOSABLE": "1"})
        self.assertEqual(err.exception.code, "disposable_opt_in_required")

    def test_remote_or_ambiguous_origins_are_rejected(self):
        for value in ("https://example.com", "http://localhost.evil.test", "http://localhost/path",
                      "http://user@localhost:4000", "http://127.0.0.1:4000?to=upstream"):
            with self.subTest(value=value), self.assertRaises(matrix.Failure):
                matrix.origin(value)


if __name__ == "__main__":
    unittest.main()
