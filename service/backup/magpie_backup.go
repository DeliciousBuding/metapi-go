package backup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// MagpieBackupOrigin is the default origin key for a Magpie backup.
	MagpieBackupOrigin = "magpie-backup"

	magpieBackupFormat   = "magpie-backup"
	magpieBackupVersion  = 1
	magpieBundleVersion  = 2
	magpieBackupMaxBytes = 20 << 20
)

var (
	// ErrMagpiePassphrase deliberately does not distinguish a wrong
	// passphrase from a modified ciphertext.
	ErrMagpiePassphrase = errors.New("wrong Magpie backup passphrase or damaged file")
	ErrMagpieCorrupt    = errors.New("not a valid Magpie backup")
	ErrMagpieVersion    = errors.New("unsupported Magpie backup version")
)

type magpieEnvelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Data       []byte `json:"data"`
}

// IsMagpieBackupPayload recognizes the encrypted envelope without attempting
// decryption. Detection must stay cheap enough for the shared import endpoint.
func IsMagpieBackupPayload(raw []byte) bool {
	raw = trimMagpieBOM(raw)
	if len(raw) == 0 || len(raw) > magpieBackupMaxBytes {
		return false
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return false
	}
	var format string
	return json.Unmarshal(top["format"], &format) == nil && format == magpieBackupFormat
}

// OpenMagpieBackup validates and decrypts a Magpie .magpie-backup envelope.
// Only the decrypted bundle is returned; the passphrase is never retained.
func OpenMagpieBackup(raw []byte, passphrase string) ([]byte, error) {
	raw = trimMagpieBOM(raw)
	if len(raw) > magpieBackupMaxBytes {
		return nil, fmt.Errorf("Magpie backup exceeds %d MiB", magpieBackupMaxBytes>>20)
	}
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("Magpie backup passphrase is required")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, ErrMagpieCorrupt
	}
	var e magpieEnvelope
	if err := json.Unmarshal(raw, &e); err != nil || e.Format != magpieBackupFormat {
		return nil, ErrMagpieCorrupt
	}
	if e.Version != magpieBackupVersion || e.KDF != "pbkdf2-sha256" {
		return nil, ErrMagpieVersion
	}
	if e.Iterations < 100_000 || e.Iterations > 10_000_000 || len(e.Salt) < 16 || len(e.Nonce) != 12 || len(e.Data) == 0 {
		return nil, ErrMagpieCorrupt
	}
	key, err := pbkdf2.Key(sha256.New, passphrase, e.Salt, e.Iterations, 32)
	if err != nil {
		return nil, errors.New("Magpie backup key derivation failed")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrMagpieCorrupt
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrMagpieCorrupt
	}
	aad := fmt.Appendf(nil, "%s/%d/%s/%d/%x", e.Format, e.Version, e.KDF, e.Iterations, e.Salt)
	plain, err := gcm.Open(nil, e.Nonce, e.Data, aad)
	if err != nil {
		return nil, ErrMagpiePassphrase
	}
	if len(plain) > magpieBackupMaxBytes {
		return nil, errors.New("Magpie backup contents exceed 20 MiB")
	}
	return plain, nil
}

// ParseMagpieBackup opens and parses a Magpie backup.
func ParseMagpieBackup(raw []byte, passphrase string) (*MagpieSource, error) {
	plain, err := OpenMagpieBackup(raw, passphrase)
	if err != nil {
		return nil, err
	}
	return ParseMagpieSource(plain)
}

// ParseMagpieSource parses the decrypted Magpie Bundle. Structural errors are
// fatal; a valid feature with no safe Metapi equivalent is left for the
// compiler to report.
func ParseMagpieSource(raw []byte) (*MagpieSource, error) {
	raw = trimMagpieBOM(raw)
	if len(raw) > magpieBackupMaxBytes {
		return nil, errors.New("Magpie bundle exceeds 20 MiB")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("invalid Magpie backup JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil || top == nil {
		return nil, errors.New("invalid Magpie backup JSON: expected an object")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, errors.New("invalid Magpie backup JSON: trailing data")
	}
	src := &MagpieSource{Sections: map[string]int{}, Unknown: map[string]int{}}
	if err := json.Unmarshal(top["version"], &src.Version); err != nil {
		return nil, errors.New("invalid Magpie bundle version")
	}
	if src.Version < 1 || src.Version > magpieBundleVersion {
		return nil, fmt.Errorf("%w: bundle version %d", ErrMagpieVersion, src.Version)
	}
	if err := json.Unmarshal(top["created"], &src.Created); err != nil || src.Created.IsZero() {
		return nil, errors.New("invalid Magpie bundle creation time")
	}
	_ = json.Unmarshal(top["app"], &src.App)
	if err := json.Unmarshal(top["keys"], &src.KeysIncluded); err != nil {
		return nil, errors.New("invalid Magpie credential-inclusion flag")
	}
	if rawKeys := bytes.TrimSpace(top["providersKeys"]); len(rawKeys) > 0 && string(rawKeys) != "null" {
		if err := json.Unmarshal(rawKeys, &src.KeysIncluded); err != nil {
			return nil, errors.New("invalid Magpie provider credential flag")
		}
	}
	if err := decodeMagpieProviders(top["providers"], src); err != nil {
		return nil, err
	}
	if err := decodeMagpieGroups(top["groups"], src); err != nil {
		return nil, err
	}
	if err := decodeMagpieGatewayKeys(top["gatewayKeys"], src); err != nil {
		return nil, err
	}
	magpieCountSections(top, src)
	for _, name := range magpieBundleSections {
		delete(top, name)
	}
	for name, value := range top {
		src.Unknown[sanitizeIdentifier(name)] = magpieSectionCount(value)
	}
	if err := validateMagpieSource(src); err != nil {
		return nil, err
	}
	return src, nil
}

func trimMagpieBOM(raw []byte) []byte {
	return bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
}

var magpieBundleSections = []string{
	"version", "created", "app", "keys", "providers", "icons", "groups",
	"settings", "gatewayKeys", "profiles", "agents", "library", "searches",
	"order", "groupOrder", "providersKeys", "settingsKeys", "libraryKeys",
}

func magpieSectionCount(raw json.RawMessage) int {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return 0
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) == nil {
		return len(rows)
	}
	return 1
}

func magpieCountSections(top map[string]json.RawMessage, src *MagpieSource) {
	src.Sections["icons"] = lenMap(top["icons"])
	src.Sections["profiles"] = lenMap(top["profiles"])
	src.Sections["agents"] = lenMap(top["agents"])
	src.Sections["order"] = magpieSectionCount(top["order"])
	src.Sections["groupOrder"] = magpieSectionCount(top["groupOrder"])
	src.Sections["searches"] = magpieSectionCount(top["searches"])
	if present(top["settings"]) {
		src.Sections["settings"] = 1
	}
	if present(top["library"]) {
		src.Sections["library"] = 1
	}
}

func lenMap(raw json.RawMessage) int {
	if !present(raw) {
		return 0
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return 1
	}
	return len(values)
}
