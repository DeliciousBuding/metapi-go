package platform

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestCachedStreamTransportKeepsSeparateStablePool(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyResponseHeaderTimeoutSec: 1, ProxyRequestTimeoutSec: 1})
	for _, useUTLS := range []bool{false, true} {
		proxyURL, err := url.Parse("http://stream-pool.invalid:18763")
		if err != nil {
			t.Fatal(err)
		}
		proxy := http.ProxyURL(proxyURL)
		buffered := getCachedTransport(proxy, true, useUTLS)
		stream := getCachedTransportForMode(proxy, true, useUTLS, true)
		t.Cleanup(buffered.CloseIdleConnections)
		t.Cleanup(stream.CloseIdleConnections)
		if stream == buffered || stream.ResponseHeaderTimeout != 0 || buffered.ResponseHeaderTimeout != time.Second {
			t.Fatalf("uTLS=%v: same pool=%v stream header timeout=%v buffered=%v", useUTLS, stream == buffered, stream.ResponseHeaderTimeout, buffered.ResponseHeaderTimeout)
		}
		if newStreamProxyClient(stream).Timeout != 0 || newProxyClient(buffered).Timeout != time.Second {
			t.Fatal("stream/body and buffered request timeout ownership changed")
		}
		if stream.DialContext == nil || stream.Proxy == nil || stream.TLSClientConfig == nil || !stream.TLSClientConfig.InsecureSkipVerify || stream.TLSHandshakeTimeout <= 0 || stream.IdleConnTimeout <= 0 || stream.MaxIdleConnsPerHost <= 0 {
			t.Fatal("stream pool lost dial, TLS, proxy, or keep-alive configuration")
		}
		if useUTLS && stream.DialTLSContext == nil {
			t.Fatal("stream pool lost uTLS dialer")
		}
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				if got := getCachedTransportForMode(proxy, true, useUTLS, true); got != stream {
					t.Error("stream requests did not reuse their transport")
				}
			})
		}
		wg.Wait()
	}
}

func TestDoWithProxyStreamBudgetOutlivesConfiguredTransportLimits(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyResponseHeaderTimeoutSec: 1, ProxyRequestTimeoutSec: 1})
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Host != "stream-budget.invalid" {
			t.Errorf("request did not use explicit HTTP proxy: %s", req.URL)
		}
		select {
		case <-time.After(1200 * time.Millisecond):
		case <-req.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-time.After(1200 * time.Millisecond):
			_, _ = io.WriteString(w, "data: second\n\n")
		case <-req.Context().Done():
		}
	}))
	t.Cleanup(proxyServer.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://stream-budget.invalid/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ProxyConfig{ProxyURL: proxyServer.URL}
	resp, err := DoWithProxyStreamBudget(req.Context(), req, cfg, 2000)
	if err != nil {
		t.Fatalf("1200ms headers should fit 2000ms budget despite 1s buffered limits: %v", err)
	}
	defer resp.Body.Close()
	if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "data: first\n\ndata: second\n\n" {
		t.Fatalf("body past header budget: body=%q error=%v", body, err)
	}
}

func TestDoWithProxyStreamBudgetReusesConnectionsAcrossBudgets(t *testing.T) {
	var connections atomic.Int32
	proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "data: ok\n\n")
	}))
	proxyServer.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	proxyServer.Start()
	t.Cleanup(proxyServer.Close)
	for _, ms := range []int64{500, 1000, 0} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://stream-budget.invalid/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := DoWithProxyStreamBudget(req.Context(), req, &ProxyConfig{ProxyURL: proxyServer.URL}, ms)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read=%v close=%v", readErr, closeErr)
		}
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("three budgets opened %d connections, want one reused connection", got)
	}
}

func TestDoWithProxyStreamBudgetPreservesSiteGuards(t *testing.T) {
	for _, target := range []string{"http://169.254.169.254/latest/meta-data/", "http://metadata.google.internal/"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := DoWithProxyStreamBudget(req.Context(), req, nil, 500)
		if resp != nil || err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatalf("target=%s response=%v error=%v, want site guard refusal", target, resp, err)
		}
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "http://cross-origin.invalid/events", http.StatusFound)
	}))
	t.Cleanup(origin.Close)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DoWithProxyStreamBudget(req.Context(), req, nil, 500)
	if resp != nil || err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("response=%v error=%v, want cross-origin redirect refusal", resp, err)
	}
}
