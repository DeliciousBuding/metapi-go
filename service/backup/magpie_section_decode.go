package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

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
