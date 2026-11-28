package proxyhandler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type messagesReplayHistory struct {
	group, prefix [sha256.Size]byte
	owned         bool
}

func messagesReplayGroup(ids []string) ([sha256.Size]byte, error) {
	seen := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return [sha256.Size]byte{}, messagesReplayUnavailable("tool group is empty")
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return [sha256.Size]byte{}, messagesReplayUnavailable("tool group has invalid or duplicate IDs")
		}
		seen[id] = true
	}
	return messagesReplayHash(ids...), nil
}

// Decode a snapshot, never mutate Ctx.Body. Prefer precision-preserving RawBody
// messages when they agree with the prepared body; fail rather than key stale
// raw input after a caller has changed the transcript.
func messagesReplayInput(ctx *Ctx) ([]any, error) {
	encoded, err := json.Marshal(ctx.Body["messages"])
	if err != nil {
		return nil, messagesReplayUnavailable("invalid request transcript")
	}
	if len(ctx.RawBody) != 0 {
		var raw map[string]json.RawMessage
		if json.Unmarshal(ctx.RawBody, &raw) != nil || len(raw["messages"]) == 0 {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		var precise any
		decoder := json.NewDecoder(bytes.NewReader(raw["messages"]))
		decoder.UseNumber()
		if decoder.Decode(&precise) != nil {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		prepared, err := json.Marshal(precise)
		if err != nil {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		if !bytes.Equal(prepared, encoded) {
			// PrepareCtx currently decodes numbers as float64. Compare that view
			// too, but hash the exact raw numbers so large integers do not alias.
			var view any
			if json.Unmarshal(raw["messages"], &view) != nil {
				return nil, messagesReplayUnavailable("invalid raw request transcript")
			}
			prepared, err = json.Marshal(view)
			if err != nil || !bytes.Equal(prepared, encoded) {
				return nil, messagesReplayUnavailable("raw and prepared transcripts disagree")
			}
		}
		encoded = raw["messages"]
	}
	var history []any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&history) != nil || len(history) == 0 {
		return nil, messagesReplayUnavailable("request transcript is empty or invalid")
	}
	return history, nil
}

// Ignore only protocol-level cache hints, not similarly named tool arguments.
// A string and a single text block are the same transcript content. Other
// changes (including compaction) deliberately change the prefix and miss cache.
func messagesReplayContent(value any) (any, error) {
	if text, ok := value.(string); ok {
		return []any{map[string]any{"type": "text", "text": text}}, nil
	}
	blocks, ok := value.([]any)
	if !ok {
		return nil, messagesReplayUnavailable("invalid transcript content")
	}
	out := make([]any, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return nil, messagesReplayUnavailable("invalid transcript content block")
		}
		copy := make(map[string]any, len(block))
		for key, value := range block {
			if key != "cache_control" {
				copy[key] = value
			}
		}
		if copy["type"] == "tool_result" && copy["content"] != nil {
			content, err := messagesReplayContent(copy["content"])
			if err != nil {
				return nil, err
			}
			copy["content"] = content
		}
		out = append(out, copy)
	}
	return out, nil
}

func messagesReplayPrefixes(ctx *Ctx) ([sha256.Size]byte, []messagesReplayHistory, error) {
	history, err := messagesReplayInput(ctx)
	if err != nil {
		return [sha256.Size]byte{}, nil, err
	}
	var system []byte
	if ctx.Body["system"] != nil {
		value, err := messagesReplayContent(ctx.Body["system"])
		if err != nil {
			return [sha256.Size]byte{}, nil, err
		}
		system, err = json.Marshal(value)
		if err != nil {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid system content")
		}
	}
	prefix := messagesReplayHash("messages-replay-prefix-v1", string(system))
	var groups []messagesReplayHistory
	pendingOwned := make(map[string]bool)
	for _, value := range history {
		message, ok := value.(map[string]any)
		if !ok || (message["role"] != "user" && message["role"] != "assistant") {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid transcript message")
		}
		content, err := messagesReplayContent(message["content"])
		if err != nil {
			return [sha256.Size]byte{}, nil, err
		}
		message["content"] = content
		if message["role"] == "assistant" {
			if len(pendingOwned) != 0 {
				return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tools were not fully answered")
			}
			var ids []string
			for _, value := range content.([]any) {
				block := value.(map[string]any)
				if block["type"] == "tool_use" {
					id, _ := block["id"].(string)
					ids = append(ids, id)
				}
			}
			if len(ids) != 0 {
				group, err := messagesReplayGroup(ids)
				if err != nil {
					return [sha256.Size]byte{}, nil, err
				}
				owned, err := messagesReplayOwnedGroup(ids)
				if err != nil {
					return [sha256.Size]byte{}, nil, err
				}
				groups = append(groups, messagesReplayHistory{group: group, prefix: prefix, owned: owned})
				if owned {
					for _, id := range ids {
						pendingOwned[id] = true
					}
				}
			}
		}
		if message["role"] == "user" {
			for _, value := range content.([]any) {
				block := value.(map[string]any)
				id, _ := block["tool_use_id"].(string)
				if block["type"] == "tool_result" && strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
					if !pendingOwned[id] {
						return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tool result has no matching call")
					}
					delete(pendingOwned, id)
				}
			}
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid transcript message")
		}
		// A hash chain is linear in transcript bytes, not an O(n^2) serialization
		// of every prefix. No transcript strings remain in the replay cache.
		prefix = messagesReplayHash(string(prefix[:]), string(encoded))
	}
	if len(pendingOwned) != 0 {
		return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tools were not fully answered")
	}
	return prefix, groups, nil
}

// The prefix is a provenance marker, not authorization or a persistence token.
// Any malformed/mixed use of the reserved namespace remains fail-closed.
func messagesReplayOwnedGroup(ids []string) (bool, error) {
	owned, ordinary := false, false
	for _, id := range ids {
		if !strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
			ordinary = true
			continue
		}
		owned = true
		suffix := strings.TrimPrefix(id, messagesBridgeToolIDPrefix)
		if len(suffix) != 32 {
			return false, messagesReplayUnavailable("malformed bridge-owned tool ID")
		}
		decoded, err := hex.DecodeString(suffix)
		if err != nil || len(decoded) != 16 {
			return false, messagesReplayUnavailable("malformed bridge-owned tool ID")
		}
	}
	if owned && ordinary {
		return false, messagesReplayUnavailable("mixed bridge-owned and native tool group")
	}
	return owned, nil
}
