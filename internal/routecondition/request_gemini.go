package routecondition

import (
	"bytes"
	"encoding/json"
	"strings"
)

type geminiPart struct {
	Text             string
	Thought          bool
	ThoughtSignature string
	InlineData       *struct{ MIMEType, Data string }
	FileData         *struct{ MIMEType, FileURI string }
	FunctionCall     *struct {
		ID, Name string
		Args     json.RawMessage
	}
	FunctionResponse *struct {
		ID, Name string
		Response json.RawMessage
	}
}

func (part *geminiPart) UnmarshalJSON(raw []byte) error {
	type fields geminiPart
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value fields
	if decoder.Decode(&value) != nil {
		return errRequest
	}
	*part = geminiPart(value)
	return nil
}

type geminiContent struct {
	Role  string
	Parts []geminiPart
}

func describeGemini(object map[string]json.RawMessage, result *Request) error {
	var request struct {
		SystemInstruction *geminiContent
		Contents          []*geminiContent
		Tools             []struct {
			FunctionDeclarations []struct {
				Name, Description                string
				Parameters, ParametersJSONSchema json.RawMessage
			}
			GoogleSearch, CodeExecution, URLContext json.RawMessage
		}
	}
	body, _ := json.Marshal(object)
	if json.Unmarshal(body, &request) != nil || len(request.Contents) == 0 {
		return errRequest
	}
	if request.SystemInstruction != nil {
		var parts []string
		for _, part := range request.SystemInstruction.Parts {
			if part.Text != "" && !part.Thought {
				parts = append(parts, part.Text)
			}
		}
		if text := strings.Join(parts, "\n"); text != "" {
			result.PromptTokens += sourceTokens("system") + sourceTokens(text)
		}
	}
	previousCalls := map[string]string{}
	for _, content := range request.Contents {
		if content == nil || len(content.Parts) == 0 {
			continue
		}
		role := content.Role
		if role == "model" {
			role = "assistant"
		} else if role == "" {
			role = "user"
		}
		var contentTokens, toolTokens int64
		partCount := 0
		onlyText, reasoning := "", ""
		features := &Request{}
		var response *struct {
			ID, Name string
			Response json.RawMessage
		}
		for _, part := range content.Parts {
			switch {
			case part.Text != "":
				if part.Thought {
					reasoning = part.Text // Source keeps the last thought text.
				} else {
					partCount++
					onlyText = part.Text
					contentTokens += sourceTokens("text") + sourceTokens(part.Text)
				}
			case part.InlineData != nil:
				partCount++
				onlyText = ""
				url := "data:" + part.InlineData.MIMEType + ";base64," + part.InlineData.Data
				contentTokens += geminiMediaTokens(part.InlineData.MIMEType, url, features)
			case part.FileData != nil:
				partCount++
				onlyText = ""
				contentTokens += geminiMediaTokens(part.FileData.MIMEType, part.FileData.FileURI, features)
			case part.FunctionCall != nil:
				args := "{}"
				var err error
				if len(part.FunctionCall.Args) > 0 && !bytes.Equal(part.FunctionCall.Args, []byte("null")) {
					args, err = compactJSON(part.FunctionCall.Args, true)
				}
				if err != nil {
					return err
				}
				toolTokens += sourceTokens("function") + sourceTokens(part.FunctionCall.Name) + sourceTokens(args)
			case part.FunctionResponse != nil:
				response = part.FunctionResponse
			default:
				// Empty parts/signature-only parts produce an empty source message.
			}
			if response != nil {
				break
			}
		}
		if response != nil {
			text, err := compactJSON(response.Response, true)
			if err != nil {
				return err
			}
			id := response.ID
			if id == "" {
				id = previousCalls[response.Name]
			}
			result.PromptTokens += sourceTokens("tool") + sourceTokens(id) + sourceTokens(response.Name) + sourceTokens(text)
		} else {
			if partCount == 1 && onlyText != "" {
				contentTokens = sourceTokens(onlyText)
			}
			result.PromptTokens += sourceTokens(role) + contentTokens + toolTokens + sourceTokens(reasoning)
			result.HasImage = result.HasImage || features.HasImage
			result.HasVideo = result.HasVideo || features.HasVideo
			result.HasDocument = result.HasDocument || features.HasDocument
			result.HasAudio = result.HasAudio || features.HasAudio
		}
		for _, part := range content.Parts {
			if part.FunctionCall != nil && part.FunctionCall.ID != "" {
				previousCalls[part.FunctionCall.Name] = part.FunctionCall.ID
			}
		}
	}
	for _, tool := range request.Tools {
		for _, function := range tool.FunctionDeclarations {
			var schema string
			if len(function.Parameters) > 0 {
				var err error
				schema, err = geminiSchema(function.Parameters)
				if err != nil {
					return err
				}
			}
			// ParametersJSONSchema is kept in a different source field, which
			// its conditional estimator does not count.
			result.PromptTokens += sourceTokens("function") + sourceTokens(function.Name) + sourceTokens(function.Description) + sourceTokens(schema)
		}
		for _, builtin := range []struct {
			raw  json.RawMessage
			name string
		}{
			{tool.GoogleSearch, "google_search"}, {tool.CodeExecution, "google_code_execution"}, {tool.URLContext, "google_url_context"},
		} {
			if len(builtin.raw) > 0 && !bytes.Equal(builtin.raw, []byte("null")) {
				result.PromptTokens += sourceTokens(builtin.name)
			}
		}
	}
	return nil
}

func geminiMediaTokens(mime, url string, result *Request) int64 {
	lower := strings.ToLower(mime)
	switch {
	case strings.HasPrefix(lower, "application/pdf"), strings.HasPrefix(lower, "application/msword"),
		strings.HasPrefix(lower, "application/vnd.openxmlformats-officedocument"),
		strings.HasPrefix(lower, "application/vnd.ms-"), strings.HasPrefix(lower, "text/"):
		result.HasDocument = true
		return sourceTokens("document") + min(sourceTokens(url), 512) + sourceTokens(mime)
	case strings.HasPrefix(lower, "video/"):
		result.HasVideo = true
		return sourceTokens("video_url") + 128
	case strings.HasPrefix(lower, "audio/"):
		result.HasAudio = true
		format := "mp3"
		switch lower {
		case "audio/wav", "audio/wave", "audio/x-wav":
			format = "wav"
		case "audio/webm":
			format = "webm"
		case "audio/ogg":
			format = "ogg"
		case "audio/flac":
			format = "flac"
		case "audio/aac":
			format = "aac"
		case "audio/mp4", "audio/m4a":
			format = "mp4"
		}
		return sourceTokens("input_audio") + 128 + sourceTokens(format)
	default:
		result.HasImage = true
		return sourceTokens("image_url") + 128
	}
}
