package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

const codexImageFixture = "iVBORw0KGgo="

func TestDirectCodexImagesRequest(t *testing.T) {
	endpoint := &store.DirectEndpoint{Profile: "codex-image", RequestModel: "responses-main-model"}
	for _, edit := range []bool{false, true} {
		for _, form := range []bool{false, true} {
			path := "/v1/images/generations"
			if edit {
				path = "/v1/images/edits"
			}
			t.Run(path+map[bool]string{true: "_multipart", false: "_json"}[form], func(t *testing.T) {
				payload := map[string]any{"model": "client-model", "prompt": " preserve whitespace ", "n": 1, "stream": true, "partial_images": 2, "size": "1024x1024", "quality": "high", "background": "transparent", "output_format": "webp", "output_compression": 77, "moderation": "low", "input_fidelity": "high"}
				payload["image"] = "data:image/png;base64," + codexImageFixture
				if edit {
					payload["mask"] = payload["image"]
				}
				raw, _ := json.Marshal(payload)
				r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
				if form {
					var buffer bytes.Buffer
					writer := multipart.NewWriter(&buffer)
					for key, value := range payload {
						if key == "image" || key == "mask" {
							part, _ := writer.CreateFormFile(key, "fixture.png")
							data, _ := base64.StdEncoding.DecodeString(codexImageFixture)
							_, _ = part.Write(data)
						} else {
							field, _ := json.Marshal(value)
							if text, ok := value.(string); ok {
								field = []byte(text)
							}
							_ = writer.WriteField(key, string(field))
						}
					}
					_ = writer.Close()
					r = httptest.NewRequest("POST", path, &buffer)
					r.Header.Set("Content-Type", writer.FormDataContentType())
					if _, err := ParseMultipartFormData(r); err != nil {
						t.Fatal(err)
					}
					defer r.MultipartForm.RemoveAll()
				}
				converted, err := prepareDirectCodexImagesRequest(r, &Ctx{DownstreamPath: path, Multipart: form, IsStream: true}, endpoint, "actual-image-model", raw)
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				_ = json.Unmarshal(converted, &got)
				tool := got["tools"].([]any)[0].(map[string]any)
				if got["model"] != "responses-main-model" || tool["model"] != "actual-image-model" || got["stream"] != true || got["store"] != false || got["tool_choice"] != "required" {
					t.Fatalf("invalid Responses contract: %s", converted)
				}
				for _, key := range []string{"partial_images", "size", "quality", "background", "output_format", "output_compression", "moderation", "input_fidelity"} {
					want, _ := json.Marshal(payload[key])
					actual, _ := json.Marshal(tool[key])
					if string(want) != string(actual) {
						t.Errorf("lost %s", key)
					}
				}
				if !bytes.Contains(converted, []byte(codexImageFixture)) || !bytes.Contains(converted, []byte(" preserve whitespace ")) {
					t.Fatal("lost reference or prompt")
				}
				if edit && tool["input_image_mask"] == nil {
					t.Fatal("lost mask")
				}
			})
		}
	}
}

func TestDirectCodexImagesRejectLossyInput(t *testing.T) {
	endpoint := &store.DirectEndpoint{Profile: "codex-image", RequestModel: "main"}
	for _, field := range []string{`"n":2`, `"stream":null`, `"user":"identity"`, `"style":"vivid"`, `"response_format":"url"`, `"image":"https://example.com/image.png"`, `"image":["data:image/png;base64,!!!"]`, `"image":null`, `"images":[{"image_url":"data:image/png;base64,aQ==","extra":true}]`, `"partial_images":1`, `"output_compression":101`, `"size":null`} {
		t.Run(field, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/images/generations", nil)
			_, err := prepareDirectCodexImagesRequest(r, &Ctx{DownstreamPath: r.URL.Path}, endpoint, "image", []byte(`{"prompt":"image",`+field+`}`))
			if err == nil {
				t.Fatal("unrepresentable input accepted")
			}
		})
	}
}

func codexImageFrames(image string, finalOutput bool) string {
	item := `{"id":"ig_fixture","type":"image_generation_call","status":"generating","action":"generate","background":"transparent","output_format":"png","quality":"high","size":"1024x1024","revised_prompt":"actual prompt","result":"` + image + `"}`
	output := "[]"
	if finalOutput {
		output = "[" + item + "]"
	}
	return "data: " + `{"type":"response.created","response":{"created_at":123}}` + "\n\n" +
		"data: " + `{"type":"response.image_generation_call.partial_image","partial_image_b64":"` + codexImageFixture + `","partial_image_index":0}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","output_index":0,"item":` + item + `}` + "\n\n" +
		"data: " + `{"type":"response.completed","response":{"status":"completed","created_at":123,"output":` + output + `,"usage":{"input_tokens":5,"output_tokens":9,"total_tokens":14,"input_tokens_details":{"cached_tokens":2}}}}` + "\n\n"
}

func TestDirectCodexImagesResponseLargeFramesAndUsage(t *testing.T) {
	image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 1<<20))
	for _, stream := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			reader := newDirectCodexImagesBody(io.NopCloser(strings.NewReader(codexImageFrames(image, full))), "/v1/images/edits", stream, 8<<20)
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			usage := reader.original.Result().Usage
			if !usage.Found || usage.TotalTokens != 14 {
				t.Fatalf("actual usage lost: %+v", usage)
			}
			if !bytes.Contains(body, []byte(image)) || bytes.Contains(body, []byte("response.output")) {
				t.Fatal("image bytes missing or source protocol leaked")
			}
			if stream {
				if !bytes.Contains(body, []byte("image_edit.partial_image")) || !bytes.Contains(body, []byte("image_edit.completed")) || !bytes.Contains(body, []byte(`"total_tokens":14`)) {
					t.Fatal("stream lost partial, completion or usage")
				}
			} else if !json.Valid(body) || !bytes.Contains(body, []byte(`"created":123`)) || !bytes.Contains(body, []byte(`"revised_prompt":"actual prompt"`)) {
				t.Fatal("invalid Images JSON")
			}
		}
	}
}

func TestDirectCodexImagesRejectIncompleteAndFailedTools(t *testing.T) {
	complete := codexImageFrames(codexImageFixture, false)
	inputs := []string{
		strings.Split(complete, `data: {"type":"response.completed"`)[0],
		strings.Replace(complete, `"status":"generating"`, `"status":"failed"`, 1),
		strings.Replace(complete, `"status":"completed"`, `"status":"incomplete"`, 1),
		`data: {"type":"response.completed","response":{"status":"completed","output":[]}}` + "\n\n",
		`event: error` + "\n" + `data: {"message":"untrusted secret"}` + "\n\n",
		strings.Replace(complete, codexImageFixture, "invalid***", 1),
	}
	for _, raw := range inputs {
		reader := newDirectCodexImagesBody(io.NopCloser(strings.NewReader(raw)), "/v1/images/generations", true, 8<<20)
		body, err := io.ReadAll(reader)
		if err == nil || strings.Contains(string(body), "image_generation.completed") {
			t.Fatal("failed image stream appeared successful")
		}
		if strings.Contains(err.Error(), "untrusted secret") {
			t.Fatal("upstream error contents leaked")
		}
	}
}

func TestDirectCodexImagesSafeErrorUsageAndLimits(t *testing.T) {
	raw := `event: error` + "\n" + `data: {"message":"fixture-access-secret","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}` + "\n\n"
	reader := newDirectCodexImagesBody(io.NopCloser(strings.NewReader(raw)), "/v1/images/generations", true, 1<<20)
	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("error event succeeded")
	}
	result := reader.original.Result()
	if result.Usage.TotalTokens != 3 || !result.HasErrorEvent {
		t.Fatalf("error usage lost: %+v", result)
	}
	for _, event := range result.ErrorEvents {
		if strings.Contains(event.Data, "fixture-access-secret") {
			t.Fatal("provider secret retained in log analysis")
		}
	}
	reader = newDirectCodexImagesBody(io.NopCloser(strings.NewReader(codexImageFrames(codexImageFixture, false))), "/v1/images/generations", true, 32)
	_, err = io.ReadAll(reader)
	if err != errMessagesChatStreamLimit {
		t.Fatalf("stream limit not enforced: %v", err)
	}
	t.Setenv("PROXY_MAX_MULTIPART_FILE_BYTES", "4")
	err = validateDirectCodexImageURL("data:image/png;base64," + codexImageFixture)
	if !isRequestBodyTooLarge(err) {
		t.Fatalf("JSON image limit not enforced: %v", err)
	}
}

func TestDirectCodexImagesObservedFirstOutput(t *testing.T) {
	reader := newDirectCodexImagesBody(io.NopCloser(strings.NewReader(codexImageFrames(codexImageFixture, false))), "/v1/images/generations", true, 1<<20)
	calls := 0
	reader.original.onFirstOutput = func() { calls++ }
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("first image output callback count=%d", calls)
	}
}

func TestDirectCodexImagesPartialMetadata(t *testing.T) {
	converter := newDirectCodexImagesStream("/v1/images/generations", true)
	frame := `data: {"type":"response.image_generation_call.partial_image","partial_image_index":1,"partial_image_b64":"` + codexImageFixture + `","created_at":321,"background":"transparent","output_format":"png","quality":"high","size":"1024x1024"}` + "\n\n"
	got, err := converter.TransformEvent([]byte(frame))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"created_at":321`, `"partial_image_index":1`, `"background":"transparent"`, `"output_format":"png"`, `"quality":"high"`, `"size":"1024x1024"`} {
		if !bytes.Contains(got, []byte(field)) {
			t.Fatalf("partial image metadata lost: %s", field)
		}
	}
}
