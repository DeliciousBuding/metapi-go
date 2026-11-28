package proxyhandler

import (
	"fmt"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	gemini "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

type protocolEventStream interface {
	TransformEvent([]byte) ([]byte, error)
	Finish() ([]byte, error)
}
type chainedProtocolStream struct{ first, second protocolEventStream }

func (s *chainedProtocolStream) ResponsesFailureEvents(message string, delivered ...int) []byte {
	return responsesFailureEvents(s.second, message, delivered...)
}

func (s *directProviderStream) ResponsesFailureEvents(message string, delivered ...int) []byte {
	return responsesFailureEvents(s.next, message, delivered...)
}

func (s *chainedProtocolStream) TransformEvent(frame []byte) ([]byte, error) {
	out, err := s.first.TransformEvent(frame)
	if err != nil {
		return nil, err
	}
	return transformChainedFrames(s.second, out)
}
func (s *chainedProtocolStream) Finish() ([]byte, error) {
	out, err := s.first.Finish()
	if err != nil {
		return nil, err
	}
	converted, err := transformChainedFrames(s.second, out)
	if err != nil {
		return nil, err
	}
	last, err := s.second.Finish()
	return append(converted, last...), err
}
func transformChainedFrames(stream protocolEventStream, frames []byte) ([]byte, error) {
	var out []byte
	for len(frames) > 0 {
		index, sep := nextSseBoundary(string(frames))
		if index < 0 {
			return nil, fmt.Errorf("converter returned an incomplete SSE frame")
		}
		next, err := stream.TransformEvent(frames[:index+sep])
		if err != nil {
			return nil, err
		}
		out = append(out, next...)
		frames = frames[index+sep:]
	}
	return out, nil
}
func directResponseStream(downstream, upstream, model string, options messages.Options) protocolEventStream {
	d, _ := proxy.EndpointFromPath(downstream)
	u, _ := proxy.EndpointFromPath(upstream)
	var first, second protocolEventStream
	switch u {
	case proxy.EndpointResponses:
		first = responses.NewResponsesStream(model)
	case proxy.EndpointMessages:
		first = messages.NewMessagesStream(model)
	case proxy.EndpointGemini:
		first = gemini.NewGeminiStream(model)
	}
	switch d {
	case proxy.EndpointResponses:
		second = responses.NewChatStream(model)
	case proxy.EndpointMessages:
		second = messages.NewChatStream(model, options)
	case proxy.EndpointGemini:
		second = gemini.NewChatStream(model)
	}
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return &chainedProtocolStream{first: first, second: second}
}
