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
	"slices"
	"strings"
	"time"
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

// MagpieSource is the decrypted, configuration-bearing part of a Magpie
// backup. History and agent-local material are counted, never executed.
type MagpieSource struct {
	Version        int
	Created        time.Time
	App            string
	KeysIncluded   bool
	Providers      []MagpieSourceProvider
	Groups         []MagpieSourceGroup
	GatewayKeys    []MagpieSourceGatewayKey
	HasGatewayKeys bool
	Sections       map[string]int
	Unknown        map[string]int
}

type MagpieSourceProvider struct {
	ID                 string
	Name               string
	Was                []string
	Off                bool
	Chat               string
	Responses          string
	Anthropic          string
	Gemini             string
	Decide             string
	Key                string
	KeyName            string
	Keys               []MagpieSourceKey
	KeyProtocol        string
	KeyWeight          int
	Routing            string
	Sink               bool
	Affinity           string
	Headers            map[string]string
	Proxy              string
	AccountProxies     map[string]string
	AccountModels      map[string][]string
	Models             []string
	PickedFrom         []string
	Unlisted           bool
	Contexts           map[string]int
	HasVertex          bool
	HasControlSecrets  bool
	HasAccountControls bool
	UnsupportedFields  []string
}

type MagpieSourceKey struct {
	Name     string
	Key      string
	Off      bool
	Protocol string
	Weight   int
}

type MagpieSourceGroup struct {
	ID                string
	Name              string
	Members           []string
	Match             []string
	Matched           []string
	Routing           string
	Affinity          string
	Sink              bool
	FirstToken        int
	Patience          int
	Off               []string
	Pick              string
	HasRules          bool
	Classifier        string
	Effort            string
	Context           int
	Levels            []string
	Fast              []string
	Family            string
	Auto              bool
	Hidden            bool
	Disabled          bool
	UnsupportedFields []string
}

type MagpieSourceGatewayKey struct {
	ID                string
	Name              string
	Off               bool
	LAN               bool
	Secret            string
	Masked            string
	Limit             *MagpieSourceLimit
	Models            []string
	Accounts          []string
	UnsupportedFields []string
}

type MagpieSourceLimit struct {
	Period     string
	Days       int
	Tokens     int64
	Cost       float64
	CacheReads bool
	HasSince   bool
}

type magpieProviderJSON struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Was            []string            `json:"was"`
	Off            bool                `json:"off"`
	Chat           string              `json:"chat"`
	Responses      string              `json:"responses"`
	Anthropic      string              `json:"anthropic"`
	Gemini         string              `json:"gemini"`
	Decide         string              `json:"decide"`
	Key            string              `json:"key"`
	KeyName        string              `json:"keyName"`
	Keys           []magpieKeyJSON     `json:"keys"`
	KeyProtocol    string              `json:"keyProtocol"`
	KeyWeight      int                 `json:"keyWeight"`
	Routing        string              `json:"routing"`
	Sink           bool                `json:"sink"`
	Affinity       string              `json:"affinity"`
	Headers        map[string]string   `json:"headers"`
	Proxy          string              `json:"proxy"`
	AccountProxies map[string]string   `json:"accountProxies"`
	AccountModels  map[string][]string `json:"accountModels"`
	Models         []string            `json:"models"`
	PickedFrom     []string            `json:"pickedFrom"`
	Unlisted       bool                `json:"unlisted"`
	Contexts       map[string]int      `json:"contexts"`
	Vertex         json.RawMessage     `json:"vertex"`
}

type magpieKeyJSON struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	Off      bool   `json:"off"`
	Protocol string `json:"protocol"`
	Weight   int    `json:"weight"`
}

type magpieGroupJSON struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Members    []string          `json:"members"`
	Match      []string          `json:"match"`
	Matched    []string          `json:"matched"`
	Routing    string            `json:"routing"`
	Affinity   string            `json:"affinity"`
	Sink       bool              `json:"sink"`
	FirstToken int               `json:"firstToken"`
	Patience   int               `json:"patience"`
	Off        []string          `json:"off"`
	Pick       string            `json:"pick"`
	Rules      []json.RawMessage `json:"rules"`
	Classifier string            `json:"classifier"`
	Effort     string            `json:"effort"`
	Context    int               `json:"context"`
	Levels     []string          `json:"levels"`
	Fast       []string          `json:"fast"`
	Family     string            `json:"family"`
	Auto       bool              `json:"auto"`
	Hidden     bool              `json:"hidden"`
	Disabled   bool              `json:"disabled"`
}

type magpieGatewayKeyJSON struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Off      bool             `json:"off"`
	LAN      bool             `json:"lan"`
	Secret   string           `json:"secret"`
	Masked   string           `json:"masked"`
	Limit    *magpieLimitJSON `json:"limit"`
	Models   []string         `json:"models"`
	Accounts []string         `json:"accounts"`
}

type magpieLimitJSON struct {
	Period     string          `json:"period"`
	Days       int             `json:"days"`
	Tokens     int64           `json:"tokens"`
	Cost       float64         `json:"cost"`
	CacheReads bool            `json:"cacheReads"`
	Since      json.RawMessage `json:"since"`
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

func decodeMagpieProviders(raw json.RawMessage, src *MagpieSource) error {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		src.Sections["providers"] = 0
		return nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return errors.New("invalid Magpie providers section")
	}
	src.Sections["providers"] = len(rows)
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return fmt.Errorf("invalid Magpie provider: %w", err)
		}
		var p magpieProviderJSON
		if err := json.Unmarshal(row, &p); err != nil {
			return errors.New("invalid Magpie provider fields")
		}
		out := MagpieSourceProvider{
			ID: strings.ToLower(strings.TrimSpace(p.ID)), Name: strings.TrimSpace(p.Name),
			Was: cleanMagpieList(p.Was), Off: p.Off,
			Chat: strings.TrimSpace(p.Chat), Responses: strings.TrimSpace(p.Responses),
			Anthropic: strings.TrimSpace(p.Anthropic), Gemini: strings.TrimSpace(p.Gemini),
			Decide: strings.TrimSpace(p.Decide),
			Key:    strings.TrimSpace(p.Key), KeyName: strings.TrimSpace(p.KeyName),
			KeyProtocol: strings.ToLower(strings.TrimSpace(p.KeyProtocol)), KeyWeight: p.KeyWeight,
			Routing: strings.ToLower(strings.TrimSpace(p.Routing)), Sink: p.Sink,
			Affinity: strings.ToLower(strings.TrimSpace(p.Affinity)),
			Headers:  p.Headers, Proxy: strings.TrimSpace(p.Proxy),
			AccountProxies: p.AccountProxies, AccountModels: p.AccountModels,
			Models: cleanMagpieList(p.Models), PickedFrom: cleanMagpieList(p.PickedFrom),
			Unlisted: p.Unlisted, Contexts: p.Contexts,
			HasVertex:          present(p.Vertex),
			HasControlSecrets:  present(obj["accessKeyID"]) || present(obj["secretAccessKey"]) || present(obj["balanceToken"]),
			HasAccountControls: present(obj["accountCaps"]) || present(obj["accountWindowCaps"]) || present(obj["accountNames"]),
			UnsupportedFields:  unknownKeys(obj, magpieProviderFields),
		}
		for _, k := range p.Keys {
			out.Keys = append(out.Keys, MagpieSourceKey{
				Name: strings.TrimSpace(k.Name), Key: strings.TrimSpace(k.Key),
				Off: k.Off, Protocol: strings.ToLower(strings.TrimSpace(k.Protocol)), Weight: k.Weight,
			})
		}
		src.Providers = append(src.Providers, out)
	}
	return nil
}

func decodeMagpieGroups(raw json.RawMessage, src *MagpieSource) error {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		src.Sections["groups"] = 0
		return nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return errors.New("invalid Magpie groups section")
	}
	src.Sections["groups"] = len(rows)
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return fmt.Errorf("invalid Magpie group: %w", err)
		}
		var g magpieGroupJSON
		if err := json.Unmarshal(row, &g); err != nil {
			return errors.New("invalid Magpie group fields")
		}
		out := MagpieSourceGroup{
			ID: strings.TrimSpace(g.ID), Name: strings.TrimSpace(g.Name),
			Members: cleanMagpieList(g.Members), Match: cleanMagpieList(g.Match),
			Matched:  cleanMagpieList(g.Matched),
			Routing:  strings.ToLower(strings.TrimSpace(g.Routing)),
			Affinity: strings.ToLower(strings.TrimSpace(g.Affinity)), Sink: g.Sink,
			FirstToken: g.FirstToken, Patience: g.Patience,
			Off: cleanMagpieList(g.Off), Pick: strings.TrimSpace(g.Pick),
			HasRules: len(g.Rules) > 0, Classifier: strings.TrimSpace(g.Classifier),
			Effort: strings.ToLower(strings.TrimSpace(g.Effort)), Context: g.Context,
			Levels: cleanMagpieList(g.Levels), Fast: cleanMagpieList(g.Fast),
			Family: strings.TrimSpace(g.Family), Auto: g.Auto, Hidden: g.Hidden,
			Disabled: g.Disabled, UnsupportedFields: unknownKeys(obj, magpieGroupFields),
		}
		src.Groups = append(src.Groups, out)
	}
	return nil
}

func decodeMagpieGatewayKeys(raw json.RawMessage, src *MagpieSource) error {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return errors.New("invalid Magpie gateway keys section")
	}
	src.HasGatewayKeys = true
	src.Sections["gatewayKeys"] = len(rows)
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return fmt.Errorf("invalid Magpie gateway key: %w", err)
		}
		var k magpieGatewayKeyJSON
		if err := json.Unmarshal(row, &k); err != nil {
			return errors.New("invalid Magpie gateway key fields")
		}
		out := MagpieSourceGatewayKey{
			ID: strings.TrimSpace(k.ID), Name: strings.TrimSpace(k.Name),
			Off: k.Off, LAN: k.LAN, Secret: strings.TrimSpace(k.Secret),
			Masked: strings.TrimSpace(k.Masked), Models: cleanMagpieList(k.Models),
			Accounts: cleanMagpieList(k.Accounts), UnsupportedFields: unknownKeys(obj, magpieGatewayKeyFields),
		}
		if k.Limit != nil {
			out.Limit = &MagpieSourceLimit{
				Period: strings.ToLower(strings.TrimSpace(k.Limit.Period)), Days: k.Limit.Days,
				Tokens: k.Limit.Tokens, Cost: k.Limit.Cost, CacheReads: k.Limit.CacheReads,
				HasSince: present(k.Limit.Since),
			}
		}
		src.GatewayKeys = append(src.GatewayKeys, out)
	}
	return nil
}

func validateMagpieSource(src *MagpieSource) error {
	providerIDs := map[string]bool{}
	aliases := map[string]string{}
	for _, p := range src.Providers {
		if p.ID == "" || providerIDs[p.ID] {
			return errors.New("invalid or duplicate Magpie provider id")
		}
		providerIDs[p.ID] = true
		aliases[p.ID] = p.ID
	}
	for _, p := range src.Providers {
		for _, was := range p.Was {
			was = strings.ToLower(strings.TrimSpace(was))
			if was == "" || was == p.ID {
				continue
			}
			if owner, ok := aliases[was]; ok && owner != p.ID {
				return errors.New("ambiguous Magpie provider alias")
			}
			aliases[was] = p.ID
		}
	}
	seenModels := map[string]bool{}
	for _, p := range src.Providers {
		seenKeys := map[string]bool{}
		if p.Key != "" {
			seenKeys[p.Key] = true
		}
		for _, k := range p.Keys {
			if k.Key != "" && seenKeys[k.Key] {
				return errors.New("duplicate key in Magpie provider")
			}
			seenKeys[k.Key] = true
		}
		for _, model := range p.Models {
			key := p.ID + "\x00" + model
			if seenModels[key] {
				return errors.New("duplicate model in Magpie provider")
			}
			seenModels[key] = true
		}
	}
	groupIDs := map[string]bool{}
	for _, g := range src.Groups {
		if g.ID == "" || groupIDs[g.ID] {
			return errors.New("invalid or duplicate Magpie group id")
		}
		groupIDs[g.ID] = true
	}
	keyIDs := map[string]bool{}
	secrets := map[string]bool{}
	for _, k := range src.GatewayKeys {
		if k.ID == "" || keyIDs[k.ID] {
			return errors.New("invalid or duplicate Magpie gateway key id")
		}
		keyIDs[k.ID] = true
		if k.Secret != "" {
			if secrets[k.Secret] {
				return errors.New("duplicate Magpie gateway key secret")
			}
			secrets[k.Secret] = true
		}
	}
	return nil
}

func cleanMagpieList(in []string) []string {
	var out []string
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

var magpieProviderFields = []string{
	"id", "name", "was", "icon", "preset", "key", "accessKeyID", "secretAccessKey",
	"keyName", "keys", "keyProtocol", "keyWeight", "chat", "responses", "anthropic",
	"gemini", "decide", "baseAPI", "routing", "sink", "affinity", "headers",
	"proxy", "accountProxies", "accountModels", "accountCaps", "accountWindowCaps",
	"accountNames", "balanceToken", "vertex", "models", "off", "contexts",
}

var magpieGroupFields = []string{
	"id", "name", "members", "match", "matched", "routing", "affinity", "sink",
	"firstToken", "patience", "off", "pick", "rules", "classifier", "effort",
	"context", "levels", "fast", "family", "auto", "hidden", "disabled",
}

var magpieGatewayKeyFields = []string{
	"id", "name", "off", "lan", "secret", "masked", "limit", "models", "accounts",
}
