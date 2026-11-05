package store

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
)

// DirectRateLimit is the validated channel configuration, shared by import,
// administration, routing and runtime admission. Counters are not persisted.
type DirectRateLimit struct {
	channellimit.Config
}

func (c DirectRateLimit) Value() (driver.Value, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	value, err := json.Marshal(c)
	return string(value), err
}

func (c *DirectRateLimit) Scan(value any) error {
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("invalid stored channel rate limits")
	}
	return c.UnmarshalJSON(raw)
}

func (c *DirectRateLimit) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("channel rate limits must be an object")
	}
	var config channellimit.Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("invalid channel rate limits")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid channel rate limits")
	}
	if err := config.Validate(); err != nil {
		return err
	}
	c.Config = config
	return nil
}
