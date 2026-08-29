package admin

import (
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestSettingsRuntimeStatusRangesPersistAndApply(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyRetryStatusRanges":   "401,500-599",
		"proxyDisableStatusRanges": "401",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update runtime: %d %s", resp.Code, resp.Body.String())
	}
	if config.Runtime().ProxyRetryStatusRanges != "401,500-599" {
		t.Fatalf("config.Runtime().ProxyRetryStatusRanges = %q, want 401,500-599", config.Runtime().ProxyRetryStatusRanges)
	}
	if config.Runtime().ProxyDisableStatusRanges != "401" {
		t.Fatalf("config.Runtime().ProxyDisableStatusRanges = %q, want 401", config.Runtime().ProxyDisableStatusRanges)
	}

	for key, want := range map[string]string{
		"proxy_retry_status_ranges":   `"401,500-599"`,
		"proxy_disable_status_ranges": `"401"`,
	} {
		var stored string
		if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("query %s: %v", key, err)
		}
		if stored != want {
			t.Fatalf("stored %s = %q, want %q", key, stored, want)
		}
	}
}

func TestSettingsRuntimeStatusRangesRejectInvalidSpec(t *testing.T) {
	_, r, _ := setupEdgeTest(t)

	for _, spec := range []any{"not-a-range", "500-400", "600"} {
		resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
			"proxyRetryStatusRanges": spec,
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("spec %v: status = %d body=%s, want 400", spec, resp.Code, resp.Body.String())
		}
		if config.Runtime().ProxyRetryStatusRanges != "" {
			t.Fatalf("spec %v: cfg mutated to %q, want untouched", spec, config.Runtime().ProxyRetryStatusRanges)
		}
	}

	// Non-string values follow the existing settings contract: normalize to
	// the empty spec (restore defaults), never a 400.
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyRetryStatusRanges": 42,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("numeric spec: status = %d body=%s, want 200 (normalize to empty)", resp.Code, resp.Body.String())
	}
	if config.Runtime().ProxyRetryStatusRanges != "" {
		t.Fatalf("numeric spec: cfg mutated to %q, want empty", config.Runtime().ProxyRetryStatusRanges)
	}
}

func TestSettingsRuntimeStatusRangesEmptyClearsToDefault(t *testing.T) {
	_, r, _ := setupEdgeTest(t)

	// Set a custom value first, then clear it: the apply path accepts the
	// empty spec and the routing layer falls back to the historical default.
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyDisableStatusRanges": "401",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("set disable: %d %s", resp.Code, resp.Body.String())
	}

	resp = doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyDisableStatusRanges": "",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("clear disable: %d %s", resp.Code, resp.Body.String())
	}
	if config.Runtime().ProxyDisableStatusRanges != "" {
		t.Fatalf("config.Runtime().ProxyDisableStatusRanges = %q after clear, want empty", config.Runtime().ProxyDisableStatusRanges)
	}
}
