// Package events is the single source of truth for structured event
// emission (F5). Every event definition lives in this registry; producers
// reference a definition by Key and pass typed params, and WriteEvent
// renders the English fallback title/message for legacy consumers (notify /
// CSV export / history) while storing the structured titleKey + params the
// UI needs to render the event in the viewer's locale.
//
// Design: docs/internal/design/events-structured.md.
package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// ParamKind is the expected JSON type of a single param value.
type ParamKind string

const (
	ParamString ParamKind = "string"
	ParamInt    ParamKind = "int"
)

// ParamSpec describes one named param of a definition.
type ParamSpec struct {
	Name     string
	Kind     ParamKind
	Required bool
}

// Definition is one typed event in the registry. Key is the stable slug that
// also names the frontend locale entry under `events.titles.*` — the
// frontend i18n-existence test enumerates the same keys, so a Key with no
// locale entry (or vice versa) fails CI.
type Definition struct {
	// Key is the stable machine identifier (camelCase, same convention as
	// errorCode). Example: "checkinSuccess".
	Key string
	// Type is the legacy events.type value for non-UI consumers (checkin /
	// token / proxy / status / …). Must match the historical producer value.
	Type string
	// TitleEn is the legacy English title persisted into events.title for
	// non-UI consumers. It MUST match the historical producer title so a
	// migrated producer emits byte-identical rows for legacy readers.
	TitleEn string
	// Params declares the accepted params and their types.
	Params []ParamSpec
	// MessageEn is the English message template with {{name}} placeholders
	// resolved from Params.
	MessageEn string
}

// Ref names a registry entry plus the concrete param values.
type Ref struct {
	Key    string
	Params map[string]any
}

// Options carries the row-level event fields that are not part of the
// definition (level / related entity).
type Options struct {
	Level       string
	RelatedID   int64
	RelatedType string
}

var registry = map[string]Definition{}

// register adds a definition to the registry. Idempotent per Key (append-only
// definitions; a Key is never redefined with different semantics).
func register(def Definition) {
	if _, exists := registry[def.Key]; exists {
		panic("events: duplicate definition key " + def.Key)
	}
	if def.Key == "" || def.TitleEn == "" {
		panic("events: definition requires Key and TitleEn")
	}
	seen := map[string]bool{}
	for _, p := range def.Params {
		if p.Name == "" {
			panic("events: param name required in " + def.Key)
		}
		if seen[p.Name] {
			panic("events: duplicate param " + p.Name + " in " + def.Key)
		}
		seen[p.Name] = true
	}
	registry[def.Key] = def
}

// Keys returns every registered Key (sorted for stable test output).
func Keys() []string {
	out := make([]string, 0, len(registry))
	for key := range registry {
		out = append(out, key)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// validateParams checks Ref.Params against the definition's spec and returns
// a normalized copy (missing optional params are dropped, not zero-filled).
func (def Definition) validateParams(params map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(def.Params))
	declared := map[string]ParamKind{}
	for _, spec := range def.Params {
		declared[spec.Name] = spec.Kind
		if spec.Required {
			value, exists := params[spec.Name]
			if !exists || value == nil {
				return nil, fmt.Errorf("events: %s requires param %q", def.Key, spec.Name)
			}
			_ = value
		}
	}
	for name, value := range params {
		kind, ok := declared[name]
		if !ok {
			return nil, fmt.Errorf("events: %s does not declare param %q", def.Key, name)
		}
		if value == nil {
			continue
		}
		switch kind {
		case ParamString:
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("events: %s param %q must be a string", def.Key, name)
			}
		case ParamInt:
			switch value.(type) {
			case int, int64, float64:
			default:
				return nil, fmt.Errorf("events: %s param %q must be a number", def.Key, name)
			}
		}
		out[name] = value
	}
	return out, nil
}

// renderMessage resolves {{name}} placeholders from validated params.
func (def Definition) renderMessage(params map[string]any) string {
	msg := def.MessageEn
	for name, value := range params {
		msg = strings.ReplaceAll(msg, "{{"+name+"}}", fmt.Sprint(value))
	}
	return msg
}

// WriteEvent persists one structured event. It validates the params against
// the registry, renders the English fallback title/message (identical to the
// legacy CreateEvent output for migrated producers), and stores titleKey +
// params JSON alongside so the UI can render in the viewer's locale.
func WriteEvent(db *sqlx.DB, ref Ref, opts Options) error {
	def, ok := registry[ref.Key]
	if !ok {
		return fmt.Errorf("events: unknown event key %q", ref.Key)
	}
	params, err := def.validateParams(ref.Params)
	if err != nil {
		return err
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("events: marshal params for %s: %w", def.Key, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(
		db.Rebind(`INSERT INTO events (type, title, message, level, read, related_id, related_type, created_at, title_key, params)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		def.Type, def.TitleEn, def.renderMessage(params), opts.Level, false,
		opts.RelatedID, opts.RelatedType, now, def.Key, string(paramsJSON),
	)
	return err
}
