package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
)

const openRouterImageFixture = "data:image/png;base64,iVBORw0KGgo="

func TestDirectOpenRouterImagePrepare(t *testing.T) {
	credentialURL := (&url.URL{Scheme: "https", Host: "example.test", Path: "/reference.png", User: url.UserPassword("user", "password")}).String()
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		for _, refs := range []string{`"image":"` + openRouterImageFixture + `"`, `"image":["` + openRouterImageFixture + `"]`, `"images":["` + openRouterImageFixture + `"]`, `"images":[{"image_url":"` + openRouterImageFixture + `"}]`} {
			wire, ct, err := prepareDirectOpenRouterImage(path, "application/json", []byte(`{"model":"provider-image","prompt":"sunset","n":2,"size":"1024x1024","quality":"high","background":"transparent","output_format":"webp","output_compression":80,"seed":9007199254740993,"response_format":"url",`+refs+`}`))
			if err != nil || ct != "application/json" {
				t.Fatalf("prepare: %s %s %v", path, refs, err)
			}
			var body map[string]json.RawMessage
			_ = json.Unmarshal(wire.Body, &body)
			for key, want := range map[string]string{"model": `"provider-image"`, "prompt": `"sunset"`, "n": "2", "size": `"1024x1024"`, "quality": `"high"`, "background": `"transparent"`, "output_format": `"webp"`, "output_compression": "80", "seed": "9007199254740993"} {
				if string(body[key]) != want {
					t.Errorf("%s=%s want=%s", key, body[key], want)
				}
			}
			if string(body["input_references"]) != `[{"image_url":{"url":"`+openRouterImageFixture+`"},"type":"image_url"}]` {
				t.Fatalf("wrong references: %s", body["input_references"])
			}
			if wire.ImageResponseFormat != "url" || len(body["response_format"]) != 0 || len(body["image"]) != 0 || len(body["images"]) != 0 {
				t.Fatal("client output format or image aliases leaked upstream")
			}
		}
	}
	for _, body := range []string{
		`{"model":"m","prompt":"p","mask":"x"}`, `{"model":"m","prompt":"p","stream":true}`,
		`{"model":"m","prompt":"p","moderation":"low"}`, `{"model":"m","prompt":"p","style":"vivid"}`, `{"model":"m","prompt":"p","user":"alice"}`,
		`{"model":"m","prompt":"p","partial_images":1}`, `{"model":"m","prompt":"p","n":0}`, `{"model":"m","prompt":"p","output_compression":101}`,
		`{"model":"m","prompt":"p","response_format":"remote_url"}`, `{"model":"m","prompt":"p","image":null}`,
		`{"model":"m","prompt":"p","image":"` + credentialURL + `"}`, `{"model":"m","prompt":"p","images":[{"image_url":"` + openRouterImageFixture + `","detail":"high"}]}`,
		`{"model":"m","prompt":"p","image":"` + openRouterImageFixture + `","images":["` + openRouterImageFixture + `"]}`,
	} {
		if _, _, err := prepareDirectOpenRouterImage("/v1/images/generations", "application/json", []byte(body)); err == nil {
			t.Errorf("accepted unsupported request %s", body)
		}
	}
	if _, _, err := prepareDirectOpenRouterImage("/v1/images/edits", "application/json", []byte(`{"model":"m","prompt":"p"}`)); err == nil {
		t.Fatal("edit without references accepted")
	}
	for _, count := range []int{16, 17} {
		refs := make([]string, count)
		for i := range refs {
			refs[i] = openRouterImageFixture
		}
		body, _ := json.Marshal(map[string]any{"model": "m", "prompt": "p", "images": refs})
		_, _, err := prepareDirectOpenRouterImage("/v1/images/edits", "application/json", body)
		if (err != nil) != (count > 16) {
			t.Fatalf("reference limit count=%d err=%v", count, err)
		}
	}
	oversize := "data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(int(defaultMaxMultipartFileBytes))+4)
	if err := validateOpenRouterImageReference(oversize); err == nil {
		t.Fatal("oversized reference accepted")
	}
}

func TestDirectOpenRouterImageRemoteReferences(t *testing.T) {
	for _, reference := range []string{"https://example.test/ref.png?signature=a%2Fb%2Bcd&expires=123", "http://example.test/ref.jpg", "HTTPS://example.test/ref.webp"} {
		body, _ := json.Marshal(map[string]any{"model": "m", "prompt": "p", "image": reference})
		wire, _, err := prepareDirectOpenRouterImage("/v1/images/edits", "application/json", body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			References []struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"input_references"`
		}
		if json.Unmarshal(wire.Body, &payload) != nil || len(payload.References) != 1 || payload.References[0].ImageURL.URL != reference {
			t.Fatalf("remote reference changed: %s", wire.Body)
		}
	}
	for _, reference := range []string{"/ref.png", "file:///tmp/ref.png", "ftp://example.test/ref.png", "https://user@example.test/ref.png", "https://", "https://example.test/ref.png\r\nX-Api-Key: value", "https://example.test/a b.png", "https://:443/a.png"} {
		if validateOpenRouterImageReference(reference) == nil {
			t.Errorf("invalid remote reference accepted: %q", reference)
		}
	}
}

func TestDirectOpenRouterImageMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range map[string]string{"model": "m", "prompt": "p", "response_format": "b64_json", "output_format": "jpeg", "output_compression": "75"} {
		_ = writer.WriteField(k, v)
	}
	for _, b := range [][]byte{{137, 80, 78, 71, 13, 10, 26, 10}, {255, 216, 255, 224}} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="image[]"; filename="ref.png"`)
		h.Set("Content-Type", "image/png")
		part, _ := writer.CreatePart(h)
		_, _ = part.Write(b)
	}
	_ = writer.Close()
	wire, ct, err := prepareDirectOpenRouterImage("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
	if err != nil || ct != "application/json" {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(wire.Body, &payload)
	if string(payload["output_compression"]) != "75" || string(payload["output_format"]) != `"jpeg"` || wire.ImageResponseFormat != "b64_json" {
		t.Fatalf("formats confused: %s", wire.Body)
	}
	if !bytes.Contains(wire.Body, []byte("data:image/png;base64,iVBORw0KGgo=")) || !bytes.Contains(wire.Body, []byte("data:image/jpeg;base64,/9j/4A==")) {
		t.Fatalf("references lost: %s", wire.Body)
	}
}

func TestDirectOpenRouterImageHTTP(t *testing.T) {
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		if r.Method != "POST" || r.URL.Path != "/api/v1/images" || r.Header.Get("Authorization") != "Bearer fixture-image-key" {
			t.Errorf("bad endpoint/auth: %s %s", r.Method, r.URL)
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if mediaString(body, "model") != "m" || len(body["input_references"]) == 0 || len(body["response_format"]) != 0 {
			t.Errorf("bad native body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "original")
		_, _ = io.WriteString(w, `{"created":9007199254740993,"data":[{"b64_json":"iVBORw0KGgo","media_type":"image/png"},{"b64_json":"/9j/4A==","media_type":"image/jpeg"}],"usage":{"prompt_tokens":4,"completion_tokens":24,"total_tokens":28,"prompt_tokens_details":{"cached_tokens":1,"image_tokens":2},"completion_tokens_details":{"reasoning_tokens":3}},"opaque":9007199254740993}`)
	}))
	defer upstream.Close()
	for _, format := range []string{"", "b64_json", "url"} {
		wire, _, err := prepareDirectOpenRouterImage("/v1/images/edits", "application/json", []byte(`{"model":"m","prompt":"p","image":"`+openRouterImageFixture+`","response_format":"`+format+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest("POST", upstream.URL+"/api/v1/images", bytes.NewReader(wire.Body))
		req.Header = wire.Headers.Clone()
		req.Header.Set("Authorization", "Bearer fixture-image-key")
		resp, err := sendDirectOpenRouterImageRequest(&UpstreamConfig{}, req, nil, 1000, wire.ImageResponseFormat)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("ETag") != "" || !bytes.Contains(data, []byte("9007199254740993")) {
			t.Fatalf("response: %d %s", resp.StatusCode, data)
		}
		var payload map[string]json.RawMessage
		_ = json.Unmarshal(data, &payload)
		var images []map[string]json.RawMessage
		_ = json.Unmarshal(payload["data"], &images)
		if mediaString(payload, "output_format") != "png" || mediaString(images[1], "media_type") != "image/jpeg" {
			t.Fatal("first-image output format or per-image media type changed")
		}
		if (len(images[0]["url"]) > 0) != (format != "b64_json") || (len(images[0]["b64_json"]) > 0) != (format != "url") {
			t.Fatalf("response format %q not honored: %s", format, data)
		}
		if format != "b64_json" && !strings.HasPrefix(mediaString(images[0], "url"), "data:image/png;base64,") {
			t.Fatal("output URL is not an embedded image")
		}
		usage := ParseUsageFromBody(data)
		if !usage.Found || usage.PromptTokens != 4 || usage.CompletionTokens != 24 || usage.TotalTokens != 28 || usage.CacheReadTokens != 1 || usage.ReasoningTokens != 3 {
			t.Fatalf("usage lost: %+v", usage)
		}
		if EstimateBillingCostFromUsage("m", "openai", usage).EstimatedCost <= 0 {
			t.Fatal("normalized usage did not reach billing parser")
		}
	}
	if posts.Load() != 3 {
		t.Fatalf("unexpected request replay: %d", posts.Load())
	}
}

func TestDirectOpenRouterImageFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
		usage  bool
	}{
		{"http-error", 429, `{"error":{"message":"busy"},"usage":{"total_tokens":5}}`, 429, true},
		{"http-error-invalid-usage", 429, `{"error":{"message":"busy"},"usage":{"total_tokens":2.5}}`, 429, false},
		{"http-error-html", 503, `<html>Service unavailable</html>`, 503, false},
		{"application-error", 200, `{"error":{"message":"failed","code":13},"usage":{"total_tokens":5}}`, 502, true},
		{"invalid-image", 200, `{"data":[{"b64_json":"??"}],"usage":{"total_tokens":5}}`, 502, true},
		{"invalid-media", 200, `{"data":[{"b64_json":"aGk=","media_type":"text/html"}],"usage":{"total_tokens":5}}`, 502, true},
		{"missing-data", 200, `{"data":[],"usage":{"total_tokens":5}}`, 502, true},
		{"invalid-created", 200, `{"created":"yesterday","data":[{"b64_json":"aGk="}],"usage":{"total_tokens":5}}`, 502, true},
		{"invalid-json", 200, `{`, 502, false},
		{"invalid-usage", 200, `{"data":[{"b64_json":"aGk="}],"usage":{"total_tokens":2.5}}`, 502, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			req, _ := http.NewRequest("POST", server.URL, nil)
			resp, err := sendDirectOpenRouterImageRequest(&UpstreamConfig{}, req, nil, 1000, "b64_json")
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want || ParseUsageFromBody(data).Found != tc.usage {
				t.Fatalf("failure accounting: %d %s", resp.StatusCode, data)
			}
		})
	}
	previous := config.Get()
	copy := *previous
	copy.ProxyMaxBufferedResponseBytes = 128
	config.Set(&copy)
	defer config.Set(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 129)) }))
	defer server.Close()
	req, _ := http.NewRequest("POST", server.URL, nil)
	_, err := sendDirectOpenRouterImageRequest(&UpstreamConfig{}, req, nil, 1000, "")
	if !errors.Is(err, proxy.ErrBufferedResponseBodyTooLarge) {
		t.Fatalf("buffer limit was bypassed: %v", err)
	}
	copy.ProxyMaxBufferedResponseBytes = 200
	config.Set(&copy)
	expanded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+strings.Repeat("AAAA", 20)+`"}],"usage":{"total_tokens":7}}`)
	}))
	defer expanded.Close()
	req, _ = http.NewRequest("POST", expanded.URL, nil)
	resp, err := sendDirectOpenRouterImageRequest(&UpstreamConfig{}, req, nil, 1000, "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 502 || ParseUsageFromBody(data).TotalTokens != 7 {
		t.Fatalf("expanded response lost observed usage: %d %s", resp.StatusCode, data)
	}
}

func TestDirectOpenRouterImageFirstKnownMediaType(t *testing.T) {
	var payload map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"data":[{"b64_json":"iVBORw0KGgo="},{"b64_json":"/9j/4A==","media_type":" image/jpeg "}]}`), &payload)
	if err := normalizeDirectOpenRouterImage(payload, "b64_json"); err != nil || mediaString(payload, "output_format") != "jpeg" {
		t.Fatalf("missing media hint overrode first known format: %s %v", payload["output_format"], err)
	}
}

func TestDirectOpenRouterImageConcurrentCancellation(t *testing.T) {
	var canceled atomic.Int32
	started := make([]chan struct{}, 8)
	for i := range started {
		started[i] = make(chan struct{})
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index, _ := strconv.Atoi(r.URL.Query().Get("index"))
		close(started[index])
		if r.URL.Query().Get("cancel") == "true" {
			<-r.Context().Done()
			canceled.Add(1)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk=","media_type":"image/png"}],"usage":{"total_tokens":7}}`)
	}))
	defer upstream.Close()
	var wg sync.WaitGroup
	failures := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			shouldCancel := i%2 == 0
			if shouldCancel {
				go func() {
					select {
					case <-started[i]:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/images?cancel=%v&index=%d", upstream.URL, shouldCancel, i), nil)
			format := "b64_json"
			if i%3 == 0 {
				format = "url"
			}
			resp, err := sendDirectOpenRouterImageRequest(&UpstreamConfig{}, req, nil, 1000, format)
			if shouldCancel {
				if err == nil {
					failures <- "cancellation was ignored"
					_ = resp.Body.Close()
				}
				return
			}
			if err != nil {
				failures <- err.Error()
				return
			}
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var payload map[string]json.RawMessage
			_ = json.Unmarshal(data, &payload)
			var images []map[string]json.RawMessage
			_ = json.Unmarshal(payload["data"], &images)
			if len(images) != 1 || (len(images[0]["url"]) > 0) != (format == "url") {
				failures <- string(data)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	deadline := time.Now().Add(time.Second)
	for canceled.Load() != 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if canceled.Load() != 4 {
		t.Fatalf("upstream cancellations=%d", canceled.Load())
	}
}
