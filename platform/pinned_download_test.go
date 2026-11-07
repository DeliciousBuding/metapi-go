package platform

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestPinnedHTTPSDownloadUsesOriginalSNIThroughConnect(t *testing.T) {
	for _, mode := range []string{"trusted", "untrusted", "wrong-host"} {
		t.Run(mode, func(t *testing.T) {
			var downloads, tunnels atomic.Int32
			var observedSNI atomic.Value
			cdn := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				if r.Host != "example.com" || r.Header.Get("Range") != "bytes=0-3" {
					t.Error("HTTPS download lost original Host/Range")
				}
				for _, key := range []string{"Authorization", "Cookie", "X-Private", "Proxy-Authorization"} {
					if r.Header.Get(key) != "" {
						t.Errorf("HTTPS download retained %s", key)
					}
				}
				_, _ = io.WriteString(w, "fixture")
			}))
			cdn.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				observedSNI.Store(hello.ServerName)
				return nil, nil
			}}
			cdn.StartTLS()
			defer cdn.Close()
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tunnels.Add(1)
				if r.Method != "CONNECT" || r.Host != "93.184.216.34:443" || r.Header.Get("Authorization") != "" {
					t.Error("HTTPS proxy did not receive a credential-free pinned target")
					w.WriteHeader(400)
					return
				}
				// Only this controlled proxy maps the public literal to loopback.
				// The downloader never connects to a real public fixture target.
				upstream, err := net.Dial("tcp", cdn.Listener.Addr().String())
				if err != nil {
					t.Error(err)
					w.WriteHeader(502)
					return
				}
				defer upstream.Close()
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				_ = rw.Flush()
				done := make(chan struct{})
				go func() {
					_, _ = io.Copy(upstream, rw.Reader)
					_ = upstream.Close()
					close(done)
				}()
				_, _ = io.Copy(conn, upstream)
				_ = conn.Close()
				<-done
			}))
			defer proxy.Close()
			proxyURL, _ := url.Parse(proxy.URL)
			proxyFunc := http.ProxyURL(proxyURL)
			base := getCachedTransportForMode(proxyFunc, false, false, true).Clone()
			roots := x509.NewCertPool()
			if mode != "untrusted" {
				roots.AddCert(cdn.Certificate())
			}
			base.TLSClientConfig = &tls.Config{RootCAs: roots}
			key := transportCacheKey(proxyFunc, false, false) + "|stream"
			transportCache.Store(key, base)
			t.Cleanup(func() { transportCache.Delete(key); base.CloseIdleConnections() })
			sni := "example.com"
			if mode == "wrong-host" {
				sni = "other.fixture.invalid"
			}
			req, _ := http.NewRequestWithContext(t.Context(), "GET", "https://93.184.216.34/movie", nil)
			req.Host = "example.com"
			req.Header.Set("Range", "bytes=0-3")
			req.Header.Set("Authorization", "Bearer fixture-private")
			resp, err := DoPinnedDownloadStreamBudget(t.Context(), req, &ProxyConfig{ProxyURL: proxy.URL, InsecureSkipTLS: true, UseUTLS: true, CustomHeaders: map[string]string{"X-Private": "fixture-private"}}, sni, 1000)
			if tunnels.Load() != 1 || observedSNI.Load() != sni {
				t.Fatal("HTTPS download did not preserve CONNECT target and original SNI")
			}
			if mode != "trusted" {
				if err == nil {
					_ = resp.Body.Close()
					t.Fatal("factory TLS bypass accepted an untrusted certificate or wrong hostname")
				}
				if downloads.Load() != 0 {
					t.Fatal("failed TLS validation sent an HTTP download")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "fixture" || downloads.Load() != 1 {
				t.Fatalf("trusted HTTPS download failed: %s %v", body, err)
			}
			if base.TLSClientConfig.ServerName != "" {
				t.Fatal("download mutated the shared transport TLS configuration")
			}
		})
	}
}

func pinnedTunnelTestProxy(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "93.184.216.34:80" {
			t.Errorf("proxy target was not pinned: %s %s", r.Method, r.Host)
			w.WriteHeader(400)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		for {
			req, err := http.ReadRequest(rw.Reader)
			if err != nil {
				return
			}
			response := httptest.NewRecorder()
			handler(response, req)
			result := response.Result()
			result.ContentLength = int64(response.Body.Len())
			result.Header.Set("Content-Length", fmt.Sprintf("%d", response.Body.Len()))
			_ = result.Write(conn)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPinnedDownloadKeepsHostAndRangeWithoutCredentialsOrTLSBypass(t *testing.T) {
	var calls atomic.Int32
	proxy := pinnedTunnelTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Host != "" || r.Host != "cdn.fixture.invalid" || r.Header.Get("Range") != "bytes=0-3" {
			t.Error("proxy received unpinned host or lost original Host/Range")
		}
		for _, key := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Private"} {
			if r.Header.Get(key) != "" {
				t.Errorf("download retained %s", key)
			}
		}
		if r.URL.Path == "/first" {
			w.Header().Set("Location", "/next")
			w.WriteHeader(302)
			return
		}
		if r.URL.Path == "/next" {
			w.Header().Set("Location", "http://cdn.fixture.invalid/final")
			w.WriteHeader(302)
			return
		}
		_, _ = io.WriteString(w, "fixture")
	})
	defer proxy.Close()
	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://93.184.216.34/first", nil)
	req.Host = "cdn.fixture.invalid"
	req.Header.Set("Range", "bytes=0-3")
	req.Header.Set("Authorization", "Bearer fixture-private")
	req.Header.Set("Cookie", "fixture-private")
	resp, err := DoPinnedDownloadStreamBudget(t.Context(), req, &ProxyConfig{ProxyURL: proxy.URL, InsecureSkipTLS: true, UseUTLS: true, CustomHeaders: map[string]string{"X-Private": "fixture-private"}}, "cdn.fixture.invalid", 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "fixture" || calls.Load() != 3 {
		t.Fatalf("pinned download failed: %s %v", body, err)
	}
	transport := resp.Body.(*pinnedDownloadBody).transport
	if transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.ServerName != "cdn.fixture.invalid" || transport.DialTLSContext != nil || transport.ResponseHeaderTimeout != 0 {
		t.Fatal("download inherited factory TLS bypass/fingerprint or competing header timer")
	}
}

func TestPinnedDownloadRejectsRedirectsThatChangeOriginOrAddCredentials(t *testing.T) {
	userinfo := &url.URL{Scheme: "http", Host: "93.184.216.34", Path: "/private", User: url.UserPassword("fixture-user", "fixture-password")}
	for _, target := range []string{"http://127.0.0.1/private", userinfo.String(), "http://other.fixture.invalid/private", "https://93.184.216.34/private", "http://cdn.fixture.invalid:8080/private"} {
		t.Run(target, func(t *testing.T) {
			var calls atomic.Int32
			proxy := pinnedTunnelTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", target)
				w.WriteHeader(302)
			})
			defer proxy.Close()
			req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://93.184.216.34/movie", nil)
			req.Host = "cdn.fixture.invalid"
			if resp, err := DoPinnedDownloadStreamBudget(t.Context(), req, &ProxyConfig{ProxyURL: proxy.URL}, "cdn.fixture.invalid", 1000); err == nil {
				resp.Body.Close()
				t.Fatal("unsafe redirected download accepted")
			}
			if calls.Load() != 1 {
				t.Fatal("unsafe redirect made another request")
			}
		})
	}
}
