package messages

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrReasoningReplay means a tool continuation cannot safely preserve hidden
// reasoning. Callers must not retry by silently dropping reasoning or disabling
// thinking. A missing/expired record is different from a known empty record.
var ErrReasoningReplay = errors.New("messages Chat bridge: hidden tool reasoning replay is unavailable")

// Options supplies caller-owned reasoning continuity without putting hidden
// thoughts in client-visible Messages blocks or a package-global cache.
//
// Callbacks operate on the full ordered set of tool IDs in one assistant message.
// The caller must scope storage to the authenticated client/conversation and
// validate the complete group; tool IDs alone are not authorization. Storage
// lifetime, capacity and expiry are the caller's responsibility. The bridge does
// not provide persistence or claim continuity across restarts/cache misses.
//
// LoadReasoning returns found=true even for a recorded response with no reasoning
// (an empty string). Such a record does not add a fabricated reasoning_content
// field. SaveReasoning is called for every successfully completed tool response
// when supplied, including known empty reasoning, and must finish before native
// completion is released. Neither callback should log hidden reasoning.
type Options struct {
	LoadReasoning func(toolUseIDs []string) (reasoning string, found bool, err error)
	SaveReasoning func(toolUseIDs []string, reasoning string) error
	// NewToolUseID optionally assigns caller-owned, client-visible identities.
	// Nil preserves upstream IDs. Allocation occurs once per output tool; Save
	// receives these output IDs, while streaming still tracks upstream IDs.
	NewToolUseID func() (string, error)
}

func requestOptionsValue(options []Options) (Options, error) {
	if len(options) > 1 {
		return Options{}, invalid("options", "must contain at most one Options value")
	}
	if len(options) == 1 {
		return options[0], nil
	}
	return Options{}, nil
}

func (options Options) replay(message wireObject, required bool) error {
	calls, ok := message["tool_calls"].([]wireObject)
	if !ok || len(calls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call["id"].(string))
	}
	if options.LoadReasoning == nil {
		if required {
			return fmt.Errorf("%w: adaptive assistant tool history requires LoadReasoning", ErrReasoningReplay)
		}
		return nil
	}
	reasoning, found, err := options.LoadReasoning(ids)
	if err != nil {
		// Callback errors can contain sensitive storage/debug context. Expose
		// the failure category, not the callback's raw message.
		return fmt.Errorf("%w: lookup failed", ErrReasoningReplay)
	}
	if !found {
		if required {
			return fmt.Errorf("%w: assistant tool history has no complete replay record", ErrReasoningReplay)
		}
		return nil
	}
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	return nil
}

func (options Options) remember(ids []string, reasoning string) error {
	if len(ids) == 0 {
		return nil
	}
	if options.SaveReasoning == nil {
		if reasoning != "" {
			return fmt.Errorf("%w: a tool response with hidden reasoning requires SaveReasoning", ErrReasoningReplay)
		}
		return nil
	}
	if err := options.SaveReasoning(ids, reasoning); err != nil {
		return fmt.Errorf("%w: save failed", ErrReasoningReplay)
	}
	return nil
}

func (options Options) toolUseID(upstreamID string) (string, error) {
	if options.NewToolUseID == nil {
		return upstreamID, nil
	}
	id, err := options.NewToolUseID()
	if err != nil {
		return "", fmt.Errorf("%w: tool identity allocation failed", ErrReasoningReplay)
	}
	if !utf8.ValidString(id) || strings.TrimSpace(id) == "" {
		return "", invalid("tool_use.id", "allocator returned an invalid identity")
	}
	return id, nil
}
