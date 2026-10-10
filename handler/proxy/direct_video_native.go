package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func isDirectNativeVideoProfile(profile string) bool {
	return profile == "seedance-video" || profile == "zenmux-video"
}

// Conversion runs after model mapping and parameter overrides. URLs and
// credentials remain owned by the existing direct endpoint dispatcher.
func prepareDirectNativeVideoProfile(profile, method, path, contentType string, body []byte) (*directProviderWire, string, error) {
	if !isDirectNativeVideoProfile(profile) || !strings.HasPrefix(path, "/v1/videos") {
		return nil, "", fmt.Errorf("unsupported native video operation")
	}
	if (method == http.MethodPost && strings.HasSuffix(path, "/remix")) || (profile == "zenmux-video" && method == http.MethodDelete) {
		return nil, "", fmt.Errorf("%s does not support this video operation", profile)
	}
	wire := &directProviderWire{Profile: profile, Body: body, Headers: make(http.Header), Query: make(url.Values)}
	wire.Headers.Set("Accept", "application/json")
	if method != http.MethodPost {
		return wire, contentType, nil
	}
	if path != "/v1/videos" {
		return nil, "", fmt.Errorf("unsupported native video creation path")
	}
	var payload map[string]json.RawMessage
	var err error
	if isMultipartFormDataContentType(contentType) {
		payload, err = directVideoMultipart(body, contentType)
	} else {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil || payload == nil {
		return nil, "", fmt.Errorf("invalid video request")
	}
	if err := prepareNativeVideoPayload(payload, profile); err != nil {
		return nil, "", err
	}
	wire.Body, err = json.Marshal(payload)
	wire.Headers.Set("Content-Type", "application/json")
	return wire, "application/json", err
}

func prepareNativeVideoPayload(payload map[string]json.RawMessage, profile string) error {
	if mediaString(payload, "model") == "" {
		return fmt.Errorf("video model is required")
	}
	if raw := payload["stream"]; len(raw) != 0 && !bytes.Equal(raw, []byte("false")) {
		return fmt.Errorf("native video streaming is unsupported")
	}
	delete(payload, "stream")
	var extra map[string]json.RawMessage
	if raw, ok := payload["extra_body"]; ok {
		if json.Unmarshal(raw, &extra) != nil || extra == nil {
			return fmt.Errorf("video extra_body must be an object")
		}
		delete(payload, "extra_body")
	}
	unsupported := []string{"callback_url", "return_last_frame", "tools"}
	if profile == "zenmux-video" {
		unsupported = []string{"service_tier", "execution_expires_after"}
	}
	for _, key := range unsupported {
		_, exists := payload[key]
		_, extraExists := extra[key]
		if exists || extraExists {
			return fmt.Errorf("%s is unsupported for %s", key, profile)
		}
	}
	var content []json.RawMessage
	if raw, exists := payload["content"]; exists && json.Unmarshal(raw, &content) != nil {
		return fmt.Errorf("video content must be an array")
	}
	if len(content) == 0 {
		prompt := mediaString(payload, "prompt")
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("video prompt or content is required")
		}
		text, _ := json.Marshal(map[string]any{"type": "text", "text": prompt})
		content = append(content, text)
		if raw, exists := payload["input_reference"]; exists {
			var reference string
			if json.Unmarshal(raw, &reference) != nil || strings.TrimSpace(reference) == "" {
				return fmt.Errorf("invalid video input_reference")
			}
			image, _ := json.Marshal(map[string]any{"type": "image_url", "image_url": map[string]string{"url": reference}, "role": "first_frame"})
			content = append(content, image)
		}
	}
	for i, entry := range content {
		if err := validateNativeVideoContent(entry, profile); err != nil {
			return fmt.Errorf("video content[%d]: %w", i, err)
		}
	}
	payload["content"], _ = json.Marshal(content)
	delete(payload, "prompt")
	delete(payload, "input_reference")
	if raw, exists := payload["seconds"]; exists {
		if _, duplicate := payload["duration"]; duplicate {
			return fmt.Errorf("video seconds and duration cannot both be specified")
		}
		var seconds string
		if json.Unmarshal(raw, &seconds) != nil {
			return fmt.Errorf("video seconds must be a string")
		}
		duration, err := strconv.ParseFloat(strings.TrimSpace(seconds), 64)
		automaticDuration := profile == "seedance-video" && duration == -1
		if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || (duration <= 0 && !automaticDuration) || duration >= math.MaxInt64 ||
			(profile == "zenmux-video" && math.Trunc(duration) != duration) {
			return fmt.Errorf("invalid video duration")
		}
		if math.Round(duration) <= 0 && !automaticDuration {
			return fmt.Errorf("invalid video duration")
		}
		payload["duration"] = json.RawMessage(strconv.FormatInt(int64(math.Round(duration)), 10))
		delete(payload, "seconds")
	}
	// Explicit model/content and converted seconds win over native extras, as
	// in the source transformer. Extras must not replace a mapped model or prompt.
	for key, value := range extra {
		if _, present := payload[key]; !present {
			payload[key] = value
		}
	}
	for _, key := range []string{"duration", "frames", "seed", "execution_expires_after"} {
		if raw, exists := payload[key]; exists {
			var value int64
			if json.Unmarshal(raw, &value) != nil || (key != "seed" && value <= 0 && !(key == "duration" && profile == "seedance-video" && value == -1)) {
				return fmt.Errorf("invalid video %s", key)
			}
		}
	}
	for _, key := range []string{"generate_audio", "camera_fixed", "watermark", "draft", "return_last_frame"} {
		if raw, exists := payload[key]; exists {
			var value bool
			if json.Unmarshal(raw, &value) != nil || bytes.Equal(raw, []byte("null")) {
				return fmt.Errorf("video %s must be a boolean", key)
			}
		}
	}
	if size := mediaString(payload, "size"); size != "" && mediaString(payload, "ratio") == "" && mediaString(payload, "resolution") == "" {
		mapping := map[string][2]string{
			"1280x720": {"16:9", "720p"}, "720x1280": {"9:16", "720p"},
			"1920x1080": {"16:9", "1080p"}, "1080x1920": {"9:16", "1080p"},
			"640x480": {"4:3", "480p"}, "480x640": {"3:4", "480p"},
		}
		parts, ok := mapping[strings.ToLower(strings.ReplaceAll(strings.TrimSpace(size), " ", ""))]
		if !ok {
			return fmt.Errorf("video size cannot be mapped to ratio/resolution")
		}
		payload["ratio"], _ = json.Marshal(parts[0])
		payload["resolution"], _ = json.Marshal(parts[1])
	}
	delete(payload, "size")
	return nil
}

func validateNativeVideoContent(raw json.RawMessage, profile string) error {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil || item == nil {
		return fmt.Errorf("invalid content object")
	}
	kind, role := mediaString(item, "type"), mediaString(item, "role")
	allowed := map[string]bool{"type": true, "role": true, kind: true}
	for key := range item {
		if !allowed[key] {
			return fmt.Errorf("unexpected %s on %s content", key, kind)
		}
	}
	if kind == "text" {
		if strings.TrimSpace(mediaString(item, "text")) == "" || role != "" {
			return fmt.Errorf("invalid text content")
		}
		return nil
	}
	switch kind {
	case "image_url":
		if role != "" && role != "first_frame" && role != "last_frame" && role != "reference_image" {
			return fmt.Errorf("invalid image role")
		}
	case "video_url":
		if role != "reference_video" {
			return fmt.Errorf("invalid video role")
		}
	case "audio_url":
		if role != "reference_audio" {
			return fmt.Errorf("invalid audio role")
		}
	default:
		return fmt.Errorf("unsupported content type")
	}
	var media map[string]json.RawMessage
	if json.Unmarshal(item[kind], &media) != nil || strings.TrimSpace(mediaString(media, "url")) == "" {
		return fmt.Errorf("invalid reference URL")
	}
	if profile == "zenmux-video" && mediaString(media, "mime_type") != "" {
		return fmt.Errorf("ZenMux reference MIME type is unsupported")
	}
	return nil
}

func directVideoMultipart(body []byte, contentType string) (map[string]json.RawMessage, error) {
	req, err := http.NewRequest(http.MethodPost, "http://multipart.invalid", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	form, err := ParseMultipartFormData(req)
	if req.MultipartForm != nil {
		defer req.MultipartForm.RemoveAll()
	}
	if err != nil || form == nil {
		return nil, fmt.Errorf("invalid video multipart form")
	}
	payload := map[string]json.RawMessage{}
	for key, values := range form.Values {
		if len(values) != 1 {
			return nil, fmt.Errorf("video field %s must have one value", key)
		}
		switch key {
		case "model", "prompt", "input_reference", "seconds", "size", "ratio", "resolution", "service_tier", "callback_url":
			payload[key], _ = json.Marshal(values[0])
		default:
			if !json.Valid([]byte(values[0])) {
				return nil, fmt.Errorf("video field %s must be JSON", key)
			}
			payload[key] = json.RawMessage(values[0])
		}
	}
	for key, files := range form.Files {
		if key != "input_reference" || len(files) != 1 || len(payload[key]) != 0 {
			return nil, fmt.Errorf("video supports one input_reference file")
		}
		file, err := files[0].Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		_ = file.Close()
		if err != nil || len(data) == 0 || len(data) > 4<<20 {
			return nil, fmt.Errorf("invalid video reference file size")
		}
		mime := strings.ToLower(strings.TrimSpace(strings.Split(files[0].Header.Get("Content-Type"), ";")[0]))
		if mime == "" || mime == "application/octet-stream" {
			mime = http.DetectContentType(data)
		}
		if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" && mime != "image/gif" {
			return nil, fmt.Errorf("unsupported video reference image type")
		}
		payload[key], _ = json.Marshal("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data))
	}
	return payload, nil
}
