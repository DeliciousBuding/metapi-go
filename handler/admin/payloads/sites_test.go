package payloads

import (
	"encoding/json"
	"strings"
	"testing"
)

// sitesRoundTripCase describes a JSON round-trip through a sites payload type.
type sitesRoundTripCase struct {
	name    string
	input   string
	check   func(t *testing.T, decoded any)
	wantErr bool
}

// runSitesDecode runs a table of decode cases against the generic decode target.
func runSitesDecode[T any](t *testing.T, cases []sitesRoundTripCase, newTarget func() *T) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := newTarget()
			err := json.Unmarshal([]byte(tc.input), target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected decode error, got success: %#v", target)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected decode error: %v", err)
			}
			tc.check(t, target)
		})
	}
}

// TestSiteCreatePayload_Decode covers valid/invalid/edge parsing of
// SiteCreatePayload — the request body for POST /api/sites.
func TestSiteCreatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "full valid payload",
			input: `{"name":"Acme","url":"https://acme.example","platform":"sub2api","proxyUrl":"http://p","useSystemProxy":true,"customHeaders":"H: v","customHeadersOverrideRequestHeaders":true,"externalCheckinUrl":"https://c","status":"active","isPinned":true,"sortOrder":3,"globalWeight":1.5,"maxConcurrency":8,"apiEndpoints":[{"url":"https://e1","enabled":true,"sortOrder":1}]}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteCreatePayload)
				if p.Name != "Acme" || p.URL != "https://acme.example" {
					t.Fatalf("name/url mismatch: %+v", p)
				}
				if p.Platform == nil || *p.Platform != "sub2api" {
					t.Fatalf("platform = %#v", p.Platform)
				}
				if p.UseSystemProxy == nil || !*p.UseSystemProxy {
					t.Fatalf("useSystemProxy = %#v", p.UseSystemProxy)
				}
				if p.SortOrder == nil || *p.SortOrder != 3 {
					t.Fatalf("sortOrder = %#v", p.SortOrder)
				}
				if p.GlobalWeight == nil || *p.GlobalWeight != 1.5 {
					t.Fatalf("globalWeight = %#v", p.GlobalWeight)
				}
				if p.MaxConcurrency == nil || *p.MaxConcurrency != 8 {
					t.Fatalf("maxConcurrency = %#v", p.MaxConcurrency)
				}
				if len(p.APIEndpoints) != 1 || p.APIEndpoints[0].URL != "https://e1" {
					t.Fatalf("apiEndpoints = %#v", p.APIEndpoints)
				}
			},
		},
		{
			name: "minimal required fields only",
			input: `{"name":"N","url":"https://n.example"}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteCreatePayload)
				if p.Name != "N" {
					t.Fatalf("name = %q", p.Name)
				}
				if p.Platform != nil {
					t.Fatalf("optional platform should be nil, got %#v", p.Platform)
				}
				if len(p.APIEndpoints) != 0 {
					t.Fatalf("apiEndpoints should be empty, got %#v", p.APIEndpoints)
				}
			},
		},
		{
			name:    "empty object leaves required fields zero",
			input:   `{}`,
			wantErr: false,
			check: func(t *testing.T, d any) {
				p := d.(*SiteCreatePayload)
				if p.Name != "" || p.URL != "" {
					t.Fatalf("name/url should be zero, got %q/%q", p.Name, p.URL)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"name":`,
			wantErr: true,
		},
		{
			name:    "wrong type for name rejected",
			input:   `{"name":123,"url":"x"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for sortOrder rejected",
			input:   `{"name":"x","url":"y","sortOrder":"not-int"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for globalWeight rejected",
			input:   `{"name":"x","url":"y","globalWeight":"abc"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for maxConcurrency rejected",
			input:   `{"name":"x","url":"y","maxConcurrency":"no"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for apiEndpoints rejected",
			input:   `{"name":"x","url":"y","apiEndpoints":"oops"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteCreatePayload { return &SiteCreatePayload{} })
}

// TestSiteCreatePayload_OmitEmptyMarshal verifies optional pointer fields are
// omitted when nil so round-tripping a minimal payload stays minimal.
func TestSiteCreatePayload_OmitEmptyMarshal(t *testing.T) {
	t.Parallel()
	p := SiteCreatePayload{Name: "N", URL: "https://n.example"}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	encoded := string(raw)
	if strings.Contains(encoded, "platform") {
		t.Fatalf("nil platform should be omitted: %s", encoded)
	}
	if strings.Contains(encoded, "apiEndpoints") {
		t.Fatalf("empty apiEndpoints should be omitted: %s", encoded)
	}
	if !strings.Contains(encoded, `"name":"N"`) {
		t.Fatalf("name missing from %s", encoded)
	}
}

// TestSiteUpdatePayload_Decode covers the optional-field PATCH shape.
func TestSiteUpdatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "partial update",
			input: `{"status":"disabled","sortOrder":5,"postRefreshProbeEnabled":true}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteUpdatePayload)
				if p.Status == nil || *p.Status != "disabled" {
					t.Fatalf("status = %#v", p.Status)
				}
				if p.SortOrder == nil || *p.SortOrder != 5 {
					t.Fatalf("sortOrder = %#v", p.SortOrder)
				}
				if p.PostRefreshProbeEnabled == nil || !*p.PostRefreshProbeEnabled {
					t.Fatalf("postRefreshProbeEnabled = %#v", p.PostRefreshProbeEnabled)
				}
				if p.Name != nil {
					t.Fatalf("untouched name should be nil, got %#v", p.Name)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"name":`,
			wantErr: true,
		},
		{
			name:    "wrong type for postRefreshProbeLatencyThresholdMs rejected",
			input:   `{"postRefreshProbeLatencyThresholdMs":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteUpdatePayload { return &SiteUpdatePayload{} })
}

// TestSiteBatchPayload_Decode covers the batch action shape and rejects
// non-integer IDs.
func TestSiteBatchPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid batch",
			input: `{"ids":[1,2,3],"action":"delete"}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteBatchPayload)
				if len(p.IDs) != 3 || p.IDs[0] != 1 || p.IDs[2] != 3 {
					t.Fatalf("ids = %#v", p.IDs)
				}
				if p.Action != "delete" {
					t.Fatalf("action = %q", p.Action)
				}
			},
		},
		{
			name:    "empty object zeroes everything",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteBatchPayload)
				if len(p.IDs) != 0 || p.Action != "" {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"ids":[`,
			wantErr: true,
		},
		{
			name:    "wrong ids element type rejected",
			input:   `{"ids":["x"],"action":"delete"}`,
			wantErr: true,
		},
		{
			name:    "wrong action type rejected",
			input:   `{"ids":[1],"action":5}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteBatchPayload { return &SiteBatchPayload{} })
}

// TestSiteDetectPayload_Decode covers the single-field detect shape.
func TestSiteDetectPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid",
			input: `{"url":"https://detect.example"}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteDetectPayload)
				if p.URL != "https://detect.example" {
					t.Fatalf("url = %q", p.URL)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteDetectPayload)
				if p.URL != "" {
					t.Fatalf("url = %q, want empty", p.URL)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"url":`,
			wantErr: true,
		},
		{
			name:    "wrong url type rejected",
			input:   `{"url":123}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteDetectPayload { return &SiteDetectPayload{} })
}

// TestSiteDisabledModelsPayload_Decode covers the models-list shape.
func TestSiteDisabledModelsPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid models",
			input: `{"models":["gpt-4","claude-3"]}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteDisabledModelsPayload)
				if len(p.Models) != 2 || p.Models[1] != "claude-3" {
					t.Fatalf("models = %#v", p.Models)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				if len(d.(*SiteDisabledModelsPayload).Models) != 0 {
					t.Fatalf("expected empty models")
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"models":[`,
			wantErr: true,
		},
		{
			name:    "wrong model element type rejected",
			input:   `{"models":[1,2]}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteDisabledModelsPayload { return &SiteDisabledModelsPayload{} })
}

// TestSiteImportPayload_Decode covers the nested batch import shape.
func TestSiteImportPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid nested import",
			input: `{"items":[{"name":"S","url":"https://s.example","platform":"sub2api","globalWeight":2.0,"accounts":[{"accessToken":"tok"}]}],"duplicateStrategy":"skip"}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteImportPayload)
				if len(p.Items) != 1 {
					t.Fatalf("items len = %d", len(p.Items))
				}
				if p.Items[0].Name != "S" {
					t.Fatalf("item name = %q", p.Items[0].Name)
				}
				if p.Items[0].GlobalWeight == nil || *p.Items[0].GlobalWeight != 2.0 {
					t.Fatalf("globalWeight = %#v", p.Items[0].GlobalWeight)
				}
				if len(p.Items[0].Accounts) != 1 || p.Items[0].Accounts[0].AccessToken != "tok" {
					t.Fatalf("accounts = %#v", p.Items[0].Accounts)
				}
				if p.DuplicateStrategy != "skip" {
					t.Fatalf("duplicateStrategy = %q", p.DuplicateStrategy)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*SiteImportPayload)
				if len(p.Items) != 0 || p.DuplicateStrategy != "" {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"items":[`,
			wantErr: true,
		},
		{
			name:    "wrong items element type rejected",
			input:   `{"items":"oops"}`,
			wantErr: true,
		},
		{
			name:    "wrong account accessToken type rejected",
			input:   `{"items":[{"name":"S","url":"u","accounts":[{"accessToken":5}]}]}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *SiteImportPayload { return &SiteImportPayload{} })
}

// TestProbeNowBody_Decode covers the probe-now request shape.
func TestProbeNowBody_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid body",
			input: `{"scope":"single","modelName":"gpt-4","latencyThresholdMs":800}`,
			check: func(t *testing.T, d any) {
				p := d.(*ProbeNowBody)
				if p.Scope == nil || *p.Scope != "single" {
					t.Fatalf("scope = %#v", p.Scope)
				}
				if p.ModelName == nil || *p.ModelName != "gpt-4" {
					t.Fatalf("modelName = %#v", p.ModelName)
				}
				if p.LatencyThresholdMs == nil || *p.LatencyThresholdMs != 800 {
					t.Fatalf("latencyThresholdMs = %#v", p.LatencyThresholdMs)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*ProbeNowBody)
				if p.Scope != nil || p.ModelName != nil {
					t.Fatalf("expected nil optionals: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"scope":`,
			wantErr: true,
		},
		{
			name:    "wrong latencyThresholdMs type rejected",
			input:   `{"latencyThresholdMs":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *ProbeNowBody { return &ProbeNowBody{} })
}
