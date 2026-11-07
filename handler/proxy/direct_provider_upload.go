package proxyhandler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/store"
)

// These handles stay in the host attempt. JS sees only independently copied
// metadata; no byte buffer, file reader or temporary path is serialized.
type directProviderUpload struct {
	files    map[string]*multipart.FileHeader
	metadata []channelplugin.FileReference
	parts    []channelplugin.MultipartPart
	profile  string
}

func directMediaDriverProtocol(direct *store.DirectUpstreamCandidate, path string) string {
	if !strings.HasPrefix(path, "/v1/videos") {
		return ""
	}
	bit, _ := directVideoSelectedEndpoint(direct)
	for _, entry := range direct.Endpoints.Entries() {
		if entry.Protocol == bit {
			return entry.Key
		}
	}
	return ""
}

func hasDirectMultipartDriver(direct *store.DirectUpstreamCandidate, path string) bool {
	if direct == nil || direct.RequestConfig.Provider == nil {
		return false
	}
	protocol := directPluginProtocol(path)
	if strings.HasPrefix(path, "/v1/videos") {
		protocol = directMediaDriverProtocol(direct, path)
	}
	return slices.Contains(direct.RequestConfig.Provider.Protocols, protocol) && channelplugin.SupportsMultipartProtocol(protocol)
}

func directDriverDescriptorBody(descriptor channelplugin.RequestDescriptor, wire *directProviderWire) ([]byte, error) {
	if descriptor.Parts == nil {
		return []byte(descriptor.Body), nil
	}
	if wire.DriverUpload == nil {
		return nil, channelplugin.ErrResult
	}
	body, err := multipartPartsFields(descriptor.Parts, wire.DriverUpload)
	if err != nil {
		return nil, err
	}
	wire.DriverUpload.parts = descriptor.Parts
	return body, nil
}

func multipartDriverFields(values map[string][]string) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	for key, entries := range values {
		if len(entries) == 0 {
			return nil, channelplugin.ErrResult
		}
		if len(entries) == 1 {
			fields[key], _ = json.Marshal(entries[0])
			// Stream is a protocol control, not an arbitrary form string.
			if key == "stream" && (entries[0] == "false" || entries[0] == "true") {
				fields[key] = json.RawMessage(entries[0])
			}
		} else {
			if key == "model" || key == "stream" {
				return nil, channelplugin.ErrResult
			}
			fields[key], _ = json.Marshal(entries)
		}
	}
	return json.Marshal(fields)
}

func prepareDirectProviderUpload(r *http.Request, direct *store.DirectUpstreamCandidate, model string) (*directProviderUpload, []byte, error) {
	if r.MultipartForm == nil || validateMultipartForm(r.MultipartForm) != nil || len(r.MultipartForm.Value["model"]) != 1 {
		return nil, nil, channelplugin.ErrResult
	}
	values := make(map[string][]string, len(r.MultipartForm.Value))
	for key, entries := range r.MultipartForm.Value {
		values[key] = slices.Clone(entries)
	}
	overrides, err := directMultipartOverrides(model, direct.ParamOverride)
	if err != nil {
		return nil, nil, err
	}
	for key, value := range overrides {
		values[key] = []string{value}
	}
	body, err := multipartDriverFields(values)
	if err != nil || len(body) > channelplugin.MaxInputBytes {
		return nil, nil, channelplugin.ErrResult
	}
	upload := &directProviderUpload{files: map[string]*multipart.FileHeader{}}
	keys := make([]string, 0, len(r.MultipartForm.File))
	for key := range r.MultipartForm.File {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if _, exists := values[key]; exists || strings.EqualFold(key, "model") || strings.EqualFold(key, "stream") {
			return nil, nil, channelplugin.ErrResult
		}
		for _, file := range r.MultipartForm.File[key] {
			if file == nil {
				return nil, nil, channelplugin.ErrResult
			}
			ref := "file_" + rand.Text()
			copy := *file
			copy.Header = textproto.MIMEHeader(http.Header(file.Header).Clone())
			upload.files[ref] = &copy
			upload.metadata = append(upload.metadata, channelplugin.FileReference{Ref: ref, Field: key, Filename: file.Filename, MIMEType: file.Header.Get("Content-Type"), Size: file.Size})
		}
	}
	return upload, body, nil
}

func multipartPartsFields(parts []channelplugin.MultipartPart, upload *directProviderUpload) ([]byte, error) {
	values := map[string][]string{}
	for _, part := range parts {
		if part.Value != nil {
			values[part.Field] = append(values[part.Field], *part.Value)
			continue
		}
		if upload.files[part.FileRef] == nil || strings.EqualFold(part.Field, "model") || strings.EqualFold(part.Field, "stream") {
			return nil, channelplugin.ErrResult
		}
	}
	for _, part := range parts {
		if part.FileRef != "" && len(values[part.Field]) != 0 {
			return nil, channelplugin.ErrResult
		}
	}
	return multipartDriverFields(values)
}

type uploadBodyWriter struct {
	bytes.Buffer
	limit int
}

func (w *uploadBodyWriter) Write(body []byte) (int, error) {
	if len(body) > w.limit-w.Len() {
		return 0, channelplugin.ErrResult
	}
	return w.Buffer.Write(body)
}

type uploadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r uploadContextReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(body)
}

func providerUploadLimit() int {
	limit := config.DefaultRequestBodyLimit
	if cfg := config.GetSafe(); cfg != nil && cfg.RequestBodyLimit > 0 {
		limit = cfg.RequestBodyLimit
	}
	return limit
}

// Final serialization happens after all semantic operations/budget checks.
// Multipart fields stay extensible; immutable file refs can only resolve to
// this request's handles. No plugin or wire hook observes inline file bytes.
func finalizeDirectProviderUpload(ctx context.Context, wire *directProviderWire) (string, error) {
	upload := wire.DriverUpload
	if upload == nil {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if upload.parts == nil {
		body, err := expandDirectUploadJSON(ctx, wire.Body, upload, 0, new(int64))
		if err != nil || len(body) > providerUploadLimit() {
			return "", channelplugin.ErrResult
		}
		wire.Body = body
		wire.Headers.Set("Content-Type", "application/json")
		return "application/json", nil
	}
	var fields map[string]json.RawMessage
	if requestoverride.Decode(wire.Body, &fields) != nil {
		return "", channelplugin.ErrResult
	}
	values := map[string][]string{}
	for key, value := range fields {
		var single string
		var repeated []string
		if len(value) > 0 && value[0] == '"' && json.Unmarshal(value, &single) == nil {
			values[key] = []string{single}
		} else if len(value) > 0 && value[0] == '[' && json.Unmarshal(value, &repeated) == nil && repeated != nil {
			values[key] = repeated
		} else {
			values[key] = []string{string(value)}
		}
	}
	// Body operations may add/replace fields, never convert a file into a field.
	for _, part := range upload.parts {
		if part.FileRef != "" && len(values[part.Field]) > 0 {
			return "", channelplugin.ErrResult
		}
	}
	form := &multipart.Form{Value: values, File: map[string][]*multipart.FileHeader{}}
	for _, part := range upload.parts {
		if part.FileRef != "" {
			if upload.files[part.FileRef] == nil {
				return "", channelplugin.ErrResult
			}
			form.File[part.Field] = append(form.File[part.Field], upload.files[part.FileRef])
		}
	}
	if validateMultipartForm(form) != nil {
		return "", channelplugin.ErrResult
	}
	buffer := &uploadBodyWriter{limit: providerUploadLimit()}
	writer := multipart.NewWriter(buffer)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		for _, value := range values[key] {
			if err := writer.WriteField(key, value); err != nil {
				return "", err
			}
		}
	}
	total := int64(0)
	for _, part := range upload.parts {
		if part.FileRef == "" {
			continue
		}
		file := upload.files[part.FileRef]
		if file == nil {
			return "", channelplugin.ErrResult
		}
		if err := validateDirectUploadFile(ctx, upload.profile, file); err != nil {
			return "", err
		}
		total += file.Size
		if total > int64(providerUploadLimit()) {
			return "", channelplugin.ErrResult
		}
		header := textproto.MIMEHeader(http.Header(file.Header).Clone())
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": part.Field, "filename": file.Filename}))
		output, err := writer.CreatePart(header)
		if err != nil {
			return "", err
		}
		reader, err := file.Open()
		if err != nil {
			return "", channelplugin.ErrResult
		}
		copied, copyErr := io.Copy(output, io.LimitReader(uploadContextReader{ctx, reader}, file.Size+1))
		_ = reader.Close()
		if copyErr != nil || copied != file.Size {
			return "", channelplugin.ErrResult
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	wire.Body = buffer.Bytes()
	wire.Headers.Set("Content-Type", writer.FormDataContentType())
	return writer.FormDataContentType(), nil
}

func validateDirectUploadFile(ctx context.Context, profile string, file *multipart.FileHeader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isDirectNativeVideoProfile(profile) {
		return nil
	}
	if file.Size <= 0 || file.Size > 4<<20 {
		return channelplugin.ErrResult
	}
	typeName, _, _ := mime.ParseMediaType(file.Header.Get("Content-Type"))
	if typeName == "" || typeName == "application/octet-stream" {
		reader, err := file.Open()
		if err != nil {
			return channelplugin.ErrResult
		}
		var header [512]byte
		n, _ := reader.Read(header[:])
		_ = reader.Close()
		typeName = http.DetectContentType(header[:n])
	}
	if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp", "image/gif"}, strings.ToLower(typeName)) {
		return channelplugin.ErrResult
	}
	return nil
}

func expandDirectUploadJSON(ctx context.Context, raw json.RawMessage, upload *directProviderUpload, depth int, total *int64) (json.RawMessage, error) {
	if depth > 64 || ctx.Err() != nil {
		return nil, channelplugin.ErrResult
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, channelplugin.ErrResult
	}
	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if requestoverride.Decode(raw, &object) != nil {
			return nil, channelplugin.ErrResult
		}
		if _, exists := object["__fileRef"]; exists {
			for key := range object {
				if !slices.Contains([]string{"__fileRef", "encoding", "mimeType", "maxBytes"}, key) {
					return nil, channelplugin.ErrResult
				}
			}
			var placeholder struct {
				Ref      string `json:"__fileRef"`
				Encoding string `json:"encoding"`
				MIMEType string `json:"mimeType,omitempty"`
				MaxBytes int64  `json:"maxBytes,omitempty"`
			}
			if requestoverride.Decode(raw, &placeholder) != nil || placeholder.MaxBytes < 0 || placeholder.Encoding != "base64" && placeholder.Encoding != "dataUrl" {
				return nil, channelplugin.ErrResult
			}
			file := upload.files[placeholder.Ref]
			if file == nil || placeholder.MaxBytes > 0 && file.Size > placeholder.MaxBytes || validateDirectUploadFile(ctx, upload.profile, file) != nil {
				return nil, channelplugin.ErrResult
			}
			*total += file.Size
			if *total > int64(providerUploadLimit()) {
				return nil, channelplugin.ErrResult
			}
			reader, err := file.Open()
			if err != nil {
				return nil, channelplugin.ErrResult
			}
			body, readErr := io.ReadAll(io.LimitReader(uploadContextReader{ctx, reader}, file.Size+1))
			_ = reader.Close()
			if readErr != nil || int64(len(body)) != file.Size {
				return nil, channelplugin.ErrResult
			}
			value := base64.StdEncoding.EncodeToString(body)
			if placeholder.Encoding == "dataUrl" {
				typeName := placeholder.MIMEType
				if typeName == "" {
					typeName = file.Header.Get("Content-Type")
				}
				if isDirectNativeVideoProfile(upload.profile) && (typeName == "" || typeName == "application/octet-stream") {
					typeName = http.DetectContentType(body)
				}
				if typeName == "" {
					typeName = "application/octet-stream"
				}
				parsed, _, err := mime.ParseMediaType(typeName)
				if err != nil || strings.ContainsAny(typeName, "\r\n\x00") {
					return nil, channelplugin.ErrResult
				}
				if isDirectNativeVideoProfile(upload.profile) && !slices.Contains([]string{"image/png", "image/jpeg", "image/webp", "image/gif"}, parsed) {
					return nil, channelplugin.ErrResult
				}
				value = "data:" + typeName + ";base64," + value
			}
			return json.Marshal(value)
		}
		for key, value := range object {
			next, err := expandDirectUploadJSON(ctx, value, upload, depth+1, total)
			if err != nil {
				return nil, err
			}
			object[key] = next
		}
		return json.Marshal(object)
	case '[':
		var array []json.RawMessage
		if json.Unmarshal(raw, &array) != nil {
			return nil, channelplugin.ErrResult
		}
		for index, value := range array {
			next, err := expandDirectUploadJSON(ctx, value, upload, depth+1, total)
			if err != nil {
				return nil, err
			}
			array[index] = next
		}
		return json.Marshal(array)
	default:
		if !json.Valid(raw) {
			return nil, channelplugin.ErrResult
		}
		return slices.Clone(raw), nil
	}
}
