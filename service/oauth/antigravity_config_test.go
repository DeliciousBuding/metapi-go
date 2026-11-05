package oauth

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestAntigravityMissingClientSecretMakesNoExchange(t *testing.T) {
	previous := config.GetSafe()
	copyConfig := *previous
	copyConfig.AntigravityClientSecret = ""
	config.Set(&copyConfig)
	t.Cleanup(func() { config.Set(previous) })
	var calls atomic.Int32
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		calls.Add(1)
		return http.StatusOK, map[string]any{"access_token": "synthetic-access"}
	})
	defer server.Close()
	withAntigravityEndpointSwap(t, server.URL, server.URL, server.URL)
	if _, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "synthetic-refresh", ClientID: antigravityClientID,
	}); err == nil {
		t.Fatal("refresh accepted missing client configuration")
	}
	if _, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "synthetic-code",
	}); err == nil {
		t.Fatal("authorization exchange accepted missing client configuration")
	}
	if calls.Load() != 0 {
		t.Fatal("missing configuration made an upstream call")
	}
}
