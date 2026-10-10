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
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func mediaTestRequest(t *testing.T, ctx context.Context, endpoint, profile, path, contentType string, body []byte) *http.Request {
	t.Helper()
	wire, contentType, err := prepareDirectMediaProfile(profile, path, contentType, body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(wire.Body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = wire.Headers.Clone()
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer selected-key")
	return req
}

func mediaTestOperation() directMediaOperation {
	return directMediaOperation{cfg: &UpstreamConfig{Executor: proxy.NewRuntimeExecutor(time.Second)}, pollInterval: time.Millisecond}
}

func mediaTestRead(t *testing.T, resp *http.Response) map[string]json.RawMessage {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestDirectMiniMaxImageHTTPRoundTrip(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/image_generation" || r.Header.Get("Authorization") != "Bearer selected-key" {
			t.Errorf("unexpected request: %s %s auth=%t", r.Method, r.URL.Path, r.Header.Get("Authorization") != "")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for key, want := range map[string]string{"model": `"image-01-live"`, "response_format": `"base64"`, "seed": "9007199254740993", "width": "768", "height": "1024", "n": "2", "prompt_optimizer": "false", "aigc_watermark": "true", "subject_reference": `[{"type":"character","image_file":"data:image/png;base64,aW1hZ2U="}]`} {
			if string(body[key]) != want {
				t.Errorf("%s=%s; want %s", key, body[key], want)
			}
		}
		if _, ok := body["size"]; ok {
			t.Error("OpenAI size was not translated")
		}
		_, _ = io.WriteString(w, `{"id":"trace-mini","created":123,"base_resp":{"status_code":0,"status_msg":"success"},"data":{"image_urls":["https://cdn.example/image.png"],"image_base64":["aW1hZ2U="]},"metadata":{"success_count":2},"usage":{"total_tokens":17}}`)
	}))
	defer server.Close()
	req := mediaTestRequest(t, context.Background(), server.URL+"/v1/image_generation", "minimax-image", "/v1/images/generations", "application/json", []byte(`{"model":"image-01-live","prompt":"cat","size":"1536x1024","width":768,"response_format":"b64_json","seed":9007199254740993,"n":2,"prompt_optimizer":false,"aigc_watermark":true,"subject_reference":[{"type":"character","image_file":"data:image/png;base64,aW1hZ2U="}]}`))
	resp, err := mediaTestOperation().run(req, "minimax-image")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || directMediaFirstByteLatencyMs(resp) == nil {
		t.Fatalf("status=%d; first byte=%v", resp.StatusCode, directMediaFirstByteLatencyMs(resp))
	}
	body := mediaTestRead(t, resp)
	if string(body["data"]) != `[{"url":"https://cdn.example/image.png"},{"b64_json":"aW1hZ2U="}]` || string(body["usage"]) != `{"total_tokens":17}` || mediaString(body, "id") != "trace-mini" || string(body["created"]) != "123" || string(body["metadata"]) != `{"success_count":2}` {
		t.Fatalf("response lost data: %s", mustMediaJSON(body))
	}
}

func mustMediaJSON(value any) string { body, _ := json.Marshal(value); return string(body) }

func TestDirectMiniMaxImageErrorsRetainCodesAndUsage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, code string
		wantStatus int
	}{
		{"http", 401, `{"error":{"code":"invalid_key","message":"denied"},"usage":{"total_tokens":3}}`, `"invalid_key"`, 401},
		{"business", 200, `{"base_resp":{"status_code":1008,"status_msg":"insufficient balance"},"data":{"image_urls":["https://cdn.example/bad"]},"usage":{"total_tokens":3}}`, "1008", 502},
		{"empty", 200, `{"base_resp":{"status_code":0},"data":{},"usage":{"total_tokens":3}}`, `"empty_images"`, 502},
		{"error-body", 200, `{"error":{"code":"blocked_prompt","message":"denied"},"usage":{"total_tokens":3}}`, `"blocked_prompt"`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			req := mediaTestRequest(t, context.Background(), server.URL+"/v1/image_generation", "minimax-image", "/v1/images/generations", "application/json", []byte(`{"prompt":"cat"}`))
			resp, err := mediaTestOperation().run(req, "minimax-image")
			if err != nil {
				t.Fatal(err)
			}
			body := mediaTestRead(t, resp)
			var fault map[string]json.RawMessage
			_ = json.Unmarshal(body["error"], &fault)
			if resp.StatusCode != tc.wantStatus || string(fault["code"]) != tc.code || string(body["usage"]) != `{"total_tokens":3}` {
				t.Fatalf("status=%d body=%s", resp.StatusCode, mustMediaJSON(body))
			}
		})
	}
}

func TestDirectModelScopeImageAsyncRoundTrip(t *testing.T) {
	t.Parallel()
	var posts, polls, downloads atomic.Int32
	imageBytes := []byte("fixture-image-bytes")
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Host == "scope.example" && r.URL.Path == "/custom/v1/images/generations":
			posts.Add(1)
			if r.Header.Get("Authorization") != "Bearer selected-key" || r.Header.Get("X-ModelScope-Async-Mode") != "true" || r.Header.Get("X-Private") != "fixture-private" {
				t.Error("submission headers lost")
			}
			var body map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			if mediaString(body, "size") != "2048x2048" || mediaString(body, "prompt") != "a fox" || string(body["seed"]) != "9007199254740993" || string(body["n"]) != "2" || len(body["response_format"]) != 0 {
				t.Errorf("submission body=%s", mustMediaJSON(body))
			}
			_, _ = io.WriteString(w, `{"task_id":"task /one","task_status":"PENDING","usage":{"total_tokens":23},"metadata":{"trace":"keep"}}`)
		case r.URL.Host == "scope.example" && r.URL.EscapedPath() == "/custom/v1/tasks/task%20%2Fone":
			if r.Header.Get("Authorization") != "Bearer selected-key" || r.Header.Get("X-ModelScope-Task-Type") != "image_generation" || r.Header.Get("X-ModelScope-Async-Mode") != "" {
				t.Error("task must reuse submitting credential and task header")
			}
			if polls.Add(1) == 1 {
				_, _ = io.WriteString(w, `{"task_status":"RUNNING"}`)
				return
			}
			_, _ = io.WriteString(w, `{"task_status":"SUCCEED","created":456,"output_images":["http://cdn.example/image.png",{"b64_json":"c2Vjb25k","revised_prompt":"refined"}]}`)
		case r.URL.Host == "cdn.example" && r.URL.Path == "/image.png":
			downloads.Add(1)
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Private") != "" || r.Header.Get("X-ModelScope-Task-Type") != "" {
				t.Error("download leaked selected credential or custom headers")
			}
			_, _ = w.Write(imageBytes)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	defer proxyServer.Close()
	op := mediaTestOperation()
	op.proxyConfig = &platform.ProxyConfig{ProxyURL: proxyServer.URL, CustomHeaders: map[string]string{"X-Private": "fixture-private"}}
	req := mediaTestRequest(t, context.Background(), "http://scope.example/custom/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":" a fox ","size":"AUTO","seed":9007199254740993,"n":2,"response_format":"b64_json"}`))
	resp, err := op.run(req, "modelscope-image")
	if err != nil {
		t.Fatal(err)
	}
	body := mediaTestRead(t, resp)
	var images []map[string]string
	_ = json.Unmarshal(body["data"], &images)
	if resp.StatusCode != 200 || len(images) != 2 || images[0]["b64_json"] != base64.StdEncoding.EncodeToString(imageBytes) || images[1]["revised_prompt"] != "refined" || string(body["usage"]) != `{"total_tokens":23}` || string(body["metadata"]) != `{"trace":"keep"}` || string(body["created"]) != "456" {
		t.Fatalf("status=%d response=%s", resp.StatusCode, mustMediaJSON(body))
	}
	if posts.Load() != 1 || polls.Load() != 2 || downloads.Load() != 1 {
		t.Fatalf("posts=%d polls=%d downloads=%d", posts.Load(), polls.Load(), downloads.Load())
	}
}

func TestDirectModelScopeImageFailures(t *testing.T) {
	for _, tc := range []struct {
		name, submit, poll     string
		pollStatus, wantStatus int
		code                   string
	}{
		{"failed", `{"task_id":"one","usage":{"total_tokens":7}}`, `{"task_status":"FAILED","errors":{"code":"policy_rejected","message":"prompt rejected"}}`, 200, 502, `"policy_rejected"`},
		{"poll-http", `{"task_id":"one","usage":{"total_tokens":7}}`, `{"error":{"code":"task_unavailable","message":"try later"}}`, 503, 503, `"task_unavailable"`},
		{"cancelled", `{"task_id":"one","usage":{"total_tokens":7}}`, `{"task_status":"CANCELLED","code":42,"message":"cancelled upstream"}`, 200, 502, "42"},
		{"missing", `{"usage":{"total_tokens":7}}`, ``, 200, 502, `"missing_task"`},
		{"empty", `{"task_id":"one","usage":{"total_tokens":7}}`, `{"task_status":"SUCCEED","output_images":[]}`, 200, 502, `"empty_images"`},
		{"unknown", `{"task_id":"one","usage":{"total_tokens":7}}`, `{"task_status":"UNRECOGNIZED"}`, 200, 502, `"invalid_task_status"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					_, _ = io.WriteString(w, tc.submit)
					return
				}
				w.WriteHeader(tc.pollStatus)
				_, _ = io.WriteString(w, tc.poll)
			}))
			defer server.Close()
			req := mediaTestRequest(t, context.Background(), server.URL+"/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":"cat"}`))
			resp, err := mediaTestOperation().run(req, "modelscope-image")
			if err != nil {
				t.Fatal(err)
			}
			body := mediaTestRead(t, resp)
			var fault map[string]json.RawMessage
			_ = json.Unmarshal(body["error"], &fault)
			if resp.StatusCode != tc.wantStatus || string(fault["code"]) != tc.code || string(body["usage"]) != `{"total_tokens":7}` || posts.Load() != 1 {
				t.Fatalf("status=%d body=%s posts=%d", resp.StatusCode, mustMediaJSON(body), posts.Load())
			}
		})
	}
}

func TestDirectModelScopeImageCancellationAndDeadline(t *testing.T) {
	for _, phase := range []string{"submit", "poll", "download"} {
		t.Run(phase, func(t *testing.T) {
			entered := make(chan struct{})
			released := make(chan struct{})
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := "submit"
				if strings.Contains(r.URL.Path, "/tasks/") {
					current = "poll"
				}
				if r.URL.Path == "/image.png" {
					current = "download"
				}
				if current == phase {
					close(entered)
					select {
					case <-r.Context().Done():
					case <-released:
					}
					return
				}
				if current == "submit" {
					_, _ = io.WriteString(w, `{"task_id":"one","usage":{"total_tokens":31}}`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"task_status":"SUCCEED","output_images":[%q]}`, server.URL+"/image.png")
			}))
			defer server.Close()
			defer close(released)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := mediaTestRequest(t, ctx, server.URL+"/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":"cat"}`))
			result := make(chan error, 1)
			go func() { _, err := mediaTestOperation().run(req, "modelscope-image"); result <- err }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("phase not entered")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error=%v", err)
				}
				if phase != "submit" && directMediaFailureUsage(err).TotalTokens != 31 {
					t.Fatalf("observed usage lost after cancellation: %+v", directMediaFailureUsage(err))
				}
			case <-time.After(time.Second):
				t.Fatal("operation ignored cancellation")
			}
		})
	}
	t.Run("hung-poll-deadline", func(t *testing.T) {
		released := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				_, _ = io.WriteString(w, `{"task_id":"one"}`)
				return
			}
			select {
			case <-r.Context().Done():
			case <-released:
			}
		}))
		defer server.Close()
		defer close(released)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		req := mediaTestRequest(t, ctx, server.URL+"/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":"cat"}`))
		_, err := mediaTestOperation().run(req, "modelscope-image")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestDirectModelScopeImageEditsAndValidation(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "Qwen/edit", "prompt": "merge", "size": " 1024x1024 ", "seed": "9007199254740993"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range []string{"image-a", "image-b"} {
		part, err := writer.CreateFormFile("image[]", "image.png")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(part, data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	wire, contentType, err := prepareDirectMediaProfile("modelscope-image", "/v1/images/edits", writer.FormDataContentType(), body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(wire.Body, &payload)
	var images []string
	_ = json.Unmarshal(payload["image_url"], &images)
	want := []string{"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("image-a")), "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("image-b"))}
	if !reflect.DeepEqual(images, want) || mediaString(payload, "size") != "1024x1024" || string(payload["seed"]) != "9007199254740993" || contentType != "application/json" {
		t.Fatalf("request=%s", wire.Body)
	}
	for _, input := range []string{`{"image":"data:image/png;base64,aW1hZ2U="}`, `{"images":[{"image_url":"data:image/png;base64,aW1hZ2U="}]}`} {
		input = strings.TrimSuffix(input, "}") + `,"model":"Qwen/edit","prompt":"cat"}`
		wire, _, err := prepareDirectMediaProfile("modelscope-image", "/v1/images/edits", "application/json", []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(wire.Body, &payload)
		if mediaString(payload, "image_url") != "data:image/png;base64,aW1hZ2U=" {
			t.Fatalf("request=%s", wire.Body)
		}
	}
	for _, tc := range []struct{ profile, path, body string }{
		{"minimax-image", "/v1/images/edits", `{"prompt":"cat"}`},
		{"modelscope-image", "/v1/images/variations", `{"prompt":"cat"}`},
		{"modelscope-image", "/v1/images/edits", `{"model":"Qwen/edit","prompt":"cat","mask":"x"}`},
		{"modelscope-image", "/v1/images/edits", `{"model":"Qwen/edit","prompt":"cat"}`},
		{"modelscope-image", "/v1/images/generations", `{"model":"Qwen/image","prompt":"cat","stream":true}`},
		{"minimax-image", "/v1/images/generations", `{"prompt":"cat","size":"auto"}`},
		{"minimax-image", "/v1/images/generations", `{"prompt":"cat","width":1025}`},
		{"minimax-image", "/v1/images/generations", `{"prompt":" "}`},
	} {
		if _, _, err := prepareDirectMediaProfile(tc.profile, tc.path, "application/json", []byte(tc.body)); err == nil {
			t.Errorf("accepted invalid request %s %s", tc.path, tc.body)
		}
	}
}

func TestDirectModelScopeImageDownloadFailures(t *testing.T) {
	for _, test := range []struct {
		name           string
		downloadStatus int
		output         string
	}{
		{name: "http-error", downloadStatus: 403},
		{name: "empty-image", downloadStatus: 200},
		{name: "forbidden-output", output: "http://169.254.169.254/latest/meta-data"},
		{name: "invalid-data-url", output: "data:image/png;base64,invalid!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					output := test.output
					if output == "" {
						output = server.URL + "/image.png"
					}
					_, _ = fmt.Fprintf(w, `{"task_status":"SUCCEED","usage":{"total_tokens":9},"output_images":[%q]}`, output)
					return
				}
				w.WriteHeader(test.downloadStatus)
				if test.downloadStatus != 200 {
					_, _ = io.WriteString(w, `{"error":{"code":"cdn_denied","message":"expired image"}}`)
				}
			}))
			defer server.Close()
			req := mediaTestRequest(t, context.Background(), server.URL+"/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":"cat"}`))
			resp, err := mediaTestOperation().run(req, "modelscope-image")
			if test.output != "" {
				if err == nil {
					t.Fatal("unsafe or invalid result URL accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := mediaTestRead(t, resp)
			if resp.StatusCode < 400 || string(body["usage"]) != `{"total_tokens":9}` {
				t.Fatalf("download failure status=%d body=%s", resp.StatusCode, mustMediaJSON(body))
			}
		})
	}
}

func TestDirectModelScopeImageDownloadRejectsCrossOriginRedirect(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = fmt.Fprintf(w, `{"output_images":[%q]}`, server.URL+"/redirect")
			return
		}
		http.Redirect(w, r, other.URL+"/private", http.StatusFound)
	}))
	defer server.Close()
	req := mediaTestRequest(t, context.Background(), server.URL+"/v1/images/generations", "modelscope-image", "/v1/images/generations", "application/json", []byte(`{"model":"Qwen/image","prompt":"cat"}`))
	_, err := mediaTestOperation().run(req, "modelscope-image")
	if err == nil || leaked.Load() != 0 {
		t.Fatalf("redirect was followed: err=%v hits=%d", err, leaked.Load())
	}
}
