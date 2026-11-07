package channelplugins

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// FillDefaults is for new-channel construction only. It fills selected missing
// slots, never rewrites saved targets or reapplies defaults on plugin upgrade.
func FillDefaults(ctx context.Context, db sqlx.ExtContext, baseURL string, endpoints *store.DirectEndpoints, config *store.DirectRequestConfig) error {
	if config.Provider == nil {
		return nil
	}
	p, _, _, err := New(db).providerProgram(ctx, *config.Provider)
	if err != nil {
		return err
	}
	defaults := p.Manifest().Defaults
	if defaults == nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		return ErrBinding
	}
	encoded, err := json.Marshal(endpoints)
	if err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	if json.Unmarshal(encoded, &values) != nil {
		return ErrBinding
	}
	for _, protocol := range config.Provider.Protocols {
		if _, exists := values[protocol]; exists {
			continue
		}
		template, exists := defaults.Endpoints[protocol]
		if !exists {
			continue
		}
		address := strings.TrimRight(baseURL, "/") + template.Path
		endpoint := store.DirectEndpoint{URL: address, Auth: template.Auth, ModelPath: template.ModelPath, Profile: template.Profile}
		values[protocol], err = json.Marshal(endpoint)
		if err != nil {
			return err
		}
	}
	encoded, err = json.Marshal(values)
	if err != nil {
		return err
	}
	var checked store.DirectEndpoints
	if checked.Scan(encoded) != nil {
		return ErrBinding
	}
	if config.Provider.Catalog == nil && defaults.Catalog != nil {
		copyBinding := *config.Provider
		copyBinding.Catalog = &store.DirectCatalogEndpoint{URL: strings.TrimRight(baseURL, "/") + defaults.Catalog.Path, Auth: defaults.Catalog.Auth, CursorParameter: defaults.Catalog.CursorParameter}
		config.Provider = &copyBinding
	}
	*endpoints = checked
	return config.ValidateEndpoints(checked)
}
