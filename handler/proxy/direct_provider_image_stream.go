package proxyhandler

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/tidwall/gjson"
)

type imageTaskStreamOptions struct {
	prefix       string
	partialLimit int
	settings     map[string]string
}

func newImageTaskStreamOptions(protocol string, body []byte, multipart ...bool) (*imageTaskStreamOptions, error) {
	options := &imageTaskStreamOptions{prefix: "image_generation", settings: map[string]string{"background": "auto", "output_format": "png", "quality": "auto", "size": "auto"}}
	if protocol == "imageEdit" {
		options.prefix = "image_edit"
	}
	if protocol != "imageGeneration" && protocol != "imageEdit" && protocol != "modelscopeImageGeneration" {
		return nil, channelplugin.ErrResult
	}
	if !imageStreamKeys(body) {
		return nil, channelplugin.ErrResult
	}
	for key := range options.settings {
		value := gjson.GetBytes(body, key)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if value.Type != gjson.String || !validImageStreamSetting(key, value.String()) {
			return nil, channelplugin.ErrResult
		}
		options.settings[key] = value.String()
	}
	for _, setting := range []struct {
		key                 string
		fallback, low, high int
	}{{"partial_images", 0, 0, 3}, {"n", 1, 1, 10}} {
		value := gjson.GetBytes(body, setting.key)
		number := int64(setting.fallback)
		if value.Exists() && value.Type != gjson.Null {
			var err error
			raw := value.Raw
			formNumber := len(multipart) > 0 && multipart[0] && value.Type == gjson.String
			if formNumber {
				raw = value.String()
			}
			number, err = strconv.ParseInt(raw, 10, 64)
			if value.Type != gjson.Number && !formNumber || err != nil {
				return nil, channelplugin.ErrResult
			}
		}
		if number < int64(setting.low) || number > int64(setting.high) {
			return nil, channelplugin.ErrResult
		}
		if setting.key == "partial_images" {
			options.partialLimit = int(number)
		}
	}
	if value := gjson.GetBytes(body, "response_format"); value.Exists() && value.Type != gjson.Null && (value.Type != gjson.String || value.String() != "" && value.String() != "b64_json") {
		return nil, channelplugin.ErrResult
	}
	return options, nil
}

func validImageStreamSetting(key, value string) bool {
	switch key {
	case "background":
		return value == "auto" || value == "opaque" || value == "transparent"
	case "output_format":
		return value == "png" || value == "webp" || value == "jpeg"
	case "quality":
		return value == "auto" || value == "low" || value == "medium" || value == "high" || value == "xhigh" || value == "max"
	case "size":
		return value != "" && len(value) <= 128 && !strings.ContainsAny(value, "\r\n\x00")
	}
	return false
}

func imageStreamKeys(body []byte) bool {
	if requestoverride.RejectDuplicateJSONKeys(body) != nil {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return false
	}
	for key := range object {
		for _, canonical := range []string{"stream", "partial_images", "partial_image_index", "n", "b64_json", "created", "created_at", "data", "background", "output_format", "quality", "size", "type", "usage", "error", "model", "response_format"} {
			if key != canonical && strings.EqualFold(key, canonical) {
				return false
			}
		}
	}
	return true
}

func (o *imageTaskStreamOptions) event(payload map[string]json.RawMessage, kind string, index *int) ([]byte, error) {
	var encoded string
	if json.Unmarshal(payload["b64_json"], &encoded) != nil {
		return nil, channelplugin.ErrResult
	}
	encoded, err := directCodexImageBase64(encoded)
	if err != nil {
		return nil, channelplugin.ErrResult
	}
	payload["b64_json"], _ = json.Marshal(encoded)
	if value, ok := payload["type"]; ok && string(value) != "null" {
		var actual string
		if json.Unmarshal(value, &actual) != nil || actual != kind {
			return nil, channelplugin.ErrResult
		}
	}
	payload["type"], _ = json.Marshal(kind)
	if index != nil {
		if value, ok := payload["partial_image_index"]; ok {
			actual, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil || actual != int64(*index) {
				return nil, channelplugin.ErrResult
			}
		}
		payload["partial_image_index"], _ = json.Marshal(*index)
	} else if _, exists := payload["partial_image_index"]; exists {
		return nil, channelplugin.ErrResult
	}
	if created, exists := payload["created_at"]; exists {
		value, err := strconv.ParseInt(string(created), 10, 64)
		if err != nil || value < 0 {
			return nil, channelplugin.ErrResult
		}
	} else {
		payload["created_at"], _ = json.Marshal(time.Now().Unix())
	}
	for key, fallback := range o.settings {
		value := fallback
		if raw, exists := payload[key]; exists && (string(raw) == "null" || json.Unmarshal(raw, &value) != nil) {
			return nil, channelplugin.ErrResult
		}
		if !validImageStreamSetting(key, value) {
			return nil, channelplugin.ErrResult
		}
		payload[key], _ = json.Marshal(value)
	}
	raw, err := json.Marshal(payload)
	if err != nil || nativeDriverError(raw) || !imageStreamKeys(raw) {
		return nil, channelplugin.ErrResult
	}
	return []byte("event: " + kind + "\ndata: " + string(raw) + "\n\n"), nil
}

func (o *imageTaskStreamOptions) partial(index int, body []byte) ([]byte, error) {
	if index < 0 || index >= o.partialLimit || !imageStreamKeys(body) {
		return nil, channelplugin.ErrResult
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil || payload == nil {
		return nil, channelplugin.ErrResult
	}
	// Partial image events do not report final usage. Metering remains the
	// original supplier response, not a counter supplied by this projection.
	if _, present := payload["usage"]; present {
		return nil, channelplugin.ErrResult
	}
	return o.event(payload, o.prefix+".partial_image", &index)
}

func (o *imageTaskStreamOptions) completed(body []byte) ([][]byte, error) {
	if !imageStreamKeys(body) {
		return nil, channelplugin.ErrResult
	}
	var root map[string]json.RawMessage
	var data []map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil || json.Unmarshal(root["data"], &data) != nil || len(data) == 0 || len(data) > 10 {
		return nil, channelplugin.ErrResult
	}
	if created, exists := root["created_at"]; exists && !bytes.Equal(created, root["created"]) {
		return nil, channelplugin.ErrResult
	}
	root["created_at"] = root["created"]
	delete(root, "created")
	delete(root, "data")
	var frames [][]byte
	var total int64
	for _, image := range data {
		payload := make(map[string]json.RawMessage, len(root)+len(image))
		for key, value := range root {
			payload[key] = value
		}
		for key, value := range image {
			switch key {
			case "usage", "type", "created_at", "model", "error":
				if !bytes.Equal(value, root[key]) {
					return nil, channelplugin.ErrResult
				}
			}
			payload[key] = value
		}
		delete(payload, "url")
		frame, err := o.event(payload, o.prefix+".completed", nil)
		if err != nil {
			return nil, err
		}
		total += int64(len(frame))
		if total > streamResponseByteLimit() {
			return nil, errMessagesChatStreamLimit
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// Image bytes can be multi-MiB. The shared frame reader uses the stream byte
// limit, while its original analyzer receives only bounded metadata and the
// supplier observation. No image or private prompt is retained in that analyzer.
type directImageTaskEventStream struct {
	reader *messagesChatBody
}

func newDirectImageTaskSSEBody(body io.ReadCloser, onFirstOutput func()) *messagesChatBody {
	stream := &directImageTaskEventStream{}
	reader := newProtocolBridgeBody(body, stream, streamResponseByteLimit())
	reader.frameLimit = streamResponseByteLimit()
	reader.skipOriginalAnalysis = true
	reader.original.onFirstOutput = onFirstOutput
	stream.reader = reader
	return reader
}

func (s *directImageTaskEventStream) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event != nil && event.Data != "" {
		body := []byte(event.Data)
		kind := gjson.GetBytes(body, "type").String()
		metadata, _ := json.Marshal(map[string]string{"type": kind})
		if event.Event == "error" {
			metadata = []byte(`{"type":"error","error":{"message":"Image task query failed"}}`)
		}
		s.reader.original.recordEvent(SseEvent{Data: string(metadata)})
		if event.Event != "error" && gjson.GetBytes(body, "b64_json").Type == gjson.String && gjson.GetBytes(body, "b64_json").String() != "" {
			s.reader.original.result.HasGeneratedOutput = true
			if s.reader.original.onFirstOutput != nil {
				s.reader.original.onFirstOutput()
				s.reader.original.onFirstOutput = nil
			}
		}
	}
	return frame, nil
}

func (*directImageTaskEventStream) Finish() ([]byte, error) { return nil, nil }
