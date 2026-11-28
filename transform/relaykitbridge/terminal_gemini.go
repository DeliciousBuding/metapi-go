package relaykitbridge

import (
	"github.com/deliciousbuding/metapi-go/transform/shared"
)

func validateGeminiTerminal(obj object) error {
	candidates, err := array(obj["candidates"])
	if err != nil || len(candidates) != 1 {
		return terminalError()
	}
	candidate, err := readObject(candidates[0])
	if err != nil || !validReason(Gemini, shared.RawString(candidate["finishReason"])) {
		return terminalError()
	}
	return validateGeminiCandidate(candidate)
}

func (s *terminalState) observeGemini(obj object) error {
	if !present(obj["candidates"]) && present(obj["usageMetadata"]) {
		return nil
	}
	candidates, err := array(obj["candidates"])
	if err != nil {
		return err
	}
	if len(candidates) == 0 && present(obj["usageMetadata"]) {
		return nil
	}
	if len(candidates) != 1 {
		return terminalError()
	}
	candidate, err := readObject(candidates[0])
	if err != nil {
		return err
	}
	if err := validateGeminiCandidate(candidate); err != nil {
		return err
	}
	if reason := shared.RawString(candidate["finishReason"]); reason != "" {
		if !validReason(Gemini, reason) {
			return terminalError()
		}
		s.logicalEnd = true
		s.done = true
	}
	return nil
}

func validateGeminiCandidate(candidate object) error {
	if err := zeroIndex(candidate); err != nil {
		return err
	}
	if !present(candidate["content"]) {
		return nil
	}
	content, err := readObject(candidate["content"])
	if err != nil {
		return err
	}
	if !present(content["parts"]) {
		return nil
	}
	parts, err := array(content["parts"])
	if err != nil {
		return err
	}
	for _, raw := range parts {
		part, err := readObject(raw)
		if err != nil {
			return err
		}
		if present(part["functionCall"]) {
			fn, err := readObject(part["functionCall"])
			if err != nil {
				return err
			}
			if shared.RawString(fn["name"]) == "" {
				return terminalError()
			}
			if _, err := readObject(fn["args"]); err != nil {
				return err
			}
		}
	}
	return nil
}
