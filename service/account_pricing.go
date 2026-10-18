package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// RefreshAccountPricing is an explicit bounded management request. Reading the
// comparison page never triggers upstream requests.
func RefreshAccountPricing(ctx context.Context, db *sqlx.DB, accountID int64) error {
	row, err := GetAccountWithSiteByID(db, accountID)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("account not found")
	}
	if row.Account.Status == "disabled" || row.Site.Status == "disabled" {
		return fmt.Errorf("account or site disabled")
	}
	adapter, ok := platform.GetAdapter(row.Site.Platform).(platform.PricingProvider)
	if !ok {
		return fmt.Errorf("pricing is unsupported for this platform")
	}
	if strings.TrimSpace(row.Account.AccessToken) == "" {
		return fmt.Errorf("management credential required")
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	prices, fetchErr := adapter.GetPricing(callCtx, row.Site.URL, row.Account.AccessToken, resolveModelRefreshPlatformUserIDPtr(row.Account.ExtraConfig, row.Account.Username), BuildPlatformProxyConfig(nil, &row.Account, &row.Site))
	now := time.Now().UTC().Format(time.RFC3339)
	if fetchErr != nil {
		// Upstream errors may echo request credentials; persist only our stable code.
		if err := store.RecordAccountPricingFailure(ctx, db, accountID, now, "upstream_error"); err != nil {
			return err
		}
		return fmt.Errorf("upstream pricing refresh failed")
	}
	payload, err := json.Marshal(prices)
	if err != nil {
		return err
	}
	return store.SaveAccountPricing(ctx, db, store.AccountPricingSnapshot{AccountID: accountID, Payload: string(payload), FetchedAt: now, AttemptedAt: now})
}
