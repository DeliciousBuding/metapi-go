package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// QuantumNous/new-api v1.0.0-rc.34 (0c76e4dae77a279e015329b7478e6f02d6b62edd)
// controller/checkin.go returns quota_awarded, not reward or the total quota.
func TestNewApiAdapter_CheckinQuotaAwarded(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"rc34_award", `{"success":true,"message":"签到成功","data":{"quota_awarded":1000,"checkin_date":"2026-09-07"}}`, "0.002"},
		{"one_quota_is_not_two_dollars", `{"success":true,"message":"签到成功","data":{"quota_awarded":1}}`, "0.000002"},
		{"explicit_zero", `{"success":true,"message":"签到成功","data":{"quota_awarded":0}}`, "0"},
		{"native_award_precedes_legacy_reward", `{"success":true,"message":"签到成功","data":{"quota_awarded":1000,"reward":20.002}}`, "0.002"},
		{"legacy_reward_keeps_its_units", `{"success":true,"data":{"reward":"0.003"}}`, "0.003"},
		{"total_balance_is_not_an_award", `{"success":true,"message":"签到成功","data":{"balance":20.002,"quota":10001000,"checkin_date":"2026-09-07"}}`, ""},
		{"null_award_is_unknown", `{"success":true,"data":{"quota_awarded":null}}`, ""},
		{"string_award_is_not_official_contract", `{"success":true,"data":{"quota_awarded":"1000"}}`, ""},
		{"negative_award_is_unknown", `{"success":true,"data":{"quota_awarded":-1000}}`, ""},
		{"fractional_quota_is_not_official_contract", `{"success":true,"data":{"quota_awarded":0.5}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/user/checkin" {
					t.Error("unexpected upstream operation")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			userID := 17
			result, err := newApiAdapterUnderTest().Checkin(context.Background(), srv.URL, "test-dashboard-pat", &userID, nil)
			if err != nil || result == nil || !result.Success {
				t.Fatalf("Checkin = %+v, err=%v", result, err)
			}
			if result.Reward != tc.want {
				t.Fatalf("Reward = %q, want %q", result.Reward, tc.want)
			}
		})
	}
}

func TestNewApiAdapter_CheckinQuotaAwardedCookieFallback(t *testing.T) {
	for _, path := range []string{"/api/user/sign_in", "/api/user/checkin"} {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "" {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"success":false,"message":"Unauthorized"}`)
				} else if r.Method == http.MethodPost && r.URL.Path == path && r.Header.Get("Cookie") != "" {
					fmt.Fprint(w, `{"success":true,"message":"签到成功","data":{"quota_awarded":1000,"checkin_date":"2026-09-07"}}`)
				} else {
					fmt.Fprint(w, `{"success":false}`)
				}
			}))
			t.Cleanup(srv.Close)
			userID := 17
			result, err := newApiAdapterUnderTest().Checkin(context.Background(), srv.URL, "session=test-cookie", &userID, nil)
			if err != nil || result == nil || !result.Success || result.Reward != "0.002" {
				t.Fatalf("cookie Checkin = %+v, err=%v; want reward 0.002", result, err)
			}
		})
	}
}
