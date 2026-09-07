package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
)

func TestRuntimeExecutor_RefusesMetadataTarget(t *testing.T) {
	testExecutorMetadataGuard(t, func() *http.Client {
		return NewRuntimeExecutor(3 * time.Second).client
	})
}

func TestRuntimeExecutor_StreamRefusesMetadataTarget(t *testing.T) {
	testExecutorMetadataGuard(t, func() *http.Client {
		client := NewRuntimeExecutor(3 * time.Second).streamClient
		client.Timeout = 3 * time.Second
		return client
	})
}

func TestNewStreamTransport_RefusesMetadataTarget(t *testing.T) {
	testExecutorMetadataGuard(t, func() *http.Client {
		return &http.Client{Transport: NewStreamTransport(), Timeout: 3 * time.Second}
	})
}

// Use a fresh process for each environment: net/http caches ProxyFromEnvironment
// on first use. Never inherit a real proxy or forward a metadata probe outside
// this fixture, including when the guard is removed to prove these tests fail.
func testExecutorMetadataGuard(t *testing.T, newClient func() *http.Client) {
	t.Helper()
	const childEnv = "METAPI_TEST_EXECUTOR_PROXY_SCHEME"
	proxyScheme := os.Getenv(childEnv)
	if proxyScheme == "" {
		testName := t.Name()
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+regexp.QuoteMeta(testName)+"$", "-test.v", "-test.timeout=25s")
				cmd.Env = append(os.Environ(), childEnv+"="+scheme)
				output, err := cmd.CombinedOutput()
				t.Logf("%s", output)
				if err != nil {
					t.Fatalf("isolated %s proxy test: %v", scheme, err)
				}
			})
		}
		return
	}
	if proxyScheme != "http" && proxyScheme != "https" {
		t.Fatalf("unexpected proxy scheme %q", proxyScheme)
	}

	const allowedHost = "upstream.test.invalid"
	const allowedBody = "response from loopback origin"
	var originRequests, proxyHTTP, proxyCONNECT, unexpectedTargets atomic.Int32
	originHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		originRequests.Add(1)
		_, _ = io.WriteString(w, allowedBody)
	})
	origin := httptest.NewServer(originHandler)
	t.Cleanup(origin.Close)
	tlsOrigin := httptest.NewTLSServer(originHandler)
	t.Cleanup(tlsOrigin.Close)

	originURL, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(originURL)
	forwardTransport := httpclient.NewTransport(httpclient.Options{Proxy: httpclient.NoProxy})
	t.Cleanup(forwardTransport.CloseIdleConnections)
	forward.Transport = forwardTransport
	proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect && r.Host == allowedHost+":443" {
			proxyCONNECT.Add(1)
			// Tunnel only to this fixture's TLS origin, never r.Host.
			upstream, err := net.DialTimeout("tcp", tlsOrigin.Listener.Addr().String(), 3*time.Second)
			if err != nil {
				t.Errorf("connect to loopback TLS origin: %v", err)
				http.Error(w, "fixture dial failed", http.StatusBadGateway)
				return
			}
			defer upstream.Close()
			conn, buffered, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack proxy connection: %v", err)
				return
			}
			defer conn.Close()
			if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				t.Errorf("write CONNECT response: %v", err)
				return
			}
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(upstream, buffered)
				_ = upstream.Close()
				close(done)
			}()
			_, _ = io.Copy(conn, upstream)
			_ = conn.Close()
			<-done
			return
		}
		if r.Method == http.MethodGet && r.URL.Scheme == "http" && r.URL.Host == allowedHost {
			proxyHTTP.Add(1)
			forward.ServeHTTP(w, r)
			return
		}
		unexpectedTargets.Add(1)
		// Deliberately not a forbidden-target error: reaching the proxy is a
		// guard failure, not successful enforcement by this safety net.
		http.Error(w, "unexpected fixture target", http.StatusBadGateway)
	}))
	if proxyScheme == "https" {
		proxyServer.StartTLS()
	} else {
		proxyServer.Start()
	}
	t.Cleanup(proxyServer.Close)

	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, proxyServer.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
		t.Setenv(key, "")
	}
	client := newClient()
	t.Cleanup(client.CloseIdleConnections)
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = tlsOrigin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	// Both TLS fixtures use httptest's loopback certificate. Trust it while
	// keeping the synthetic origin hostname unresolvable outside the proxy.
	transport.TLSClientConfig.ServerName = "127.0.0.1"

	for _, target := range []string{
		"http://" + allowedHost + "/allowed",
		"https://" + allowedHost + "/allowed",
		origin.URL + "/direct",
	} {
		resp, err := client.Get(target)
		if err != nil {
			t.Fatalf("allowed target %s: %v", target, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != allowedBody {
			t.Fatalf("allowed target %s: status=%d body=%q error=%v", target, resp.StatusCode, body, readErr)
		}
	}
	if proxyHTTP.Load() != 1 || proxyCONNECT.Load() != 1 || originRequests.Load() != 3 {
		t.Fatalf("allowed routing: HTTP=%d CONNECT=%d origins=%d, want 1/1/3", proxyHTTP.Load(), proxyCONNECT.Load(), originRequests.Load())
	}

	for _, scheme := range []string{"http", "https"} {
		for _, host := range []string{
			"169.254.169.254",
			"metadata.google.internal",
			"METADATA.GOOGLE.INTERNAL.",
			"metadata",
			"instance-data",
			"100.100.100.200",
			"[fd00:ec2::254]",
			"[::ffff:169.254.169.254]",
		} {
			t.Run(scheme+"/"+host, func(t *testing.T) {
				resp, err := client.Get(fmt.Sprintf("%s://%s/latest/meta-data/", scheme, host))
				if err == nil {
					resp.Body.Close()
					t.Fatal("expected a forbidden-target error before contacting the proxy")
				}
				if !strings.Contains(err.Error(), "forbidden") {
					t.Errorf("expected a forbidden-target error, got %v", err)
				}
			})
		}
	}
	if got := unexpectedTargets.Load(); got != 0 {
		t.Errorf("proxy received %d forbidden requests, want zero", got)
	}
	t.Logf("loopback fixture: HTTP=%d CONNECT=%d origins=%d forbidden requests received=%d", proxyHTTP.Load(), proxyCONNECT.Load(), originRequests.Load(), unexpectedTargets.Load())
}
