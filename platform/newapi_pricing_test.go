package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAPIPricingPreservesEffectiveGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pricing" || r.Header.Get("New-API-User") != "42" || r.Header.Get("Authorization") != "Bearer synthetic-management" || r.Header.Get("X-Synthetic") != "pricing" {
			t.Errorf("missing management context: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"example","quota_type":0,"model_ratio":0,"completion_ratio":2,"enable_groups":["all"]}],"group_ratio":{"default":0.5},"usable_group":{"default":"Standard"}}`))
	}))
	defer server.Close()
	id := 42
	adapter := &NewApiAdapter{BaseAdapter: NewBaseAdapter("newapi")}
	prices, err := adapter.GetPricing(context.Background(), server.URL, "synthetic-management", &id, &ProxyConfig{CustomHeaders: map[string]string{"X-Synthetic": "pricing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prices.Models) != 1 || prices.Models[0].ModelRatio == nil || *prices.Models[0].ModelRatio != 0 || *prices.GroupRatios["default"] != .5 {
		t.Fatalf("%+v", prices)
	}
	unknown, err := ParseNewAPIPricing([]byte(`{"success":true,"data":[{"model_name":"unknown","model_ratio":-1,"enable_groups":["all"]}],"usable_group":{"missing":"Missing multiplier"},"group_ratio":{"missing":-1}}`))
	if err != nil || unknown.Models[0].ModelRatio != nil || unknown.GroupRatios["missing"] != nil {
		t.Fatalf("negative sentinel became a price: %+v %v", unknown, err)
	}
	for _, raw := range []string{`{}`, `{"success":false,"data":[]}`, `{"success":true,"data":{}}`, `{"success":true,"data":null,"usable_group":{}}`, `{"success":true,"data":[{"model_name":""}],"usable_group":{}}`} {
		if _, err := ParseNewAPIPricing([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
