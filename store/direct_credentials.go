package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const (
	DirectCredentialAPIKey = "api_key"
	DirectCredentialOAuth  = "oauth"
)

// DirectOAuthState contains refresh material. It belongs only in credential
// storage and write inputs, never routing snapshots or inventory responses.
// The access token has one owner: upstream_credentials.secret.
type DirectOAuthState struct {
	RefreshToken string `json:"refreshToken,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"` // Unix milliseconds
	IDToken      string `json:"idToken,omitempty"`
	AccountID    string `json:"accountId,omitempty"`
}

func (s DirectOAuthState) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *DirectOAuthState) Scan(value any) error {
	var data []byte
	switch value := value.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	case nil:
		*s = DirectOAuthState{}
		return nil
	default:
		return fmt.Errorf("invalid direct OAuth state")
	}
	if err := json.Unmarshal(data, s); err != nil {
		return fmt.Errorf("invalid direct OAuth state")
	}
	return nil
}
