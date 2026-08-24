package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSites_Create_URLValidationRejectsNonHTTP(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "javascript", url: "javascript:alert(1)"},
		{name: "data", url: "data:text/html,<script>alert(1)</script>"},
		{name: "file", url: "file:///etc/passwd"},
		{name: "unparseable", url: "not a url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := setupSitesTest(t)
			resp := doPostJSON(t, r, "/api/sites", map[string]any{
				"name":     "Unsafe site",
				"url":      tt.url,
				"platform": "openai",
			})
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", tt.url, resp.Code, resp.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body["error"] != "Invalid url. Expected a valid http(s) URL." {
				t.Fatalf("unexpected error response: %#v", body)
			}
		})
	}
}

func TestSites_Create_URLValidationAllowsHTTPAndHTTPS(t *testing.T) {
	tests := []string{
		"https://api.example.com/v1",
		"http://localhost:8080/v1",
	}

	for _, siteURL := range tests {
		t.Run(siteURL, func(t *testing.T) {
			_, r := setupSitesTest(t)
			resp := doPostJSON(t, r, "/api/sites", map[string]any{
				"name":     "Valid site",
				"url":      siteURL,
				"platform": "openai",
			})
			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200 for %q, got %d: %s", siteURL, resp.Code, resp.Body.String())
			}
		})
	}
}

func TestSites_Update_URLValidationRejectsNonHTTP(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "javascript", url: "javascript:alert(1)"},
		{name: "data", url: "data:text/plain,unsafe"},
		{name: "file", url: "file:///etc/passwd"},
		{name: "unparseable", url: "not a url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := setupSitesTest(t)
			created := doPostJSON(t, r, "/api/sites", map[string]any{
				"name":     "Existing site",
				"url":      "https://existing.example.com",
				"platform": "openai",
			})
			if created.Code != http.StatusOK {
				t.Fatalf("create fixture: %d: %s", created.Code, created.Body.String())
			}
			var site map[string]any
			if err := json.Unmarshal(created.Body.Bytes(), &site); err != nil {
				t.Fatalf("decode created site: %v", err)
			}

			resp := doPutJSON(t, r, "/api/sites/"+itoa(int64(site["id"].(float64))), map[string]any{
				"url": tt.url,
			})
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", tt.url, resp.Code, resp.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body["error"] != "Invalid url. Expected a valid http(s) URL." {
				t.Fatalf("unexpected error response: %#v", body)
			}
		})
	}
}

func TestSites_Update_URLValidationAllowsHTTPAndHTTPS(t *testing.T) {
	tests := []string{
		"https://updated.example.com/v1",
		"http://localhost:8080/v1",
	}

	for _, siteURL := range tests {
		t.Run(siteURL, func(t *testing.T) {
			_, r := setupSitesTest(t)
			created := doPostJSON(t, r, "/api/sites", map[string]any{
				"name":     "Existing site",
				"url":      "https://existing.example.com",
				"platform": "openai",
			})
			if created.Code != http.StatusOK {
				t.Fatalf("create fixture: %d: %s", created.Code, created.Body.String())
			}
			var site map[string]any
			if err := json.Unmarshal(created.Body.Bytes(), &site); err != nil {
				t.Fatalf("decode created site: %v", err)
			}

			resp := doPutJSON(t, r, "/api/sites/"+itoa(int64(site["id"].(float64))), map[string]any{
				"url": siteURL,
			})
			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200 for %q, got %d: %s", siteURL, resp.Code, resp.Body.String())
			}
		})
	}
}
