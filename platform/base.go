package platform

import (
	"context"
	"fmt"
)

// BaseAdapter provides default implementations for PlatformAdapter methods.
// Concrete adapters embed this and override specific methods as needed.
type BaseAdapter struct {
	name string
}

// NewBaseAdapter creates a BaseAdapter with the given platform name.
func NewBaseAdapter(name string) *BaseAdapter {
	return &BaseAdapter{name: name}
}

// PlatformName returns the platform identifier.
func (b *BaseAdapter) PlatformName() string {
	return b.name
}

// Detect must be implemented by concrete adapters.
func (b *BaseAdapter) Detect(ctx context.Context, url string) (bool, error) {
	return false, fmt.Errorf("detect not implemented for %s", b.name)
}

// Checkin returns unsupported by default.
func (b *BaseAdapter) Checkin(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	return &CheckinResult{Success: false, Message: fmt.Sprintf("%s for %s", DefaultCheckinUnsupportedMessage, b.name)}, nil
}

// GetBalance returns zero balance by default.
func (b *BaseAdapter) GetBalance(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	return &BalanceInfo{Balance: 0, Used: 0, Quota: 0}, nil
}

// GetModels returns empty list by default.
func (b *BaseAdapter) GetModels(ctx context.Context, url, token string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	return []string{}, nil
}

// GetAPIToken returns nil by default.
func (b *BaseAdapter) GetAPIToken(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	return nil, nil
}

// GetAPITokens returns a single-item list if GetAPIToken finds one.
func (b *BaseAdapter) GetAPITokens(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	token, err := b.GetAPIToken(ctx, url, accessToken, platformUserId, proxy)
	if err != nil || token == nil {
		return []ApiTokenInfo{}, nil
	}
	return []ApiTokenInfo{
		{Name: "default", Key: *token, Enabled: true, TokenGroup: "default"},
	}, nil
}

// CreateAPIToken returns false by default.
func (b *BaseAdapter) CreateAPIToken(ctx context.Context, url, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	return false, nil
}

// DeleteAPIToken returns nil (no-op) by default.
func (b *BaseAdapter) DeleteAPIToken(ctx context.Context, url, accessToken string, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	return nil
}

// GetSiteAnnouncements returns empty list by default.
func (b *BaseAdapter) GetSiteAnnouncements(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]SiteAnnouncement, error) {
	return []SiteAnnouncement{}, nil
}

// GetUserGroups returns ["default"] by default.
func (b *BaseAdapter) GetUserGroups(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	return []string{"default"}, nil
}
