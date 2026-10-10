package proxyhandler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// prepareDirectCodexImagesRequest consumes the original Images representation.
// Credentials are deliberately absent; the regular direct dispatcher resolves
// and refreshes them after conversion. model is the selected image tool model.
func prepareDirectCodexImagesRequest(r *http.Request, ctx *Ctx, endpoint *store.DirectEndpoint, model string, body []byte) ([]byte, error) {
	if endpoint == nil || endpoint.Profile != "codex-image" || strings.TrimSpace(endpoint.RequestModel) == "" {
		return nil, fmt.Errorf("Codex images require a Responses request model")
	}
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	action := "generate"
	switch strings.TrimRight(ctx.DownstreamPath, "/") {
	case "/v1/images/generations":
	case "/v1/images/edits":
		action = "edit"
	default:
		return nil, fmt.Errorf("Codex image tools only support generation and editing")
	}
	var fields map[string]json.RawMessage
	var err error
	if ctx.Multipart {
		fields, err = directCodexImageForm(r)
	} else if json.Unmarshal(body, &fields) != nil || fields == nil {
		err = fmt.Errorf("invalid image JSON request")
	}
	if err != nil {
		return nil, err
	}
	tool := map[string]any{"type": "image_generation", "model": model, "action": action}
	var prompt string
	if json.Unmarshal(fields["prompt"], &prompt) != nil || strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("image prompt is required")
	}
	for key, value := range fields {
		switch key {
		case "prompt", "image", "images", "mask":
		case "model":
			var valueString string
			if json.Unmarshal(value, &valueString) != nil || string(value) == "null" {
				return nil, fmt.Errorf("image model must be a string")
			}
		case "stream":
			var stream bool
			if json.Unmarshal(value, &stream) != nil || string(value) == "null" || stream != ctx.IsStream {
				return nil, fmt.Errorf("image stream must be a boolean")
			}
		case "n", "output_compression", "partial_images":
			var number int
			if json.Unmarshal(value, &number) != nil || string(value) == "null" {
				return nil, fmt.Errorf("image %s must be an integer", key)
			}
			if key == "n" {
				if number != 1 {
					return nil, fmt.Errorf("Codex image tools only support n=1")
				}
				continue
			}
			if number < 0 || key == "output_compression" && number > 100 || key == "partial_images" && (number > 3 || number > 0 && !ctx.IsStream) {
				return nil, fmt.Errorf("unsupported image %s value", key)
			}
			tool[key] = number
		case "response_format":
			var format string
			if json.Unmarshal(value, &format) != nil || format != "b64_json" {
				return nil, fmt.Errorf("Codex image tools only support response_format=b64_json")
			}
		case "background", "input_fidelity", "moderation", "output_format", "quality", "size":
			var option string
			if json.Unmarshal(value, &option) != nil || strings.TrimSpace(option) == "" {
				return nil, fmt.Errorf("image %s must be a nonempty string", key)
			}
			tool[key] = option
		default:
			return nil, fmt.Errorf("image parameter %q cannot be represented by the Codex image tool", key)
		}
	}
	if _, one := fields["image"]; one && fields["images"] != nil {
		return nil, fmt.Errorf("specify either image or images, not both")
	}
	images, err := directCodexImageReferences(fields["image"], false)
	if err == nil && fields["images"] != nil {
		images, err = directCodexImageReferences(fields["images"], true)
	}
	if err != nil {
		return nil, err
	}
	if action == "edit" && len(images) == 0 {
		return nil, fmt.Errorf("image editing requires at least one reference image")
	}
	maxFiles := positiveEnvInt("PROXY_MAX_MULTIPART_FILES", defaultMaxMultipartFiles)
	if len(images) > maxFiles {
		return nil, fmt.Errorf("%w: too many reference images", ErrMultipartLimitExceeded)
	}
	content := []any{map[string]any{"type": "input_text", "text": prompt}}
	for _, imageURL := range images {
		content = append(content, map[string]any{"type": "input_image", "image_url": imageURL})
	}
	if rawMask := fields["mask"]; rawMask != nil {
		var mask string
		if json.Unmarshal(rawMask, &mask) != nil || len(images) == 0 || action != "edit" {
			return nil, fmt.Errorf("mask requires an image edit and a single data URL")
		}
		if err := validateDirectCodexImageURL(mask); err != nil {
			return nil, err
		}
		if len(images)+1 > maxFiles {
			return nil, fmt.Errorf("%w: too many reference images and masks", ErrMultipartLimitExceeded)
		}
		tool["input_image_mask"] = map[string]string{"image_url": mask}
	}
	return json.Marshal(map[string]any{
		"model":        endpoint.RequestModel,
		"input":        []any{map[string]any{"role": "user", "content": content}},
		"instructions": "You are a helpful assistant that generates images. Use the image generation tool to fulfill the request.",
		"tools":        []any{tool}, "tool_choice": "required", "stream": true, "store": false,
	})
}

func directCodexImageReferences(raw json.RawMessage, objects bool) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var one string
	if !objects && json.Unmarshal(raw, &one) == nil && string(raw) != "null" {
		if err := validateDirectCodexImageURL(one); err != nil {
			return nil, err
		}
		return []string{one}, nil
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || string(raw) == "null" {
		return nil, fmt.Errorf("image references must be data URLs or an array of data URLs")
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		var value string
		if json.Unmarshal(entry, &value) != nil {
			var obj map[string]json.RawMessage
			if !objects || json.Unmarshal(entry, &obj) != nil || len(obj) != 1 || json.Unmarshal(obj["image_url"], &value) != nil {
				return nil, fmt.Errorf("images entries must be data URLs or image_url objects")
			}
		}
		if err := validateDirectCodexImageURL(value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func directCodexImageMIME(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func validateDirectCodexImageURL(value string) error {
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") || !directCodexImageMIME(strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")) {
		return fmt.Errorf("image references require a PNG, JPEG, WebP or GIF base64 data URL")
	}
	limit := positiveEnvInt64("PROXY_MAX_MULTIPART_FILE_BYTES", defaultMaxMultipartFileBytes)
	if int64(len(data)) > (limit+2)/3*4 {
		return fmt.Errorf("%w: reference image is too large", ErrMultipartLimitExceeded)
	}
	n, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
	if err != nil || n == 0 {
		return fmt.Errorf("invalid base64 reference image")
	}
	if n > limit {
		return fmt.Errorf("%w: reference image is too large", ErrMultipartLimitExceeded)
	}
	return nil
}

func directCodexImageForm(r *http.Request) (map[string]json.RawMessage, error) {
	if r.MultipartForm == nil {
		return nil, fmt.Errorf("image multipart form was not parsed")
	}
	if err := validateMultipartForm(r.MultipartForm); err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	for key, values := range r.MultipartForm.Value {
		if len(values) != 1 {
			return nil, fmt.Errorf("image field %q must occur once", key)
		}
		value := values[0]
		switch key {
		case "n", "output_compression", "partial_images":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("image %s must be an integer", key)
			}
			fields[key], _ = json.Marshal(n)
		case "stream":
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("image stream must be true or false")
			}
			fields[key] = json.RawMessage(value)
		default:
			fields[key], _ = json.Marshal(value)
		}
	}
	var images []string
	if len(r.MultipartForm.File["image"]) > 0 && len(r.MultipartForm.File["image[]"]) > 0 {
		return nil, fmt.Errorf("use only one image file field name")
	}
	for _, key := range []string{"image", "image[]", "mask"} {
		for _, fh := range r.MultipartForm.File[key] {
			if err := r.Context().Err(); err != nil {
				return nil, err
			}
			if fh == nil {
				return nil, fmt.Errorf("invalid image file")
			}
			file, err := fh.Open()
			if err != nil {
				return nil, fmt.Errorf("cannot open image file")
			}
			limit := positiveEnvInt64("PROXY_MAX_MULTIPART_FILE_BYTES", defaultMaxMultipartFileBytes)
			data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
			_ = file.Close()
			if readErr != nil {
				return nil, fmt.Errorf("cannot read image file")
			}
			if int64(len(data)) > limit {
				return nil, fmt.Errorf("%w: image file is too large", ErrMultipartLimitExceeded)
			}
			media := http.DetectContentType(data)
			if !directCodexImageMIME(media) {
				return nil, fmt.Errorf("image files must contain PNG, JPEG, WebP or GIF data")
			}
			encoded := "data:" + media + ";base64," + base64.StdEncoding.EncodeToString(data)
			if key == "mask" {
				if fields["mask"] != nil {
					return nil, fmt.Errorf("image mask must occur once")
				}
				fields["mask"], _ = json.Marshal(encoded)
			} else {
				images = append(images, encoded)
			}
		}
	}
	for key := range r.MultipartForm.File {
		if key != "image" && key != "image[]" && key != "mask" {
			return nil, fmt.Errorf("unsupported image file field %q", key)
		}
	}
	if len(images) > 0 {
		if fields["image"] != nil || fields["images"] != nil {
			return nil, fmt.Errorf("cannot mix image files and image fields")
		}
		fields["image"], _ = json.Marshal(images)
	}
	return fields, nil
}
