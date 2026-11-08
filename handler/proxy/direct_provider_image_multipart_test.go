package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderAsyncImageMultipartUsesOriginalFileAndSingleAttempt(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, protocol := range []string{"imageEdit", "imageVariation"} {
			modes := []bool{false}
			if protocol == "imageEdit" {
				modes = append(modes, true)
			}
			for _, stream := range modes {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", dialect, protocol, stream), func(t *testing.T) {
					var calls atomic.Int32
					fileBytes := []byte{0, 1, 2, 255, 13, 10}
					path, bit := "/images/edits", store.DirectProtocolImageEdit
					if protocol == "imageVariation" {
						path, bit = "/images/variations", store.DirectProtocolImageVariation
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						count := calls.Add(1)
						if r.Header.Get("Authorization") != "Bearer fixture-media-key" {
							t.Error("multipart image operation lost current supplier authentication")
						}
						if count == 1 {
							if r.Method != http.MethodPost || r.URL.Path != path || r.ParseMultipartForm(1<<20) != nil {
								t.Error("image submission lost its selected multipart operation")
								w.WriteHeader(400)
								return
							}
							defer r.MultipartForm.RemoveAll()
							if stream && r.FormValue("stream") != "" {
								t.Error("multipart facade asked supplier to stream")
							}
							if r.FormValue("model") != "provider-model" || r.FormValue("prompt") != "fixture image" || len(r.MultipartForm.File["image"]) != 1 {
								t.Error("multipart model, fields or file reference changed")
								w.WriteHeader(400)
								return
							}
							file := r.MultipartForm.File["image"][0]
							reader, err := file.Open()
							if err != nil {
								t.Error(err)
								return
							}
							observed, readErr := io.ReadAll(reader)
							_ = reader.Close()
							if readErr != nil || !bytes.Equal(observed, fileBytes) || file.Filename != "source.png" || file.Header.Get("Content-Type") != "image/png" {
								t.Error("host multipart encoding changed image bytes/name/MIME")
							}
						} else if r.Method != http.MethodGet || r.URL.EscapedPath() != "/tasks/edit%2Fone" || r.Header.Get("Content-Type") != "" {
							t.Error("query reused upload bytes or changed its saved operation")
						}
						status, output := "PENDING", 0
						if count > 1 {
							status, output = "DONE", 2
						}
						w.Header().Set("Content-Type", "application/json")
						native := `{"created":1,"data":[{"b64_json":"AAEC/w0K"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
						_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "edit/one", "task_status": status, "native": native, "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}})
					}))
					defer server.Close()
					endpoint := &store.DirectEndpoint{URL: server.URL + path, Auth: store.DirectAuthBearer}
					endpoints := store.DirectEndpoints{ImageEdit: endpoint}
					if protocol == "imageVariation" {
						endpoints = store.DirectEndpoints{ImageVariation: endpoint}
					}
					db := installDirectMediaRuntime(t, dialect, endpoints, bit, "")
					source := strings.ReplaceAll(imageOperationModule, "imageGeneration", protocol)
					if stream {
						source = strings.Replace(source, `query:{path`, `stream:true,query:{path`, 1)
					}
					source = strings.Replace(source, `credentialKinds:["api_key"],`, `credentialKinds:["api_key"],requestKinds:["json","multipart"],`, 1)
					source = strings.Replace(source, `return {method:"POST",body:i.body};`, `if(i.bodyKind!=="multipart"||Object.keys(i.files[0]).sort().join(",")!=="field,filename,mimeType,ref,size")throw new Error("metadata only");const b=JSON.parse(i.body);return {method:"POST",parts:[{field:"model",value:i.model},{field:"prompt",value:b.prompt},{field:"image",fileRef:i.files[0].ref}]};`, 1)
					if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", source, true, 0); err != nil {
						t.Fatal(err)
					}
					config := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "image-operation", Protocols: []string{protocol}, ImageQueries: map[string]store.DirectImageQueryEndpoint{protocol: {URL: server.URL + "/tasks/{taskId}", Auth: store.DirectAuthBearer}}}}
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), config); err != nil {
						t.Fatal(err)
					}
					cfg := getUpstreamConfig()
					cfg.ChannelPlugins = channelplugins.New(db.DB)
					var entries []proxy.ProxyLogEntry
					cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { entries = append(entries, entry); return nil }
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					_ = writer.WriteField("model", "client-alias")
					_ = writer.WriteField("prompt", "fixture image")
					if stream {
						_ = writer.WriteField("stream", "true")
						_ = writer.WriteField("partial_images", "0")
						_ = writer.WriteField("n", "1")
					}
					header := textproto.MIMEHeader{}
					header.Set("Content-Disposition", `form-data; name="image"; filename="source.png"`)
					header.Set("Content-Type", "image/png")
					part, err := writer.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(fileBytes)
					_ = writer.Close()
					request := makeProxyReq(http.MethodPost, "/v1"+path, body.String())
					request.Header.Set("Content-Type", writer.FormDataContentType())
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, request)
					if out.Code != http.StatusOK || calls.Load() != 2 || len(entries) != 1 || entries[0].Status != "success" || entries[0].TotalTokens == nil || *entries[0].TotalTokens != 9 || !strings.Contains(out.Body.String(), "AAEC/w0K") {
						t.Fatalf("multipart image facade replayed or changed original usage: status=%d calls=%d entries=%+v body=%s", out.Code, calls.Load(), entries, out.Body.String())
					}
					if stream && (!strings.Contains(out.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(out.Body.String(), "image_edit.completed")) {
						t.Fatal("multipart edit lost original stream facade")
					}
				})
			}
		}
	}
}
