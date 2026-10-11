package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func isDirectMediaProfile(profile string) bool {
	return profile == "minimax-image" || profile == "modelscope-image"
}

// prepareDirectMediaProfile consumes the already selected model and parameter
// overrides. Endpoint URLs and credentials remain owned by the direct dispatcher.
func prepareDirectMediaProfile(profile, path, contentType string, body []byte) (*directProviderWire, string, error) {
	if !isDirectMediaProfile(profile) {
		return nil, "", fmt.Errorf("unsupported direct media profile")
	}
	edit := path == "/v1/images/edits"
	if path != "/v1/images/generations" && !(profile == "modelscope-image" && edit) {
		return nil, "", fmt.Errorf("%s does not support this image operation", profile)
	}
	var payload map[string]json.RawMessage
	var err error
	if isMultipartFormDataContentType(contentType) {
		if !edit {
			return nil, "", fmt.Errorf("image generation requires JSON")
		}
		payload, err = directImageMultipart(body, contentType)
	} else {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil || payload == nil {
		return nil, "", fmt.Errorf("invalid image request: %v", err)
	}
	var prompt, model string
	if json.Unmarshal(payload["prompt"], &prompt) != nil || strings.TrimSpace(prompt) == "" {
		return nil, "", fmt.Errorf("image prompt is required")
	}
	if raw, ok := payload["model"]; ok && json.Unmarshal(raw, &model) != nil {
		return nil, "", fmt.Errorf("image model must be a string")
	}
	var stream bool
	if raw, ok := payload["stream"]; ok && (json.Unmarshal(raw, &stream) != nil || stream) {
		return nil, "", fmt.Errorf("%s does not support streaming images", profile)
	}
	delete(payload, "stream")
	wire := &directProviderWire{Headers: make(http.Header), Query: make(url.Values), Profile: profile}
	wire.Headers.Set("Accept", "application/json")
	wire.Headers.Set("Content-Type", "application/json")
	if profile == "minimax-image" {
		if model == "" {
			payload["model"] = json.RawMessage(`"image-01"`)
		}
		err = prepareMiniMaxImage(payload)
	} else {
		if strings.TrimSpace(model) == "" {
			return nil, "", fmt.Errorf("ModelScope image model is required")
		}
		payload["prompt"], _ = json.Marshal(strings.TrimSpace(prompt))
		err = prepareModelScopeImage(payload, edit)
		wire.Headers.Set("X-ModelScope-Async-Mode", "true")
	}
	if err != nil {
		return nil, "", err
	}
	wire.Body, err = json.Marshal(payload)
	return wire, "application/json", err
}

func prepareMiniMaxImage(payload map[string]json.RawMessage) error {
	var size, format string
	if raw, ok := payload["size"]; ok && json.Unmarshal(raw, &size) != nil {
		return fmt.Errorf("invalid MiniMax image size")
	}
	if size != "" && (len(payload["width"]) == 0 || len(payload["height"]) == 0) {
		parts := strings.Split(size, "x")
		if len(parts) != 2 {
			return fmt.Errorf("unsupported MiniMax image size")
		}
		for index, key := range []string{"width", "height"} {
			value, err := strconv.ParseInt(parts[index], 10, 64)
			if err != nil || value < 512 || value > 2048 || value%8 != 0 {
				return fmt.Errorf("unsupported MiniMax image size")
			}
			if len(payload[key]) == 0 {
				payload[key] = json.RawMessage(strconv.FormatInt(value, 10))
			}
		}
	}
	delete(payload, "size")
	for _, key := range []string{"width", "height"} {
		if raw, ok := payload[key]; ok {
			var value int64
			if json.Unmarshal(raw, &value) != nil || value < 512 || value > 2048 || value%8 != 0 {
				return fmt.Errorf("invalid MiniMax %s", key)
			}
		}
	}
	if raw, ok := payload["response_format"]; ok && json.Unmarshal(raw, &format) != nil {
		return fmt.Errorf("invalid MiniMax response_format")
	}
	switch format {
	case "", "url":
		format = "url"
	case "b64_json", "base64":
		format = "base64"
	default:
		return fmt.Errorf("unsupported MiniMax response_format")
	}
	payload["response_format"], _ = json.Marshal(format)
	return nil
}

func prepareModelScopeImage(payload map[string]json.RawMessage, edit bool) error {
	if raw := payload["mask"]; len(raw) > 0 && !bytes.Equal(raw, []byte(`""`)) && !bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("ModelScope image requests do not support masks")
	}
	delete(payload, "mask")
	var size string
	if raw, ok := payload["size"]; ok && json.Unmarshal(raw, &size) != nil {
		return fmt.Errorf("invalid ModelScope image size")
	}
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		size = "2048x2048"
	}
	payload["size"], _ = json.Marshal(size)
	delete(payload, "response_format") // Native tasks return URLs; the response owner materializes base64.
	var images []string
	for _, key := range []string{"image", "images", "image_url"} {
		raw, exists := payload[key]
		if !exists {
			continue
		}
		var single string
		if json.Unmarshal(raw, &single) == nil {
			images = append(images, single)
		} else {
			var entries []json.RawMessage
			if json.Unmarshal(raw, &entries) != nil {
				return fmt.Errorf("invalid ModelScope reference images")
			}
			for _, entry := range entries {
				var image string
				if json.Unmarshal(entry, &image) != nil {
					var object struct {
						URL string `json:"image_url"`
					}
					if json.Unmarshal(entry, &object) != nil {
						return fmt.Errorf("invalid ModelScope reference image")
					}
					image = object.URL
				}
				images = append(images, image)
			}
		}
		delete(payload, key)
	}
	if edit && len(images) == 0 {
		return fmt.Errorf("ModelScope image edits require a reference image")
	}
	if len(images) > defaultMaxMultipartFiles {
		return fmt.Errorf("too many ModelScope reference images")
	}
	for _, image := range images {
		if strings.HasPrefix(image, "data:") {
			if _, err := directImageDataURL(image); err != nil {
				return err
			}
		} else {
			u, err := url.Parse(image)
			if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("invalid ModelScope reference image URL")
			}
		}
	}
	if len(images) == 1 {
		payload["image_url"], _ = json.Marshal(images[0])
	}
	if len(images) > 1 {
		payload["image_url"], _ = json.Marshal(images)
	}
	return nil
}

func directImageMultipart(body []byte, contentType string) (map[string]json.RawMessage, error) {
	req, err := http.NewRequest(http.MethodPost, "http://multipart.invalid", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	form, err := ParseMultipartFormData(req)
	if req.MultipartForm != nil {
		defer req.MultipartForm.RemoveAll()
	}
	if err != nil {
		return nil, err
	}
	if form == nil {
		return nil, fmt.Errorf("missing image multipart form")
	}
	payload := map[string]json.RawMessage{}
	for key, values := range form.Values {
		if len(values) != 1 {
			return nil, fmt.Errorf("image field %s must have one value", key)
		}
		value := values[0]
		switch key {
		case "n", "seed", "width", "height", "steps", "guidance_scale", "prompt_optimizer", "aigc_watermark", "stream":
			if !json.Valid([]byte(value)) {
				return nil, fmt.Errorf("invalid image field %s", key)
			}
			payload[key] = json.RawMessage(value)
		default:
			payload[key], _ = json.Marshal(value)
		}
	}
	images := []string{}
	fileField := ""
	for key, files := range form.Files {
		if len(files) == 0 {
			continue
		}
		if key == "mask" {
			return nil, fmt.Errorf("image requests do not support mask files")
		}
		if key != "image" && key != "image[]" && key != "images" {
			return nil, fmt.Errorf("unsupported image file field %s", key)
		}
		if fileField != "" {
			return nil, fmt.Errorf("use only one reference image file field")
		}
		fileField = key
	}
	if fileField != "" {
		for _, key := range []string{"image", "image[]", "images", "image_url"} {
			if len(form.Values[key]) > 0 {
				return nil, fmt.Errorf("use reference image text fields or uploaded files, not both")
			}
		}
	}
	for _, file := range form.Files[fileField] {
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, defaultMaxMultipartFileBytes+1))
		reader.Close()
		if err != nil {
			return nil, err
		}
		if len(data) == 0 || int64(len(data)) > defaultMaxMultipartFileBytes {
			return nil, fmt.Errorf("invalid reference image size")
		}
		mediaType := http.DetectContentType(data)
		if !strings.HasPrefix(mediaType, "image/") {
			mediaType = "image/png"
		}
		images = append(images, "data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	if len(images) > 0 {
		payload["image"], _ = json.Marshal(images)
	}
	return payload, nil
}

func directImageDataURL(value string) (string, error) {
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || data == "" {
		return "", fmt.Errorf("invalid base64 image data URL")
	}
	if len(data) > base64.StdEncoding.EncodedLen(int(defaultMaxMultipartFileBytes)) {
		return "", fmt.Errorf("image data URL exceeds size limit")
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return "", fmt.Errorf("invalid base64 image data URL")
	}
	return data, nil
}
