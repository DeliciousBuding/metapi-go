package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/store"
)

func providerUploadForTest(t *testing.T) (*directProviderUpload, []byte) {
	t.Helper()
	body, contentType := providerUploadBody(t, []byte{0, 1, 2, 3}, "application/octet-stream")
	r := httptest.NewRequest("POST", "/v1/videos", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	if _, err := ParseMultipartFormData(r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.MultipartForm.RemoveAll() })
	upload, fields, err := prepareDirectProviderUpload(r, &store.DirectUpstreamCandidate{}, "provider-model")
	if err != nil {
		t.Fatal(err)
	}
	return upload, fields
}

func TestProviderUploadPartsPreserveRepeatedFieldsAndOriginalFile(t *testing.T) {
	upload, _ := providerUploadForTest(t)
	model, prompt, empty, a, b := "provider-model", "fixture prompt", "", "one", "two"
	upload.parts = []channelplugin.MultipartPart{{Field: "model", Value: &model}, {Field: "prompt", Value: &prompt}, {Field: "empty", Value: &empty}, {Field: "tag", Value: &a}, {Field: "tag", Value: &b}, {Field: "image[]", FileRef: upload.metadata[0].Ref}}
	fields, err := multipartPartsFields(upload.parts, upload)
	if err != nil {
		t.Fatal(err)
	}
	wire := &directProviderWire{Body: fields, Headers: make(http.Header), DriverUpload: upload}
	contentType, err := finalizeDirectProviderUpload(t.Context(), wire)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/vendor", bytes.NewReader(wire.Body))
	r.Header.Set("Content-Type", contentType)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	defer r.MultipartForm.RemoveAll()
	if strings.Join(r.MultipartForm.Value["tag"], ",") != "one,two" || len(r.MultipartForm.Value["empty"]) != 1 || r.FormValue("model") != model {
		t.Fatal("multipart field values/order/empty changed")
	}
	file := r.MultipartForm.File["image[]"][0]
	reader, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if !bytes.Equal(body, []byte{0, 1, 2, 3}) || file.Filename != "reference.bin" || file.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatal("file data/name/type changed")
	}
}

func TestProviderUploadRejectsCrossRequestRefsAndFileFieldCollisions(t *testing.T) {
	upload, _ := providerUploadForTest(t)
	other, _ := providerUploadForTest(t)
	value := "fixture"
	for _, parts := range [][]channelplugin.MultipartPart{
		{{Field: "image", FileRef: other.metadata[0].Ref}},
		{{Field: "image", FileRef: "temporary-path"}},
		{{Field: "MODEL", FileRef: upload.metadata[0].Ref}},
		{{Field: "image", Value: &value}, {Field: "image", FileRef: upload.metadata[0].Ref}},
	} {
		if _, err := multipartPartsFields(parts, upload); err != channelplugin.ErrResult {
			t.Fatalf("unsafe file refs accepted: %v", err)
		}
	}
}

func TestProviderUploadEnforcesFinalSizeCountAndCancellation(t *testing.T) {
	for _, mode := range []string{"body-limit", "file-count", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			upload, fields := providerUploadForTest(t)
			model := "provider-model"
			upload.parts = []channelplugin.MultipartPart{{Field: "model", Value: &model}, {Field: "image", FileRef: upload.metadata[0].Ref}}
			ctx := t.Context()
			if mode == "body-limit" {
				old := config.GetSafe()
				config.Set(&config.Config{RequestBodyLimit: 32})
				t.Cleanup(func() { config.Set(old) })
			}
			if mode == "file-count" {
				t.Setenv("PROXY_MAX_MULTIPART_FILES", "1")
				upload.parts = append(upload.parts, upload.parts[1])
			}
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			wire := &directProviderWire{Body: fields, Headers: make(http.Header), DriverUpload: upload}
			if _, err := finalizeDirectProviderUpload(ctx, wire); err == nil {
				t.Fatal("final upload exceeded host limit or ignored cancellation")
			}
		})
	}
}

func TestProviderUploadJSONKeepsExactNumbersAndBoundsInlineExpansion(t *testing.T) {
	for _, mode := range []string{"normal", "unknown", "limit", "shadow", "extra-field", "too-deep"} {
		t.Run(mode, func(t *testing.T) {
			upload, _ := providerUploadForTest(t)
			placeholder := map[string]any{"__fileRef": upload.metadata[0].Ref, "encoding": "base64"}
			if mode == "unknown" {
				placeholder["__fileRef"] = "cross-request"
			}
			if mode == "limit" {
				placeholder["maxBytes"] = 1
			}
			if mode == "shadow" {
				placeholder["__FileRef"] = upload.metadata[0].Ref
			}
			if mode == "extra-field" {
				placeholder["path"] = "temporary-path"
			}
			raw, _ := json.Marshal(placeholder)
			body := []byte(`{"number":9007199254740993,"nested":[` + string(raw) + `]}`)
			if mode == "too-deep" {
				body = []byte(strings.Repeat("[", 66) + string(raw) + strings.Repeat("]", 66))
			}
			wire := &directProviderWire{Body: body, Headers: make(http.Header), DriverUpload: upload}
			_, err := finalizeDirectProviderUpload(t.Context(), wire)
			if mode == "normal" {
				if err != nil || !bytes.Contains(wire.Body, []byte("9007199254740993")) || !bytes.Contains(wire.Body, []byte("AAECAw==")) {
					t.Fatalf("inline expansion changed data: %s %v", wire.Body, err)
				}
			} else if err == nil {
				t.Fatal("unsafe inline descriptor accepted")
			}
		})
	}
}
