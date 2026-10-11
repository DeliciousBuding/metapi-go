package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
)

type directCodexImageItem struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Status        string          `json:"status"`
	Result        string          `json:"result"`
	RevisedPrompt string          `json:"revised_prompt"`
	Background    string          `json:"background"`
	OutputFormat  string          `json:"output_format"`
	Quality       string          `json:"quality"`
	Size          string          `json:"size"`
	Error         json.RawMessage `json:"error"`
}

// The converter owns image semantics, but no network state or credentials.
// A tool result is provisional until response.completed confirms the request.
type directCodexImagesStream struct {
	stream    bool
	prefix    string
	created   int64
	item      *directCodexImageItem
	itemIndex int
	terminal  bool
	result    []byte
	observe   func(SseEvent)
	onOutput  func()
}

func newDirectCodexImagesStream(path string, stream bool) *directCodexImagesStream {
	prefix := "image_generation"
	if strings.HasSuffix(strings.TrimRight(path, "/"), "/edits") {
		prefix = "image_edit"
	}
	return &directCodexImagesStream{stream: stream, prefix: prefix}
}

// Image frames may contain megabytes of base64. Only this bridge raises the
// per-frame limit to the bounded stream budget. The normal Chat limit remains.
func newDirectCodexImagesBody(body io.ReadCloser, path string, stream bool, byteLimit int64) *messagesChatBody {
	converter := newDirectCodexImagesStream(path, stream)
	reader := newProtocolBridgeBody(body, converter, byteLimit)
	reader.frameLimit = byteLimit
	reader.skipOriginalAnalysis = true
	converter.observe = reader.original.recordEvent
	converter.onOutput = func() {
		reader.original.result.HasGeneratedOutput = true
		if reader.original.onFirstOutput != nil {
			reader.original.onFirstOutput()
			reader.original.onFirstOutput = nil
		}
	}
	return reader
}

func (s *directCodexImagesStream) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event == nil || event.Data == "" || event.Data == "[DONE]" {
		return nil, nil
	}
	var payload struct {
		Type              string               `json:"type"`
		OutputIndex       int                  `json:"output_index"`
		Item              directCodexImageItem `json:"item"`
		PartialImage      string               `json:"partial_image_b64"`
		PartialImageIndex int                  `json:"partial_image_index"`
		Usage             json.RawMessage      `json:"usage"`
		Created           int64                `json:"created_at"`
		Background        string               `json:"background"`
		OutputFormat      string               `json:"output_format"`
		Quality           string               `json:"quality"`
		Size              string               `json:"size"`
		Response          struct {
			Status  string                 `json:"status"`
			Created int64                  `json:"created_at"`
			Output  []directCodexImageItem `json:"output"`
			Error   json.RawMessage        `json:"error"`
			Usage   json.RawMessage        `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal([]byte(event.Data), &payload) != nil {
		return nil, fmt.Errorf("invalid Codex image response event")
	}
	if event.Event == "error" {
		payload.Type = "error"
	}
	if s.terminal {
		return nil, fmt.Errorf("unexpected event after Codex image completion")
	}
	if s.observe != nil {
		// Never retain provider text, tokens, image bytes, or raw error details
		// in the generic analyzer. Its billing input is actual upstream usage.
		metadata := map[string]any{"type": payload.Type, "usage": payload.Usage, "response": map[string]any{"usage": payload.Response.Usage}}
		switch payload.Type {
		case "error", "response.failed", "response.incomplete", "response.image_generation_call.failed":
			metadata["error"] = map[string]string{"message": "Codex image upstream did not complete the request"}
		}
		raw, _ := json.Marshal(metadata)
		s.observe(SseEvent{Data: string(raw)})
	}
	switch payload.Type {
	case "error", "response.failed", "response.incomplete", "response.image_generation_call.failed":
		return nil, fmt.Errorf("Codex image upstream did not complete the request")
	case "response.created":
		s.created = payload.Response.Created
	case "response.image_generation_call.partial_image":
		if payload.PartialImageIndex < 0 || payload.PartialImageIndex > 2 {
			return nil, fmt.Errorf("invalid Codex partial image index")
		}
		image, err := directCodexImageBase64(payload.PartialImage)
		if err != nil {
			return nil, err
		}
		if s.stream {
			if s.onOutput != nil {
				s.onOutput()
			}
			created := s.created
			if payload.Created != 0 {
				created = payload.Created
			}
			partial := map[string]any{"created_at": created, "b64_json": image, "partial_image_index": payload.PartialImageIndex}
			for key, value := range map[string]string{"background": payload.Background, "output_format": payload.OutputFormat, "quality": payload.Quality, "size": payload.Size} {
				if value != "" {
					partial[key] = value
				}
			}
			return directCodexImageEvent(s.prefix+".partial_image", partial), nil
		}
	case "response.output_item.done":
		if payload.Item.Type != "image_generation_call" {
			return nil, nil
		}
		if payload.OutputIndex < 0 || payload.OutputIndex > 4095 || s.item != nil {
			return nil, fmt.Errorf("unexpected additional Codex image result")
		}
		if err := validateDirectCodexImageItem(payload.Item); err != nil {
			return nil, err
		}
		s.item, s.itemIndex = &payload.Item, payload.OutputIndex
	case "response.completed":
		response := payload.Response
		if response.Status != "completed" || directCodexImageHasError(response.Error) {
			return nil, fmt.Errorf("Codex image response was not completed")
		}
		if response.Created != 0 {
			s.created = response.Created
		}
		var terminalItem *directCodexImageItem
		for index, item := range response.Output {
			if item.Type != "image_generation_call" {
				continue
			}
			if terminalItem != nil || s.item != nil && (index != s.itemIndex || s.item.ID != "" && item.ID != s.item.ID) {
				return nil, fmt.Errorf("unexpected additional Codex image result")
			}
			if err := validateDirectCodexImageItem(item); err != nil {
				return nil, err
			}
			terminalItem = &item
		}
		if terminalItem != nil {
			s.item = terminalItem
		}
		if s.item == nil {
			return nil, fmt.Errorf("Codex response did not contain an image result")
		}
		encoded, err := directCodexImageBase64(s.item.Result)
		if err != nil {
			return nil, err
		}
		image := map[string]any{"b64_json": encoded}
		if s.item.RevisedPrompt != "" {
			image["revised_prompt"] = s.item.RevisedPrompt
		}
		result := map[string]any{"created": s.created, "data": []any{image}}
		metadata := map[string]any{"created_at": s.created, "b64_json": encoded}
		for key, value := range map[string]string{"background": s.item.Background, "output_format": s.item.OutputFormat, "quality": s.item.Quality, "size": s.item.Size, "revised_prompt": s.item.RevisedPrompt} {
			if value != "" {
				metadata[key] = value
				if key != "revised_prompt" {
					result[key] = value
				}
			}
		}
		if len(response.Usage) > 0 && string(response.Usage) != "null" {
			result["usage"], metadata["usage"] = response.Usage, response.Usage
		}
		if s.onOutput != nil {
			s.onOutput()
		}
		s.terminal = true
		if s.stream {
			return directCodexImageEvent(s.prefix+".completed", metadata), nil
		}
		s.result, _ = json.Marshal(result)
	}
	return nil, nil
}

func (s *directCodexImagesStream) Finish() ([]byte, error) {
	if !s.terminal {
		return nil, fmt.Errorf("Codex image stream ended without a completed response")
	}
	if s.stream {
		return nil, nil
	}
	return s.result, nil
}

func directCodexImageHasError(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func validateDirectCodexImageItem(item directCodexImageItem) error {
	// Some Codex versions mark a finished output_item.done as generating.
	// It is accepted only provisionally, until a real response.completed.
	if directCodexImageHasError(item.Error) || item.Status != "completed" && item.Status != "generating" {
		return fmt.Errorf("Codex image tool did not complete")
	}
	_, err := directCodexImageBase64(item.Result)
	return err
}

func directCodexImageBase64(value string) (string, error) {
	if strings.HasPrefix(value, "data:") {
		header, data, ok := strings.Cut(value, ",")
		if !ok || !strings.HasSuffix(header, ";base64") || !directCodexImageMIME(strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")) {
			return "", fmt.Errorf("Codex image result has an unsupported data URL")
		}
		value = data
	}
	n, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(value)))
	if err != nil || n == 0 {
		return "", fmt.Errorf("Codex image result does not contain valid base64 data")
	}
	return value, nil
}

func directCodexImageEvent(kind string, payload map[string]any) []byte {
	payload["type"] = kind
	raw, _ := json.Marshal(payload)
	return []byte("event: " + kind + "\ndata: " + string(raw) + "\n\n")
}

func collectDirectCodexImagesResponse(resp *http.Response, started time.Time, downstreamPath string) ([]byte, ParsedUsage, error) {
	prepared := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if prepared.Reader != nil {
		resp.Body = prepared.Reader
	}
	if !prepared.Readable {
		_ = resp.Body.Close()
		return nil, ParsedUsage{Source: usageSourceUnknown}, fmt.Errorf("cannot decode upstream image stream")
	}
	idle := &streamIdleBody{ReadCloser: resp.Body}
	idle.guard = newStreamIdleGuard(streamIdleTimeout(), idle.closeUnderlying)
	reader := newDirectCodexImagesBody(idle, downstreamPath, false, streamResponseByteLimit())
	defer reader.Close()
	var firstOutput *int64
	reader.original.onFirstOutput = func() { firstOutput = int64Ptr(time.Since(started).Milliseconds()) }
	body, err := proxy.ReadBufferedResponseBody(reader)
	usage := reader.original.Result().Usage
	usage.FirstOutputLatencyMs = firstOutput
	if idle.guard.fired.Load() {
		return nil, usage, fmt.Errorf("upstream image stream idle timeout")
	}
	return body, usage, err
}
