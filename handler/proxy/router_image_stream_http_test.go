package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func routerLargeImageFrame(encoded string) string {
	return routerChatFrame(`{"id":"large-image","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + encoded + `"},"index":0}]},"finish_reason":"stop"}]}`)
}

const routerImageUsageFrame = "data: {\"id\":\"large-image\",\"model\":\"provider-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":0,\"total_tokens\":7}}\n\n"

// Both transports, the imported SQLite graph, provider dispatcher, model
// mapping and actual usage logger participate. Zero completion tokens ensure
// image-only success is based on output evidence, not the usage fallback.
func TestRouterLargeImageHTTP(t *testing.T) {
	for i, provider := range []string{"openrouter", "cerebras"} {
		for _, down := range []directWireFixture{directWireFixtures[0], directWireFixtures[3]} {
			for _, compressed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/gzip=%t", provider, down.provider, compressed), func(t *testing.T) {
					encoded := strings.Repeat("A", (2+i)<<20)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/chat/completions" && r.URL.Path != "/v1/chat/completions" {
							t.Errorf("unexpected upstream path %s", r.URL.Path)
						}
						w.Header().Set("Content-Type", "text/event-stream")
						var target io.Writer = w
						if compressed {
							w.Header().Set("Content-Encoding", "gzip")
							writer := gzip.NewWriter(w)
							defer writer.Close()
							target = writer
						}
						_, _ = io.WriteString(target, routerLargeImageFrame(encoded)+routerImageUsageFrame+"data: [DONE]\n\n")
					}))
					defer upstream.Close()
					installAxonHubEndpointFixture(t, provider, upstream.URL, nil)
					config.UpdateRuntime(func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })
					logs := make(chan proxy.ProxyLogEntry, 1)
					getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
					downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						ctx := auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy})
						ctx = context.WithValue(ctx, downstreamResponseModelKey{}, "public-image-model")
						down.handler(w, r.WithContext(ctx))
					}))
					defer downstream.Close()
					path, request := down.path, strings.TrimSuffix(down.request, "}")+`,"stream":true}`
					if down.provider == "gemini" {
						path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						request = down.request
					}
					resp, err := http.Post(downstream.URL+path, "application/json", strings.NewReader(request))
					if err != nil {
						t.Fatal(err)
					}
					out, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if resp.StatusCode != http.StatusOK || bytes.Contains(out, []byte("upstream_error")) || !bytes.Contains(out, []byte("public-image-model")) {
						t.Fatalf("image stream/model mapping failed: status=%d bytes=%d tail=%q", resp.StatusCode, len(out), out[max(len(out)-350, 0):])
					}
					copies := 2 // Native Chat preserves images plus normalized content.
					if down.provider == "gemini" {
						copies = 1
					}
					if bytes.Count(out, []byte(encoded)) != copies {
						t.Fatalf("image truncated or duplicated: bytes=%d", len(out))
					}
					select {
					case entry := <-logs:
						if entry.Status != "success" || entry.FirstOutputLatencyMs == nil || entry.PromptTokens == nil || *entry.PromptTokens != 7 || entry.CompletionTokens == nil || *entry.CompletionTokens != 0 || entry.TotalTokens == nil || *entry.TotalTokens != 7 {
							t.Fatalf("image output or original tail usage lost: %+v", entry)
						}
						if entry.UpstreamReportedModel == nil || *entry.UpstreamReportedModel != "provider-model" {
							t.Fatalf("public mapping replaced original upstream model: %+v", entry)
						}
					case <-time.After(time.Second):
						t.Fatal("missing actual usage log")
					}
				})
			}
		}
	}
}

func TestRouterLargeImageStreamFailureBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int
		trailer string
		want    streamOutcome
	}{
		{"missing-terminal", 8 << 20, "", streamEndedUpstreamFault},
		{"raw-limit", 1 << 20, "data: [DONE]\n\n", streamEndedTruncated},
		{"expanded-output-limit", 3 << 20, "data: [DONE]\n\n", streamEndedTruncated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := config.GetSafe()
			config.Set(&config.Config{ProxyMaxStreamResponseBytes: tc.limit})
			t.Cleanup(func() { config.Set(previous) })
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				// Earlier authoritative usage survives a later byte-limit failure.
				_, _ = io.WriteString(w, routerImageUsageFrame+routerLargeImageFrame(strings.Repeat("A", 2<<20))+tc.trailer)
			}))
			defer upstream.Close()
			resp, err := upstream.Client().Get(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			r := makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"public-image-model","stream":true}`)
			r = r.WithContext(context.WithValue(r.Context(), downstreamResponseModelKey{}, "public-image-model"))
			r = r.WithContext(withDirectProviderWire(r.Context(), &directProviderWire{Profile: "openrouter"}))
			w := httptest.NewRecorder()
			usage, outcome, _ := handleStreamUpstreamForEndpoint(w, r, resp, 0, "/v1/chat/completions", "provider-model", messages.Options{}, nil, true)
			if outcome != tc.want || !strings.Contains(w.Body.String(), "upstream_error") || usage.PromptTokens != 7 || usage.TotalTokens != 7 {
				t.Fatalf("failure outcome=%s want=%s usage=%+v bytes=%d", outcome, tc.want, usage, w.Body.Len())
			}
		})
	}
}

func TestRouterFramedAnalysisAvoidsImagePendingBuffer(t *testing.T) {
	input := routerLargeImageFrame(strings.Repeat("A", 2<<20)) + routerImageUsageFrame + "data: [DONE]\n\n"
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(input)), newDirectRouterChatStream(), 8<<20)
	reader.frameLimit = 3 << 20
	reader.outputAnalysis = newIncrementalSseAnalyzer()
	defer reader.Close()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
	for _, analysis := range []*incrementalSseAnalyzer{reader.original, reader.outputAnalysis} {
		result := analysis.Result()
		if result.PendingBytes != 0 || result.DroppedOversizedEvent || !result.HasGeneratedOutput || !result.HasDoneMarker || result.Usage.TotalTokens != 7 {
			t.Fatalf("already framed image was rebuffered or dropped: %+v", result)
		}
	}
}

func TestRouterImageStreamCancelAndIdle(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelRequest), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, routerLargeImageFrame(strings.Repeat("A", 2<<20))+routerImageUsageFrame)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer upstream.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			resp, err := upstream.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			idle := &streamIdleBody{ReadCloser: resp.Body}
			// Start after image parsing, so this checks the waiting-read path
			// rather than racing the fixture's JSON decoding against a timer.
			idle.guard = newStreamIdleGuard(5*time.Second, idle.closeUnderlying)
			reader := newProtocolBridgeBody(idle, newDirectRouterChatStream(), 8<<20)
			reader.frameLimit = 3 << 20
			defer reader.Close()
			buf := make([]byte, 4096)
			for reader.original.Result().Usage.TotalTokens != 7 {
				if _, err := reader.Read(buf); err != nil {
					t.Fatal(err)
				}
			}
			if cancelRequest {
				cancel()
			} else {
				idle.guard.stop()
				idle.guard = newStreamIdleGuard(20*time.Millisecond, idle.closeUnderlying)
			}
			remaining, err := io.ReadAll(reader)
			if err == nil || bytes.Contains(remaining, []byte("[DONE]")) {
				t.Fatalf("interruption became successful: %v", err)
			}
			if cancelRequest && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation cause: %v", err)
			}
			if !cancelRequest && !idle.guard.fired.Load() {
				t.Fatalf("idle guard did not own termination: %v", err)
			}
			if result := reader.original.Result(); result.Usage.TotalTokens != 7 || result.HasDoneMarker {
				t.Fatalf("interruption lost usage or invented terminal: %+v", result)
			}
		})
	}
}

func TestStreamImageEvidenceVersusMetadata(t *testing.T) {
	previous := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{ProxyEmptyContentFailEnabled: true})
	t.Cleanup(func() { config.SetRuntime(previous) })
	for _, output := range []bool{false, true} {
		analyzer := newIncrementalSseAnalyzer()
		analyzer.Push([]byte(routerChatFrame(`{"choices":[{"delta":{"role":"assistant"}}]}`) + routerImageUsageFrame))
		if output {
			analyzer.Push([]byte(routerLargeImageFrame("AA==")))
		}
		result := analyzer.Result()
		verdict := judgeStreamContent(http.StatusOK, result, 0, false)
		if verdict.Failed == output {
			t.Fatalf("metadata/image distinction lost: output=%t result=%+v verdict=%+v", output, result, verdict)
		}
	}
}
