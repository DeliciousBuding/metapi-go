//go:build integration

package admin

import (
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// pgDSN returns the PostgreSQL DSN from PG_TEST_DSN (empty when unset).
func pgDSN() string {
	return os.Getenv("PG_TEST_DSN")
}

// TestStatsMarketplaceBuildersPostgres is the regression test for the
// 2026-08-17 v0.14.0 deployment incident: four builder queries in
// stats_marketplace.go used SQLite-only boolean literal syntax
// (COALESCE(<bool>, 0) = 1 and <bool> = 1) that PostgreSQL rejects with
// SQLSTATE 42804 ("COALESCE types boolean and integer cannot be matched" /
// "operator does not exist: boolean = integer"). The bug caused
// /api/models/token-candidates to return HTTP 500 on production Azure PG.
//
// This test seeds a site + account + token + availability rows on PostgreSQL
// and exercises all four builders so the SQL runs against PG's BOOLEAN type
// system — catching any future regression to integer-literal comparisons.
//
// Diverges from service/balance/balance_pg_test.go in two ways: (1) uses a
// //go:build integration tag so the file compiles only when -tags=integration
// is passed (the balance test relies solely on t.Skip); (2) uses NOW() which
// is PG-native (the test is PG-only via the build tag + PG_TEST_DSN guard).
func TestStatsMarketplaceBuildersPostgres(t *testing.T) {
	if pgDSN() == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}

	db, err := store.Open(store.DialectPostgres, pgDSN(), false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	// Seed a site + account + token + availability rows. NOW() is PG-native;
	// the rebind helper handles placeholder conversion (? → $N). The test is
	// PG-only (build tag + PG_TEST_DSN guard) so cross-dialect portability
	// of NOW() is not a concern here.
	probeSite := "pg-tc-probe-site"
	probeAccount := "pg-tc-probe-account"
	probeToken := "pg-tc-probe-token"
	probeModel := "gpt-4o-tc-probe"

	// Clean up any leftover rows from a prior run (idempotent re-run safety).
	cleanup := func() {
		db.MustExec(`DELETE FROM token_model_availability WHERE model_name = $1`, probeModel)
		db.MustExec(`DELETE FROM model_availability WHERE model_name = $1`, probeModel)
		db.MustExec(`DELETE FROM account_tokens WHERE name = $1`, probeToken)
		db.MustExec(`DELETE FROM accounts WHERE username = $1`, probeAccount)
		db.MustExec(`DELETE FROM sites WHERE name = $1`, probeSite)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Seed site.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES (?, 'https://probe.example.test', 'openai', 'active', NOW(), NOW())`), probeSite); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, db.Rebind(`SELECT id FROM sites WHERE name = ?`), probeSite); err != nil {
		t.Fatalf("load site id: %v", err)
	}

	// Seed account.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO accounts (site_id, username, access_token, status, created_at, updated_at)
		 VALUES (?, ?, 'sk-probe', 'active', NOW(), NOW())`), siteID, probeAccount); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, db.Rebind(`SELECT id FROM accounts WHERE username = ?`), probeAccount); err != nil {
		t.Fatalf("load account id: %v", err)
	}

	// Seed account_token (enabled, not expired). value_status is NOT NULL on PG;
	// use 'active' so the token is treated as valid by the builder WHERE clauses.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO account_tokens (account_id, name, token, enabled, is_default, value_status, created_at, updated_at)
		 VALUES (?, ?, 'tok-probe', true, true, 'active', NOW(), NOW())`), accountID, probeToken); err != nil {
		t.Fatalf("seed account_token: %v", err)
	}
	var tokenID int64
	if err := db.Get(&tokenID, db.Rebind(`SELECT id FROM account_tokens WHERE name = ?`), probeToken); err != nil {
		t.Fatalf("load token id: %v", err)
	}

	// Seed model_availability (account-level: available=true). This table
	// has no created_at/updated_at columns — uses checked_at TEXT instead.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO model_availability (account_id, model_name, available)
		 VALUES (?, ?, true)`), accountID, probeModel); err != nil {
		t.Fatalf("seed model_availability: %v", err)
	}

	// Seed token_model_availability (token-level: available=true). Same
	// schema as model_availability — no created_at/updated_at.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO token_model_availability (token_id, model_name, available)
		 VALUES (?, ?, true)`), tokenID, probeModel); err != nil {
		t.Fatalf("seed token_model_availability: %v", err)
	}

	// Exercise all four builders. Before the dialect-safe SQL fix, every one
	// of these would fail with PG 42804 because the SQL used
	// COALESCE(<bool>, 0) = 1 and <bool> = 1.
	h := &statsHandler{db: db.DB}
	allowed := h.loadGlobalAllowedModels()

	t.Run("buildTokenCandidateModels", func(t *testing.T) {
		models, err := h.buildTokenCandidateModels(allowed)
		if err != nil {
			t.Fatalf("buildTokenCandidateModels failed on PG: %v", err)
		}
		if len(models) == 0 {
			t.Fatalf("buildTokenCandidateModels returned no models; expected at least %s", probeModel)
		}
		candidates, ok := models[probeModel]
		if !ok {
			t.Fatalf("buildTokenCandidateModels did not include %s; got models: %v", probeModel, modelKeys(models))
		}
		if len(candidates) == 0 {
			t.Fatalf("buildTokenCandidateModels returned 0 candidates for %s", probeModel)
		}
		// The seeded token should appear in the candidate list.
		found := false
		for _, c := range candidates {
			if c["tokenId"].(int64) == tokenID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("buildTokenCandidateModels did not include token %d in candidates for %s", tokenID, probeModel)
		}
	})

	t.Run("buildModelsWithoutToken", func(t *testing.T) {
		// Our seeded account HAS a token, so it should NOT appear in
		// modelsWithoutToken. The key assertion is that the query runs without
		// error on PG — the data shape is a secondary check.
		without, err := h.buildModelsWithoutToken(allowed)
		if err != nil {
			t.Fatalf("buildModelsWithoutToken failed on PG: %v", err)
		}
		// The probe account has a managed token, so it must not be "without token".
		if accounts, ok := without[probeModel]; ok {
			for _, a := range accounts {
				if a["accountId"].(int64) == accountID {
					t.Fatalf("buildModelsWithoutToken incorrectly included probe account %d (it has a token)", accountID)
				}
			}
		}
	})

	t.Run("buildModelsMissingTokenGroups", func(t *testing.T) {
		// The query must run without error on PG. The probe token has no
		// token_group, so it may appear in the "uncertain" surface — but the
		// key assertion is no SQL error.
		_, err := h.buildModelsMissingTokenGroups(allowed)
		if err != nil {
			t.Fatalf("buildModelsMissingTokenGroups failed on PG: %v", err)
		}
	})

	t.Run("buildEndpointTypesByModel", func(t *testing.T) {
		// The query must run without error on PG and should include the
		// probe model (inferred endpoint type "openai" for gpt-* names).
		types, err := h.buildEndpointTypesByModel(allowed)
		if err != nil {
			t.Fatalf("buildEndpointTypesByModel failed on PG: %v", err)
		}
		if len(types) == 0 {
			t.Fatalf("buildEndpointTypesByModel returned no models")
		}
		epTypes, ok := types[probeModel]
		if !ok {
			t.Fatalf("buildEndpointTypesByModel did not include %s; got: %v", probeModel, modelKeys(types))
		}
		if len(epTypes) == 0 {
			t.Fatalf("buildEndpointTypesByModel returned empty types for %s", probeModel)
		}
	})
}

// modelKeys returns the keys of a map for diagnostic output.
func modelKeys[V any](m map[string][]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
