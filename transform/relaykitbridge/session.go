// Package relaykitbridge adapts RelayKit's four conversational protocols to the
// gateway's byte-oriented conversion boundary. It owns no transport or billing.
package relaykitbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type Format string

const (
	Chat      Format = "chat"
	Responses Format = "responses"
	Messages  Format = "messages"
	Gemini    Format = "gemini"
)

// ErrSpecialized is returned only by pre-conversion feature selection. Callers
// may select an existing specialized bridge for it, never for conversion errors.
var ErrSpecialized = errors.New("request requires a specialized protocol bridge")
var ErrConversion = errors.New("RelayKit protocol conversion failed")

// Session owns exactly one request attempt and its reverse response. It is not
// shared between attempts, users or streams, and is not concurrency-safe.
type Session struct {
	from, to      Format
	model         string
	ctx           context.Context
	meta          *convmeta.Values
	streamCreated bool
}

func kitFormat(format Format) (types.RelayFormat, error) {
	switch format {
	case Chat:
		return types.RelayFormatOpenAI, nil
	case Responses:
		return types.RelayFormatOpenAIResponses, nil
	case Messages:
		return types.RelayFormatClaude, nil
	case Gemini:
		return types.RelayFormatGemini, nil
	default:
		return "", fmt.Errorf("unknown conversational protocol")
	}
}

// ConvertRequest selects the engine before invoking RelayKit. Native requests
// keep their exact bytes and do not enter a DTO decoder. model is the selected
// upstream model; Gemini carries this identity in its URL rather than its body.
func ConvertRequest(ctx context.Context, from, to Format, model string, raw []byte) (*Session, []byte, error) {
	if _, err := kitFormat(from); err != nil {
		return nil, nil, err
	}
	target, err := kitFormat(to)
	if err != nil {
		return nil, nil, err
	}
	s := &Session{from: from, to: to, model: model, ctx: ctx, meta: &convmeta.Values{
		OriginModelName: model, UpstreamModelName: model, ChannelMetaAttached: true,
		Options: &convmeta.Options{ToolLossPolicy: types.ConversionLossPolicyStrict, Claude: convmeta.ClaudeOptions{DefaultMaxTokens: func(string) int { return 4096 }}},
	}}
	if from == to {
		return s, raw, nil
	}
	if err := eligibleRequest(from, to, raw); err != nil {
		return nil, nil, err
	}
	value, err := decode(from, "request", raw)
	if err != nil {
		return nil, nil, err
	}
	// The host selected this attempt's upstream identity. Gemini carries it in
	// the URL; other formats carry it in the body. Keep both sources consistent.
	if model != "" {
		switch request := value.(type) {
		case *dto.GeneralOpenAIRequest:
			request.Model = model
		case *dto.OpenAIResponsesRequest:
			request.Model = model
		case *dto.ClaudeRequest:
			request.Model = model
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	result, err := relayconvert.ConvertRequest(ctx, s.meta, target, value)
	if err != nil {
		return nil, nil, fmt.Errorf("request: %w", ErrConversion)
	}
	if err := checkDiagnostics(result.Diagnostics); err != nil {
		return nil, nil, err
	}
	out, err := kitutil.Marshal(result.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("request encoding: %w", ErrConversion)
	}
	return s, out, nil
}

func (s *Session) Response(ctx context.Context, raw []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrConversion
	}
	if s.from == s.to {
		return raw, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := safeResponse(s.to, raw, false); err != nil {
		return nil, err
	}
	value, err := decode(s.to, "response", raw)
	if err != nil {
		return nil, err
	}
	target, _ := kitFormat(s.from)
	result, err := relayconvert.ConvertResponse(ctx, s.meta, target, value)
	if err != nil {
		return nil, fmt.Errorf("response: %w", ErrConversion)
	}
	if err := checkDiagnostics(result.Diagnostics); err != nil {
		return nil, err
	}
	return encodeResponse(result.Value)
}

func checkDiagnostics(diagnostics []types.ConversionDiagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	codes := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		codes = append(codes, d.Code)
	}
	return fmt.Errorf("protocol conversion would lose semantics (%s): %w", strings.Join(codes, ", "), ErrConversion)
}

func decode(format Format, phase string, raw []byte) (any, error) {
	var value any
	switch format {
	case Chat:
		if phase == "request" {
			value = &dto.GeneralOpenAIRequest{}
		} else if phase == "stream" {
			value = &dto.ChatCompletionsStreamResponse{}
		} else {
			value = &dto.OpenAITextResponse{}
		}
	case Responses:
		if phase == "request" {
			value = &dto.OpenAIResponsesRequest{}
		} else if phase == "stream" {
			value = &dto.ResponsesStreamResponse{}
		} else {
			value = &dto.OpenAIResponsesResponse{}
		}
	case Messages:
		if phase == "request" {
			value = &dto.ClaudeRequest{}
		} else {
			value = &dto.ClaudeResponse{}
		}
	case Gemini:
		if phase == "request" {
			value = &dto.GeminiChatRequest{}
		} else {
			value = &dto.GeminiChatResponse{}
		}
	default:
		return nil, ErrConversion
	}
	if kitutil.Unmarshal(raw, value) != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", phase, ErrConversion)
	}
	return value, nil
}

// BillingUsage belongs to RelayKit's internal metadata, not the downstream
// protocol. The host already observes the original upstream bytes for billing.
func encodeResponse(value any) ([]byte, error) {
	if event, ok := value.(relayconvert.ChatToResponsesStreamEvent); ok {
		value = event.Payload
	}
	raw, err := kitutil.Marshal(value)
	if err != nil {
		return nil, ErrConversion
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil, ErrConversion
	}
	stripBillingUsage(obj)
	return json.Marshal(obj)
}
func stripBillingUsage(obj map[string]json.RawMessage) {
	for _, key := range []string{"usage", "usageMetadata"} {
		if raw := obj[key]; len(raw) > 0 {
			var usage map[string]json.RawMessage
			if json.Unmarshal(raw, &usage) == nil && usage != nil {
				delete(usage, "billing_usage")
				delete(usage, "usage_semantic")
				delete(usage, "usage_source")
				obj[key], _ = json.Marshal(usage)
			}
		}
	}
	for _, key := range []string{"response", "message"} {
		var nested map[string]json.RawMessage
		if json.Unmarshal(obj[key], &nested) == nil && nested != nil {
			stripBillingUsage(nested)
			obj[key], _ = json.Marshal(nested)
		}
	}
}
