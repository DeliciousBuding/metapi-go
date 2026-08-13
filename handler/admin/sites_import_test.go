package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSites_ImportIdempotent(t *testing.T) {
	_, r := setupSitesTest(t)

	body := map[string]any{
		"duplicateStrategy": "skip",
		"items": []map[string]any{
			{"name": "OpenAI", "url": "https://api.openai.com/v1"},
			{"name": "Anthropic", "url": "https://api.anthropic.com/v1"},
		},
	}
	resp := doPostJSON(t, r, "/api/sites/import", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["imported"].(float64) != 2 {
		t.Fatalf("imported=%v want 2 (body=%s)", result["imported"], resp.Body.String())
	}

	// Idempotent re-run: both skipped.
	resp2 := doPostJSON(t, r, "/api/sites/import", body)
	var result2 map[string]any
	if err := json.Unmarshal(resp2.Body.Bytes(), &result2); err != nil {
		t.Fatalf("decode rerun: %v", err)
	}
	if result2["imported"].(float64) != 0 || result2["skipped"].(float64) != 2 {
		t.Fatalf("rerun imported=%v skipped=%v want 0/2", result2["imported"], result2["skipped"])
	}
}

func TestSites_ImportMergeAccounts(t *testing.T) {
	_, r := setupSitesTest(t)

	first := map[string]any{
		"duplicateStrategy": "skip",
		"items": []map[string]any{
			{"name": "OpenAI", "url": "https://api.openai.com/v1", "accounts": []map[string]any{
				{"username": "ops", "accessToken": "sk-1"},
			}},
		},
	}
	resp := doPostJSON(t, r, "/api/sites/import", first)
	if resp.Code != http.StatusOK {
		t.Fatalf("first import status=%d body=%s", resp.Code, resp.Body.String())
	}

	merge := map[string]any{
		"duplicateStrategy": "skip",
		"items": []map[string]any{
			{"name": "OpenAI", "url": "https://api.openai.com/v1", "duplicateStrategy": "merge", "accounts": []map[string]any{
				{"username": "ops2", "accessToken": "sk-2"},
			}},
		},
	}
	resp2 := doPostJSON(t, r, "/api/sites/import", merge)
	if resp2.Code != http.StatusOK {
		t.Fatalf("merge import status=%d body=%s", resp2.Code, resp2.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp2.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode merge: %v", err)
	}
	if result["imported"].(float64) != 1 {
		t.Fatalf("merge imported=%v want 1 (body=%s)", result["imported"], resp2.Body.String())
	}
	results := result["results"].([]any)
	if results[0].(map[string]any)["status"] != "merged" {
		t.Fatalf("merge status=%v want merged", results[0].(map[string]any)["status"])
	}
}
