package balance

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// sub2apiRefreshSingleflight provides deduplication for concurrent Sub2Api token refreshes.
var (
	sub2apiRefreshMu       sync.Mutex
	sub2apiRefreshInFlight = map[int64]*sub2apiRefreshPromise{}
)

type sub2apiRefreshPromise struct {
	done   chan struct{}
	once   sync.Once
	result sub2apiRefreshResult
}

type sub2apiRefreshResult struct {
	accessToken string
	err         error
}

func newSub2APIRefreshPromise() *sub2apiRefreshPromise {
	return &sub2apiRefreshPromise{done: make(chan struct{})}
}

func (p *sub2apiRefreshPromise) wait() sub2apiRefreshResult {
	<-p.done
	return p.result
}

func (p *sub2apiRefreshPromise) resolve(result sub2apiRefreshResult) {
	p.once.Do(func() {
		p.result = result
		close(p.done)
	})
}

// refreshSub2ApiManagedSessionSingleflight refreshes a Sub2Api managed session token.
// Uses singleflight pattern: concurrent callers for the same accountID wait on the first call.
func refreshSub2ApiManagedSessionSingleflight(cfg *config.Config, db *sqlx.DB, accountID int64, account *store.Account, site *store.Site, managedAuth map[string]any) (string, error) {
	sub2apiRefreshMu.Lock()
	if p, ok := sub2apiRefreshInFlight[accountID]; ok {
		sub2apiRefreshMu.Unlock()
		result := p.wait()
		return result.accessToken, result.err
	}
	p := newSub2APIRefreshPromise()
	sub2apiRefreshInFlight[accountID] = p
	sub2apiRefreshMu.Unlock()

	defer func() {
		sub2apiRefreshMu.Lock()
		delete(sub2apiRefreshInFlight, accountID)
		sub2apiRefreshMu.Unlock()
	}()

	token, err := tryAutoReloginBalance(cfg, db, account, site)
	p.resolve(sub2apiRefreshResult{accessToken: token, err: err})
	return token, err
}

// shouldAttemptAutoReloginBalance checks if auto-relogin should be attempted for balance refresh.
// Has extra patterns beyond the checkin version: unauthorized, forbidden, not login, not logged.
func shouldAttemptAutoReloginBalance(message string) bool {
	if message == "" {
		return false
	}
	if alert.IsTokenExpiredError(0, message) {
		return true
	}
	text := strings.ToLower(message)
	return strings.Contains(text, "access token") ||
		strings.Contains(text, "new-api-user") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden") ||
		strings.Contains(text, "not login") ||
		strings.Contains(text, "not logged")
}

// tryAutoReloginBalance attempts re-login for balance refresh.
func tryAutoReloginBalance(cfg *config.Config, db *sqlx.DB, account *store.Account, site *store.Site) (string, error) {
	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return "", fmt.Errorf("no adapter for platform %s", site.Platform)
	}

	relogin := service.GetAutoReloginConfig(account.ExtraConfig)
	if relogin == nil {
		return "", fmt.Errorf("no auto-relogin config")
	}

	password := service.DecryptPassword(cfg, relogin.PasswordCipher)
	if password == "" {
		return "", fmt.Errorf("failed to decrypt password")
	}

	proxyConfig := service.BuildPlatformProxyConfig(cfg, account, site)

	result, err := platform.LoginReusingCredential(adp, context.Background(), site.URL, relogin.Username, password, nil, proxyConfig, account.AccessToken)
	if err != nil || !result.Success || result.AccessToken == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("login failed: %s", result.Message)
	}

	newStatus := account.Status
	if account.Status == "expired" {
		newStatus = "active"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(
		db.Rebind("UPDATE accounts SET access_token = ?, status = ?, updated_at = ? WHERE id = ?"),
		result.AccessToken, newStatus, now, account.ID,
	)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}
