package backup

import (
	"errors"
	"strings"
	"time"
)

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
