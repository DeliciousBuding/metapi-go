package store

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"

	"github.com/deliciousbuding/metapi-go/internal/routecondition"
)

// DirectAssociationRule preserves an association before request-time filtering.
// Several rules for one item are alternatives; they never weaken the grant.
type DirectAssociationRule struct {
	Priority      int64                     `json:"priority"`
	ProtocolOrder DirectProtocolOrder       `json:"protocolOrder"`
	Condition     *routecondition.Condition `json:"condition,omitempty"`
}

type DirectAssociationConditions []DirectAssociationRule

func (rules DirectAssociationConditions) Validate() error {
	if len(rules) > 1024 {
		return fmt.Errorf("too many association conditions")
	}
	for _, rule := range rules {
		if rule.Priority < -2147483648 || rule.Priority > 2147483647 {
			return fmt.Errorf("invalid association priority")
		}
		if len(rule.ProtocolOrder) == 0 {
			return fmt.Errorf("association protocols are required")
		}
		if rule.Condition != nil {
			if err := rule.Condition.Validate(); err != nil {
				return fmt.Errorf("invalid association condition")
			}
		}
		encoded, err := json.Marshal(rule.ProtocolOrder)
		var order DirectProtocolOrder
		if err != nil || order.Scan(encoded) != nil {
			return fmt.Errorf("invalid association protocols")
		}
	}
	return nil
}

func (rules DirectAssociationConditions) Value() (driver.Value, error) {
	if err := rules.Validate(); err != nil {
		return nil, err
	}
	if rules == nil {
		return "[]", nil
	}
	value, err := json.Marshal(rules)
	return string(value), err
}

func (rules *DirectAssociationConditions) Scan(value any) error {
	var raw []byte
	switch v := value.(type) {
	case nil:
		*rules = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("invalid association conditions")
	}
	var parsed DirectAssociationConditions
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("invalid association conditions")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&parsed) != nil || decoder.Decode(new(any)) != io.EOF || parsed.Validate() != nil {
		return fmt.Errorf("invalid association conditions")
	}
	*rules = parsed
	return nil
}
