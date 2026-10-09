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
	// ModelPath appends the native Gemini model/action to a resolved models URL.
	// Custom endpoint URLs remain exact when false.
	ModelPath bool   `json:"modelPath,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

const (
	DirectAuthBearer = "bearer"
	DirectAuthAPIKey = "x-api-key"
	DirectAuthGoogle = "x-goog-api-key"
)

// DirectEndpoints is optional for imports that use the original base/path
// contract. When present, each endpoint owns both its URL and authentication.
type DirectEndpoints struct {
	Chat      *DirectEndpoint `json:"chat,omitempty"`
	Responses *DirectEndpoint `json:"responses,omitempty"`
	Messages  *DirectEndpoint `json:"messages,omitempty"`
	Gemini    *DirectEndpoint `json:"gemini,omitempty"`
}

func (e DirectEndpoints) IsConfigured() bool {
	return e.Chat != nil || e.Responses != nil || e.Messages != nil || e.Gemini != nil
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
	for _, endpoint := range []*DirectEndpoint{decoded.Chat, decoded.Responses, decoded.Messages, decoded.Gemini} {
		if endpoint == nil {
			continue
		}
		u, err := url.Parse(endpoint.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return fmt.Errorf("invalid direct endpoint URL")
		}
		if endpoint.Auth != DirectAuthBearer && endpoint.Auth != DirectAuthAPIKey && endpoint.Auth != DirectAuthGoogle {
			return fmt.Errorf("invalid direct endpoint authentication")
		}
		if endpoint.ModelPath && endpoint != decoded.Gemini {
			return fmt.Errorf("model paths require a Gemini endpoint")
		}
		switch endpoint.Profile {
		case "":
		case "codex":
			if endpoint != decoded.Responses || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("codex profile requires bearer Responses")
			}
		case "claudecode":
			if endpoint != decoded.Messages || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("claudecode profile requires bearer Messages")
			}
		case "deepseek", "zai":
			if endpoint != decoded.Chat || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("chat profile requires bearer Chat")
			}
		default:
			return fmt.Errorf("invalid direct endpoint profile")
		}
	}
	*e = decoded
	return nil
}

// DirectProtocolOrder restricts one route item without broadening its shared grant.
type DirectProtocolOrder []int

func (p DirectProtocolOrder) Value() (driver.Value, error) {
	if p == nil {
		return "[]", nil
	}
	b, err := json.Marshal(p)
	return string(b), err
}
func (p *DirectProtocolOrder) Scan(value any) error {
	var raw []byte
	switch v := value.(type) {
	case nil:
		*p = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("invalid direct protocol order")
	}
	var values []int
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("invalid direct protocol order")
	}
	seen := map[int]bool{}
	for _, v := range values {
		if (v != 2 && v != 4 && v != 8 && v != 16) || seen[v] {
			return fmt.Errorf("invalid direct protocol order")
		}
		seen[v] = true
	}
	*p = values
	return nil
}
