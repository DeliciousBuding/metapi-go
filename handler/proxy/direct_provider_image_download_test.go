package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
)

func TestProviderImageURLDownloadPinsPublicIPAndIsolatesSupplierCredentials(t *testing.T) {
	for _, mode := range []string{"success", "http-error", "redirect", "private-target", "byte-limit", "expanded-limit"} {
		t.Run(mode, func(t *testing.T) {
			previous := mediaTargetAllowPrivate
			mediaTargetAllowPrivate = false
			t.Cleanup(func() { mediaTargetAllowPrivate = previous })
			var submissions, downloads atomic.Int32
			image := []byte{0, 1, 2, 255, 13, 10}
			if mode == "byte-limit" || mode == "expanded-limit" {
				previousConfig := config.GetSafe()
				bounded := *previousConfig
				bounded.ProxyMaxBufferedResponseBytes = 2048
				config.Set(&bounded)
				t.Cleanup(func() { config.Set(previousConfig) })
				image = bytes.Repeat([]byte{255}, 2049)
				if mode == "expanded-limit" {
					// Download fits; base64 plus the native envelope does not.
					image = bytes.Repeat([]byte{255}, 1536)
				}
			}
			address := "http://93.184.216.34/image?signature=synthetic-private"
			if mode == "private-target" {
				address = "http://127.0.0.1/private"
			}
			// The controlled proxy terminates the public-literal CONNECT. No
			// real public host or supplier DNS receives any network traffic.
			proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					submissions.Add(1)
					if r.URL.Host != "supplier.fixture.invalid" || r.URL.Path != "/images/generations" || r.Header.Get("Authorization") != "Bearer synthetic-image-secret" {
						t.Error("supplier submission changed its destination or credential")
					}
					w.Header().Set("Content-Type", "application/json")
					native := fmt.Sprintf(`{"created":1,"data":[{"url":%q}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"extension":9007199254740993}`, address)
					_ = json.NewEncoder(w).Encode(map[string]any{"task_status": "DONE", "native": native, "meter": map[string]int{"input": 7, "output": 2, "total": 9}})
					return
				}
				if r.Method != http.MethodConnect || r.Host != "93.184.216.34:80" {
					t.Errorf("download did not CONNECT to its pinned public IP: %s %s", r.Method, r.Host)
					w.WriteHeader(400)
					return
				}
				for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Signing-Key", "X-Vendor-Private"} {
					if r.Header.Get(name) != "" {
						t.Errorf("CONNECT leaked supplier header %s", name)
					}
				}
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				_ = rw.Flush()
				request, err := http.ReadRequest(rw.Reader)
				if err != nil {
					t.Error(err)
					return
				}
				defer request.Body.Close()
				downloads.Add(1)
				if request.Host != "93.184.216.34" || request.URL.RequestURI() != "/image?signature=synthetic-private" {
					t.Error("download lost its original Host or signed URL")
				}
				for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Signing-Key", "X-Vendor-Private"} {
					if request.Header.Get(name) != "" {
						t.Errorf("image CDN leaked supplier header %s", name)
					}
				}
				switch mode {
				case "redirect":
					_, _ = rw.WriteString("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1/private\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				case "http-error":
					_, _ = rw.WriteString("HTTP/1.1 503 Unavailable\r\nContent-Length: 21\r\nConnection: close\r\n\r\nsynthetic-private-cdn")
				default:
					_, _ = fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: image/png\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(image))
					_, _ = rw.Write(image)
				}
				_ = rw.Flush()
			}))
			defer proxyServer.Close()
			op := imageOperationFixture(t, "http://supplier.fixture.invalid", imageOperationModule)
			op.proxyConfig = &platform.ProxyConfig{ProxyURL: proxyServer.URL, InsecureSkipTLS: true, UseUTLS: true, ClearanceCookie: "synthetic-cookie", CustomHeaders: map[string]string{"X-Vendor-Private": "synthetic-private-header"}}
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://supplier.fixture.invalid/images/generations", strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer synthetic-image-secret")
			request.Header.Set("X-Signing-Key", "synthetic-signature")
			response, err := op.run(request)
			wantDownloads := int32(1)
			if mode == "private-target" {
				wantDownloads = 0
			}
			if submissions.Load() != 1 || downloads.Load() != wantDownloads || op.wire.DriverImageUsage == nil || op.wire.DriverImageUsage.TotalTokens != 9 {
				t.Fatalf("download replayed submission or changed supplier usage: submissions=%d downloads=%d usage=%+v", submissions.Load(), downloads.Load(), op.wire.DriverImageUsage)
			}
			if mode != "success" {
				if err == nil || directMediaFailureUsage(err).TotalTokens != 9 || strings.Contains(err.Error(), "synthetic-private") {
					t.Fatalf("unsafe/failed download succeeded or exposed private material: %v", err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("pinned image download failed: %v", err)
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			var output struct {
				Data []struct {
					Base64 string `json:"b64_json"`
				} `json:"data"`
			}
			if readErr != nil || json.Unmarshal(body, &output) != nil || len(output.Data) != 1 || !bytes.Contains(body, []byte("9007199254740993")) {
				t.Fatalf("materialization changed native JSON or precise extensions: %s", body)
			}
			decoded, decodeErr := base64.StdEncoding.DecodeString(output.Data[0].Base64)
			if decodeErr != nil || !bytes.Equal(decoded, image) {
				t.Fatal("image materialization changed downloaded file bytes")
			}
		})
	}
}
