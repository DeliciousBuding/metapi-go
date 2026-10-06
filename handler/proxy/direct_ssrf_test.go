package proxyhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
)

func TestSendUpstreamRequestRefusesProxiedMetadataTargetBeforeProxyDispatch(t *testing.T) {
	proxyCalls := 0
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer proxyServer.Close()
	request, err := http.NewRequest(http.MethodPost, "http://169.254.169.254/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sendUpstreamRequest(&UpstreamConfig{}, request, &platform.ProxyConfig{ProxyURL: proxyServer.URL}, 0, false)
	if err == nil {
		t.Fatal("proxied metadata request was dispatched")
	}
	if proxyCalls != 0 {
		t.Fatalf("proxy received %d request(s), want zero", proxyCalls)
	}
}
