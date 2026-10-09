package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
)

// DirectEndpoint is a fully resolved HTTP endpoint. The importer owns source
// provider URL conventions; execution must not normalize this URL again.
type DirectEndpoint struct {
	URL  string `json:"url"`
	Auth string `json:"auth"`
}

const (
	DirectAuthBearer = "bearer"
	DirectAuthAPIKey = "x-api-key"
)

// DirectEndpoints is optional for imports that use the original base/path
// contract. When present, each endpoint owns both its URL and authentication.
type DirectEndpoints struct {
	Chat      *DirectEndpoint `json:"chat,omitempty"`
	Responses *DirectEndpoint `json:"responses,omitempty"`
	Messages  *DirectEndpoint `json:"messages,omitempty"`
}

func (e DirectEndpoints) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

func (e *DirectEndpoints) Scan(value any) error {
	var data []byte
	switch value := value.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	case nil:
		*e = DirectEndpoints{}
		return nil
	default:
		return fmt.Errorf("invalid direct endpoint configuration type %T", value)
	}
	var decoded DirectEndpoints
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("invalid direct endpoint configuration")
	}
	for _, endpoint := range []*DirectEndpoint{decoded.Chat, decoded.Responses, decoded.Messages} {
		if endpoint == nil {
			continue
		}
		u, err := url.Parse(endpoint.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return fmt.Errorf("invalid direct endpoint URL")
		}
		if endpoint.Auth != DirectAuthBearer && endpoint.Auth != DirectAuthAPIKey {
			return fmt.Errorf("invalid direct endpoint authentication")
		}
	}
	*e = decoded
	return nil
}
