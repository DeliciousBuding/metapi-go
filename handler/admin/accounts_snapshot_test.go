package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// An empty database must serialize the accounts snapshot collections as []
// rather than null: the admin frontend rejects payloads whose accounts field
// is not an array (useAccounts Array.isArray guard), so a nil Go slice
// marshaled to JSON null triggered silent react-query retries on the sites
// and checkin pages. These tests pin the empty-DB contract for both the
// cached snapshot and the paginated variant.

func TestAccountsSnapshot_EmptyDB_SerializesEmptyArrays(t *testing.T) {
	_, r, _ := setupAccountsTest(t)

	resp := doGet(t, r, "/api/accounts")
	if resp.Code != http.StatusOK {
		t.Fatalf("list accounts on empty db: %d %s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	if strings.Contains(body, `"accounts":null`) {
		t.Errorf(`accounts serialized as null, want []; body=%s`, body)
	}
	if strings.Contains(body, `"sites":null`) {
		t.Errorf(`sites serialized as null, want []; body=%s`, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("unmarshal snapshot body: %v", err)
	}
	if string(raw["accounts"]) != "[]" {
		t.Errorf(`accounts = %s, want []`, raw["accounts"])
	}
	if string(raw["sites"]) != "[]" {
		t.Errorf(`sites = %s, want []`, raw["sites"])
	}
	if len(raw["generatedAt"]) == 0 {
		t.Error("generatedAt missing from empty snapshot — contract fields must be preserved")
	}
}

func TestComputeAccountsSnapshot_EmptyDB_SerializesEmptyArrays(t *testing.T) {
	db, _, _ := setupAccountsTest(t)
	handler := &accountsHandler{db: db.DB}

	data, err := handler.computeAccountsSnapshot()
	if err != nil {
		t.Fatalf("computeAccountsSnapshot on empty db: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if string(raw["accounts"]) != "[]" {
		t.Errorf(`accounts = %s, want []`, raw["accounts"])
	}
	if string(raw["sites"]) != "[]" {
		t.Errorf(`sites = %s, want []`, raw["sites"])
	}
}

func TestAccountsSnapshot_EmptyDB_PaginatedSerializesEmptyArrays(t *testing.T) {
	_, r, _ := setupAccountsTest(t)

	resp := doGet(t, r, "/api/accounts?page=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("paginated list accounts on empty db: %d %s", resp.Code, resp.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal paginated body: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf(`items = %s, want []`, raw["items"])
	}
	if string(raw["sites"]) != "[]" {
		t.Errorf(`sites = %s, want []`, raw["sites"])
	}
}

// Non-empty snapshots must keep their existing shape: arrays of objects, all
// three contract fields present. Guards against the empty-array fix
// accidentally altering populated responses.
func TestAccountsSnapshot_PopulatedDB_KeepsArrayShape(t *testing.T) {
	_, r, _ := setupAccountsTest(t)
	setupAccountFixture(t, r)

	resp := doGet(t, r, "/api/accounts")
	if resp.Code != http.StatusOK {
		t.Fatalf("list accounts: %d %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal snapshot body: %v", err)
	}
	accounts, ok := result["accounts"].([]any)
	if !ok || len(accounts) != 1 {
		t.Fatalf("accounts = %#v, want 1-element array", result["accounts"])
	}
	sites, ok := result["sites"].([]any)
	if !ok || len(sites) != 1 {
		t.Fatalf("sites = %#v, want 1-element array", result["sites"])
	}
	if result["generatedAt"] == nil {
		t.Error("generatedAt missing from populated snapshot")
	}
}
