package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// ModelPrice is the advertised pricing contract, before the effective group multiplier.
// Optional amounts distinguish a free rate from a rate the upstream omitted.
type ModelPrice struct {
	Model             string   `json:"model"`
	QuotaType         int      `json:"quotaType"`
	ModelRatio        *float64 `json:"modelRatio"`
	CompletionRatio   *float64 `json:"completionRatio"`
	CacheRatio        *float64 `json:"cacheRatio"`
	CacheWriteRatio   *float64 `json:"cacheWriteRatio"`
	PricePerRequest   *float64 `json:"pricePerRequest"`
	Groups            []string `json:"groups"`
	BillingMode       string   `json:"billingMode"`
	BillingExpression string   `json:"billingExpression"`
}

type AccountPricing struct {
	Models       []ModelPrice        `json:"models"`
	GroupRatios  map[string]*float64 `json:"groupRatios"`
	UsableGroups map[string]string   `json:"usableGroups"`
}

type PricingProvider interface {
	GetPricing(context.Context, string, string, *int, *ProxyConfig) (*AccountPricing, error)
}

// GetPricing uses the account's management identity. New API has already applied
// user-group overrides to group_ratio; consumers apply each value exactly once.
func (n *NewApiAdapter) GetPricing(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) (*AccountPricing, error) {
	if userID == nil {
		userID = n.discoverUserID(ctx, baseURL, token, proxy)
	}
	headers := n.authHeaders(token, userID)
	if hasUsableSessionCookie(token) {
		delete(headers, "Authorization")
		headers["Cookie"] = token
	}
	resp, err := fetchJSON(ctx, strings.TrimRight(baseURL, "/")+"/api/pricing", http.MethodGet, nil, headers, proxy)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return ParseNewAPIPricing(encoded)
}

// ParseNewAPIPricing accepts the public /api/pricing response, not the metadata
// catalog's ratio-config response. Missing group context must remain unknown.
func ParseNewAPIPricing(data []byte) (*AccountPricing, error) {
	var raw struct {
		Success *bool `json:"success"`
		Data    *[]struct {
			Model           string   `json:"model_name"`
			QuotaType       int      `json:"quota_type"`
			ModelRatio      *float64 `json:"model_ratio"`
			CompletionRatio *float64 `json:"completion_ratio"`
			CacheRatio      *float64 `json:"cache_ratio"`
			CacheWriteRatio *float64 `json:"create_cache_ratio"`
			Price           *float64 `json:"model_price"`
			Groups          []string `json:"enable_groups"`
			Mode            string   `json:"billing_mode"`
			Expression      string   `json:"billing_expr"`
		} `json:"data"`
		Ratios map[string]*float64 `json:"group_ratio"`
		Groups map[string]string   `json:"usable_group"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid pricing response: %w", err)
	}
	if raw.Success == nil || !*raw.Success || raw.Data == nil || raw.Groups == nil {
		return nil, fmt.Errorf("pricing response has no authenticated pricing context")
	}
	out := &AccountPricing{Models: make([]ModelPrice, 0, len(*raw.Data)), GroupRatios: raw.Ratios, UsableGroups: raw.Groups}
	for name, value := range out.GroupRatios {
		out.GroupRatios[name] = pricingAmount(value)
	}
	for _, m := range *raw.Data {
		if strings.TrimSpace(m.Model) == "" {
			return nil, fmt.Errorf("pricing response contains an unnamed model")
		}
		out.Models = append(out.Models, ModelPrice{Model: m.Model, QuotaType: m.QuotaType, ModelRatio: pricingAmount(m.ModelRatio), CompletionRatio: pricingAmount(m.CompletionRatio), CacheRatio: pricingAmount(m.CacheRatio), CacheWriteRatio: pricingAmount(m.CacheWriteRatio), PricePerRequest: pricingAmount(m.Price), Groups: m.Groups, BillingMode: m.Mode, BillingExpression: m.Expression})
	}
	return out, nil
}

// Negative upstream sentinel values mean a rate is unavailable.
func pricingAmount(value *float64) *float64 {
	if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return value
}
