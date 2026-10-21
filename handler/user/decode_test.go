package user

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserPortalExactDecoder(t *testing.T) {
	for _, body := range []string{
		`{"username":"alice","password":"first","password":"second"}`,
		`{"username":"alice","password":"first","pass\u0077ord":"second"}`,
		`{"username":"alice","Password":"password"}`,
		`{"username":"alice","password":null}`,
		`{"username":"alice","password":"password"} {}`,
		`{"username":"alice","password":"password","ownerUserId":1}`,
		`null`, `[]`, `"text"`, "{", strings.Repeat(" ", 16385),
	} {
		t.Run(body[:min(len(body), 70)], func(t *testing.T) {
			var payload loginPayload
			if err := decode(httptest.NewRequest("POST", "/api/user/auth/login", strings.NewReader(body)), &payload); err == nil {
				t.Fatal("accepted ambiguous input")
			}
		})
	}
	var payload loginPayload
	if err := decode(httptest.NewRequest("POST", "/api/user/auth/login", strings.NewReader(`{"username":"alice","password":"password"}`)), &payload); err != nil || payload.Username != "alice" {
		t.Fatalf("valid payload rejected: %v", err)
	}
}
