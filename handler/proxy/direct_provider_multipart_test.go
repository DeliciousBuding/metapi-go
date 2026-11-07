package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func providerUploadBody(t *testing.T, data []byte, mimeType string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "client-alias")
	_ = writer.WriteField("prompt", "fixture prompt")
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="input_reference"; filename="reference.bin"`}, "Content-Type": {mimeType}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = writer.Close()
	return body.Bytes(), writer.FormDataContentType()
}

func TestProviderVideoUploadsUseHostInlineEncoding(t *testing.T) {
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aVr8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, protocol := range []struct {
			key, profile string
			bit          int
		}{{"video", "", store.DirectProtocolVideo}, {"seedanceVideo", "seedance-video", store.DirectProtocolSeedanceVideo}, {"zenmuxVideo", "zenmux-video", store.DirectProtocolZenmuxVideo}} {
			for _, mode := range []string{"base64", "dataUrl", "unknown-ref", "shadow-placeholder", "size-limit", "model-change", "no-declaration"} {
				t.Run(dialect+"/"+protocol.key+"/"+mode, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var body map[string]json.RawMessage
						if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["model"]) != `"provider-model"` || string(body["extension"]) != "9007199254740993" {
							t.Error("inline encoding changed native metadata/integer")
						}
						var value string
						_ = json.Unmarshal(body["input_reference"], &value)
						want := base64.StdEncoding.EncodeToString(image)
						if mode == "dataUrl" {
							typeName := "application/octet-stream"
							if protocol.profile != "" {
								typeName = "image/png"
							}
							want = "data:" + typeName + ";base64," + want
						}
						if value != want {
							t.Errorf("wrong host inline encoding: %s", value)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"upload-one","object":"video","status":"queued","model":"provider-model","usage":{"input_tokens":7,"total_tokens":7}}`)
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					encoded, _ := json.Marshal(map[string]any{protocol.key: map[string]any{"url": upstream.URL + "/tenant/videos", "auth": "bearer", "profile": protocol.profile}})
					videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, string(encoded))
					videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, protocol.bit)
					videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[]'`)
					encoding := mode
					if mode != "base64" {
						encoding = "dataUrl"
					}
					source := strings.Replace(multipartMediaSource, `protocols:["video"]`, `protocols:["`+protocol.key+`"]`, 1)
					start := strings.Index(source, "export function submit")
					end := strings.Index(source, "export function query")
					hook := `export function submit(i){if(i.bodyKind!=="multipart"||Object.keys(i.files[0]).sort().join(",")!=="field,filename,mimeType,ref,size")throw new Error("file metadata expected");const b=JSON.parse(i.body);const file={__fileRef:i.files[0].ref,encoding:"` + encoding + `"};return {method:"POST",body:'{"model":'+JSON.stringify(i.model)+',"prompt":'+JSON.stringify(b.prompt)+',"input_reference":'+JSON.stringify(file)+',"extension":9007199254740993}'};}`
					if mode == "unknown-ref" {
						hook = strings.Replace(hook, "i.files[0].ref", `"file_from_another_request"`, 1)
					}
					if mode == "shadow-placeholder" {
						hook = strings.Replace(hook, "encoding:", `__FileRef:"shadow",encoding:`, 1)
					}
					if mode == "size-limit" {
						hook = strings.Replace(hook, "encoding:", "maxBytes:1,encoding:", 1)
					}
					if mode == "model-change" {
						hook = strings.Replace(hook, "JSON.stringify(i.model)", `'"other-model"'`, 1)
					}
					source = source[:start] + hook + source[end:]
					if mode == "no-declaration" {
						source = strings.Replace(source, `requestKinds:["json","multipart"],`, "", 1)
					}
					if _, err := channelplugins.Save(t.Context(), db.DB, "upload", source, true, 0); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "upload", Protocols: []string{protocol.key}}})
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					raw, contentType := providerUploadBody(t, image, "application/octet-stream")
					req, _ := http.NewRequest("POST", server.URL+"/v1/videos", bytes.NewReader(raw))
					req.Header.Set("Authorization", "Bearer client-one")
					req.Header.Set("Content-Type", contentType)
					response, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					result, _ := io.ReadAll(response.Body)
					wantStatus, wantCalls := 502, int32(0)
					if mode == "base64" || mode == "dataUrl" {
						wantStatus, wantCalls = 200, 1
					}
					if mode == "no-declaration" {
						wantStatus = 415
						var archived int
						if err := db.Get(&archived, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || archived != 0 {
							t.Fatalf("unsupported form archived a program: count=%d %v", archived, err)
						}
					}
					if response.StatusCode != wantStatus || calls.Load() != wantCalls {
						t.Fatalf("upload protocol/guard changed: status=%d calls=%d %s", response.StatusCode, calls.Load(), result)
					}
				})
			}
		}
	}
}

const multipartMediaSource = `export const meta={apiVersion:2,kind:"provider",key:"upload",name:"Upload",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],requestKinds:["json","multipart"],hooks:["submit","query","content","taskResult"]};export function submit(i){if(i.bodyKind!=="multipart"||i.files.length!==1)throw new Error("metadata expected");const b=JSON.parse(i.body);if(b.model!==i.model||i.files[0].filename!=="reference.bin"||i.files[0].size!==4)throw new Error("invalid metadata");return {method:"POST",parts:[{field:"model",value:i.model},{field:"prompt",value:b.prompt},{field:"input_reference",fileRef:i.files[0].ref}]};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestProviderVideoMultipartKeepsFilesOutOfJS(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("model") != "provider-model" || r.FormValue("prompt") != "fixture prompt" || r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
					t.Error("multipart metadata/selected authentication changed")
				}
				file := r.MultipartForm.File["input_reference"][0]
				reader, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				data, _ := io.ReadAll(reader)
				if file.Filename != "reference.bin" || file.Header.Get("Content-Type") != "application/octet-stream" || !bytes.Equal(data, []byte{0, 1, 2, 3}) {
					t.Error("host did not preserve uploaded file bytes/type/name")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"upload-one","object":"video","status":"queued","model":"provider-model","usage":{"input_tokens":7,"output_tokens":0,"total_tokens":7}}`)
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			if _, err := channelplugins.Save(t.Context(), db.DB, "upload", multipartMediaSource, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "upload", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			_ = writer.WriteField("model", "client-alias")
			_ = writer.WriteField("prompt", "fixture prompt")
			part, _ := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="input_reference"; filename="reference.bin"`}, "Content-Type": {"application/octet-stream"}})
			_, _ = part.Write([]byte{0, 1, 2, 3})
			_ = writer.Close()
			req, _ := http.NewRequest("POST", server.URL+"/v1/videos", &body)
			req.Header.Set("Authorization", "Bearer client-one")
			req.Header.Set("Content-Type", writer.FormDataContentType())
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			raw, _ := io.ReadAll(response.Body)
			var created struct {
				ID string `json:"id"`
			}
			if response.StatusCode != 200 || json.Unmarshal(raw, &created) != nil || calls.Load() != 1 {
				t.Fatalf("multipart provider did not dispatch exactly once: %d calls=%d %s", response.StatusCode, calls.Load(), raw)
			}
		})
	}
}
