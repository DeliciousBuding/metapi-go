package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// settingsApplyError carries an HTTP status and user-facing message for a
// runtime-settings apply failure; the handler maps it to writeError.
type settingsApplyError struct {
	status int
	msg    string
}

func (e *settingsApplyError) Error() string { return e.msg }

func failSettings(status int, msg string) *settingsApplyError {
	return &settingsApplyError{status: status, msg: msg}
}

// applyStringSettingDB handles the common "simple string field" pattern:
// normalize the body value, publish it into the runtime snapshot via
// UpdateRuntime (copy-on-write), and persist it. Returns the upsert error so
// callers can surface it as HTTP 500.
func applyStringSettingDB(db *store.DB, body map[string]any, key string, dbKey string, set func(*config.RuntimeSettings, string)) error {
	if v, ok := body[key]; ok {
		val := normalizeString(v)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { set(r, val) })
		if err := upsertSettingDB(db, dbKey, val); err != nil {
			return err
		}
	}
	return nil
}

// applyFilterSettings applies the list/rule filters.
func (h *settingsHandler) applyFilterSettings(body map[string]any) *settingsApplyError {
	// Admin IP allowlist
	if v, ok := body["adminIpAllowlist"]; ok {
		var list []string
		switch val := v.(type) {
		case []any:
			for _, item := range val {
				if s, ok2 := item.(string); ok2 {
					list = append(list, strings.TrimSpace(s))
				}
			}
		case string:
			list = strings.Split(val, ",")
			for i := range list {
				list[i] = strings.TrimSpace(list[i])
			}
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.AdminIpAllowlist = list })
		// A failed persist must not look like success: the allowlist would be
		// live until the next restart and then silently revert to the env
		// value, re-opening the admin API to every IP.
		if err := upsertSettingDB(h.db, "admin_ip_allowlist", list); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save admin IP allowlist")
		}
	}

	// Global blocked brands
	if v, ok := body["globalBlockedBrands"]; ok {
		brands, err := parseStringArraySetting(v, "globalBlockedBrands")
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalBlockedBrands = brands })
		if err := upsertSettingDB(h.db, "global_blocked_brands", brands); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save brand blocking")
		}
	}

	// Global allowed models
	// Only an explicit array (including []) may mutate this setting. null /
	// wrong types used to silently wipe the allowlist (upstream).
	if v, ok := body["globalAllowedModels"]; ok {
		models, err := parseStringArraySetting(v, "globalAllowedModels")
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalAllowedModels = models })
		if err := upsertSettingDB(h.db, "global_allowed_models", models); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save model allowlist")
		}
	}

	// Proxy error keywords
	if v, ok := body["proxyErrorKeywords"]; ok {
		var keywords []string
		switch val := v.(type) {
		case []any:
			for _, item := range val {
				if s, ok2 := item.(string); ok2 {
					keywords = append(keywords, strings.TrimSpace(s))
				}
			}
		case string:
			for _, kw := range strings.Split(val, ",") {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					keywords = append(keywords, kw)
				}
			}
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyErrorKeywords = keywords })
		// Same reasoning as the allowlist above: the two neighbouring filters
		// in this function already report persist failures.
		if err := upsertSettingDB(h.db, "proxy_error_keywords", keywords); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save proxy error keywords")
		}
	}

	// Payload rules
	if v, ok := body["payloadRules"]; ok {
		rules := v
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.PayloadRules = rules })
		upsertSettingDB(h.db, "payload_rules", v)
	}

	return nil
}

// applySiteBrandingSettings applies the site branding fields.
func (h *settingsHandler) applySiteBrandingSettings(body map[string]any) *settingsApplyError {
	// Site & Branding
	if err := applyStringSettingDB(h.db, body, "systemName", "system_name", func(r *config.RuntimeSettings, v string) { r.SystemName = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "logo", "logo", func(r *config.RuntimeSettings, v string) { r.Logo = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "footer", "footer", func(r *config.RuntimeSettings, v string) { r.Footer = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "about", "about", func(r *config.RuntimeSettings, v string) { r.About = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "serverAddress", "server_address", func(r *config.RuntimeSettings, v string) { r.ServerAddress = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	return nil
}
