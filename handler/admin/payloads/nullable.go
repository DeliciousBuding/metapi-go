package payloads

import (
	"bytes"
	"encoding/json"
)

// NullableBool decodes a JSON `true` / `false` / `null` value while
// remembering whether the key was present at all, so handlers can
// distinguish the three update intents the frontend form sends:
//
//	absent         → leave the stored value untouched
//	null           → clear the override (store NULL / inherit global default)
//	true / false   → store the explicit per-row override
//
// Fields should be declared as *NullableBool with omitempty so a missing
// JSON key leaves the pointer nil. Used by sites.resinEnabled / useUtls
// (z.boolean().nullable() on the frontend).
type NullableBool struct {
	Value   bool
	Present bool
	Null    bool
}

// UnmarshalJSON implements json.Unmarshaler for the tri-state wire value.
func (n *NullableBool) UnmarshalJSON(data []byte) error {
	n.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Null = true
		return nil
	}
	var value bool
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = value
	return nil
}
