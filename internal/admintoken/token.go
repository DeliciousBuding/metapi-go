// Package admintoken generates the admin bearer token and exposes the
// one-time on-disk handoff used when no AUTH_TOKEN is configured.
//
// Tokens are always produced by crypto/rand; operators never choose the
// secret. The generated value is persisted in the settings table and written
// once to a 0600 file in DATA_DIR so it can be retrieved without logging it.
package admintoken

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// GeneratedBytes is the entropy of a server-generated admin token. 32 bytes
// (64 hex characters) matches the deployment guidance and gives ample margin
// against offline brute force.
const GeneratedBytes = 32

// FileName is the one-time handoff file inside DATA_DIR.
const FileName = "admin-token"

// Generate returns a cryptographically random admin token in hex.
func Generate() (string, error) {
	buf := make([]byte, GeneratedBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// WriteFile atomically writes token to <dataDir>/admin-token with owner-only
// permissions. It returns the path for an operator-facing hint. Any existing
// file is replaced so a rotation does not leave a dead token behind.
func WriteFile(dataDir, token string) (string, error) {
	if dataDir == "" {
		return "", fmt.Errorf("data directory is empty")
	}
	path := filepath.Join(dataDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write generated admin token: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("publish generated admin token: %w", err)
	}
	return path, nil
}
