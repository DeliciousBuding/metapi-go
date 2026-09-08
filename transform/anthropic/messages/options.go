package messages

import (
	"encoding/json"
	"fmt"
)

func requestOptions(req rawObject, out wireObject) error {
	for _, field := range []string{"metadata", "cache_control"} {
		if err := hint(req[field], field); err != nil {
			return err
		}
	}
	for _, field := range []string{"temperature", "top_p"} {
		if raw, ok := req[field]; ok {
			var value float64
			if absent(raw) || json.Unmarshal(raw, &value) != nil || value < 0 || value > 1 {
				return invalid(field, "must be a number between 0 and 1")
			}
			out[field] = raw
		}
	}
	if raw, ok := req["stream"]; ok {
		stream, err := boolean(raw, "stream")
		if err != nil {
			return err
		}
		out["stream"] = stream
		if stream {
			out["stream_options"] = wireObject{"include_usage": true}
		}
	}
	if !absent(req["stop_sequences"]) {
		stops, err := array(req["stop_sequences"], "stop_sequences")
		if err != nil {
			return err
		}
		if len(stops) != 0 {
			return invalid("stop_sequences", "cannot preserve the matched stop_sequence through Chat fallback")
		}
	}
	if err := requestReasoning(req["thinking"], req["output_config"], out); err != nil {
		return err
	}
	return requestContextManagement(req["context_management"])
}

// Native adaptive thinking and portable effort levels have a direct Chat
// reasoning_effort representation. Do not approximate exact budgets or invent
// signed thinking history. As in NewAPI's EffectiveEffort, adaptive defaults to
// high; explicit effort is retained, not silently downgraded for a model.
func requestReasoning(thinking, config json.RawMessage, out wireObject) error {
	var effort, mode string
	if !absent(config) {
		value, err := object(config, "output_config")
		if err != nil {
			return err
		}
		if err := allowOnly(value, "output_config", "effort"); err != nil {
			return err
		}
		if raw, ok := value["effort"]; ok {
			effort, err = text(raw, "output_config.effort")
			if err != nil {
				return err
			}
			switch effort {
			case "low", "medium", "high", "xhigh":
			default:
				return unsupported("output_config.effort")
			}
		}
	}
	if !absent(thinking) {
		value, err := object(thinking, "thinking")
		if err != nil {
			return err
		}
		if err := allowOnly(value, "thinking", "type", "display"); err != nil {
			return err
		}
		if raw, ok := value["display"]; ok {
			display, err := text(raw, "thinking.display")
			if err != nil {
				return err
			}
			if display != "omitted" {
				return unsupported("thinking.display")
			}
		}
		mode, err = text(value["type"], "thinking.type")
		if err != nil {
			return err
		}
		switch mode {
		case "adaptive":
			if effort == "" {
				effort = "high"
			}
		case "disabled":
			if effort != "" {
				return invalid("output_config.effort", "conflicts with disabled thinking")
			}
			effort = "none"
		default:
			return unsupported("thinking.type")
		}
	}
	if mode == "" && effort != "" {
		// Claude can apply output effort without enabling thinking. Chat cannot
		// express that distinction via reasoning_effort.
		return invalid("output_config.effort", "requires adaptive thinking for Chat fallback")
	}
	if effort != "" {
		out["reasoning_effort"] = effort
	}
	return nil
}

// This is a no-op whitelist, not an implementation of context compaction.
// Retaining all thinking does not edit the submitted transcript. Clearing tools,
// thresholds, unknown edit types, and retention changes must not be discarded.
func requestContextManagement(raw json.RawMessage) error {
	if absent(raw) {
		return nil
	}
	value, err := object(raw, "context_management")
	if err != nil {
		return err
	}
	if err := allowOnly(value, "context_management", "edits"); err != nil {
		return err
	}
	raw, ok := value["edits"]
	if !ok {
		return nil
	}
	edits, err := array(raw, "context_management.edits")
	if err != nil {
		return err
	}
	for i, raw := range edits {
		path := fmt.Sprintf("context_management.edits[%d]", i)
		edit, err := object(raw, path)
		if err != nil {
			return err
		}
		if err := allowOnly(edit, path, "type", "keep"); err != nil {
			return err
		}
		typ, err := text(edit["type"], path+".type")
		if err != nil {
			return err
		}
		keep, err := text(edit["keep"], path+".keep")
		if err != nil {
			return err
		}
		if typ != "clear_thinking_20251015" || keep != "all" {
			return unsupported(path)
		}
	}
	return nil
}
