package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// TokenDefaultSwitch records one convergence SyncTokensFromUpstream had to
// perform: the stored default relay credential is no longer listed upstream, so
// the default moved to a key the upstream still lists.
type TokenDefaultSwitch struct {
	FromTokenID   int64
	FromTokenName string
	ToTokenID     int64
	ToTokenName   string
}

// TokenSyncResult summarizes SyncTokensFromUpstream outcomes.
type TokenSyncResult struct {
	Created         int
	Updated         int
	MaskedPending   int
	PendingTokenIDs []int64
	Total           int
	DefaultTokenID  *int64
	// DefaultSwitch is non-nil when the default relay credential moved because
	// the key it pointed at is gone upstream.
	DefaultSwitch *TokenDefaultSwitch
	// DefaultSwitchSkipped names why an absent default was deliberately NOT
	// re-elected, so a caller can tell "nothing to converge" from "could not
	// prove the listing was complete". Empty when no skip decision was made.
	DefaultSwitchSkipped string
}

// SyncTokensFromUpstream upserts account tokens from upstream listings.
// When an existing ready token matches by key, local enabled (and non-empty
// operator-set name) are preserved; upstream enable flags do not clobber
// operator disable/enable choices (upstream / ).
func SyncTokensFromUpstream(db *sqlx.DB, accountID int64, listing UpstreamTokenListing) (*TokenSyncResult, error) {
	upstreamTokens := listing.Tokens
	now := time.Now().UTC().Format(time.RFC3339)

	var existing []store.AccountToken
	if err := db.Select(&existing, db.Rebind("SELECT * FROM account_tokens WHERE account_id = ?"), accountID); err != nil {
		return nil, err
	}

	result := &TokenSyncResult{Total: len(existing)}
	index := len(existing) + 1

	for _, upstream := range upstreamTokens {
		tokenValue := strings.TrimSpace(upstream.Key)
		if tokenValue == "" {
			continue
		}
		tokenName := strings.TrimSpace(upstream.Name)
		if tokenName == "" {
			if index == 1 {
				tokenName = "default"
			} else {
				tokenName = fmt.Sprintf("token-%d", index)
			}
		}
		tokenGroup := strings.TrimSpace(upstream.TokenGroup)
		if tokenGroup == "" {
			tokenGroup = "default"
		}
		nextValueStatus := TokenValueStatusReady
		if IsMaskedTokenValue(tokenValue) {
			nextValueStatus = TokenValueStatusMaskedPending
		}

		// Match an existing token by exact key value. Prefer a ready row so a
		// hydrated upstream key keeps reconciling into the same usable row.
		// Fall back to a masked-pending row when the upstream still returns
		// the masked display key (platforms with no full-key endpoint): those
		// rows were previously never matched, so every sync re-inserted them.
		var byToken *store.AccountToken
		for i := range existing {
			if existing[i].Token != tokenValue {
				continue
			}
			if ResolveAccountTokenValueStatus(&existing[i]) == TokenValueStatusReady {
				byToken = &existing[i]
				break
			}
			if byToken == nil {
				byToken = &existing[i]
			}
		}
		if byToken != nil {
			// Preserve local enable state and the row's resolved value status
			// (ready stays ready; masked stays masked_pending). Keep
			// operator-set name when present.
			preservedName := byToken.Name
			if strings.TrimSpace(preservedName) == "" {
				preservedName = tokenName
			}
			preservedEnabled := byToken.Enabled
			preservedValueStatus := ResolveAccountTokenValueStatus(byToken)
			if _, err := db.Exec(
				db.Rebind(`UPDATE account_tokens SET name = ?, token_group = ?, value_status = ?, source = ?, enabled = ?, updated_at = ?
				 WHERE id = ?`),
				preservedName, tokenGroup, preservedValueStatus, "sync", preservedEnabled, now, byToken.ID,
			); err != nil {
				return nil, err
			}
			byToken.Name = preservedName
			groupCopy := tokenGroup
			byToken.TokenGroup = &groupCopy
			byToken.ValueStatus = preservedValueStatus
			byToken.Enabled = preservedEnabled
			byToken.Source = "sync"
			byToken.UpdatedAt = now
			result.Updated++
			continue
		}

		// New token from upstream.
		nextEnabled := nextValueStatus == TokenValueStatusReady && upstream.Enabled
		if nextValueStatus == TokenValueStatusMaskedPending {
			nextEnabled = false
		}
		insertRes, err := db.Exec(
			db.Rebind(`INSERT INTO account_tokens (account_id, name, token, token_group, value_status, source, enabled, is_default, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			accountID, tokenName, tokenValue, tokenGroup, nextValueStatus, "sync", nextEnabled, false, now, now,
		)
		if err != nil {
			return nil, err
		}
		id, err := insertRes.LastInsertId()
		if err != nil {
			if err := db.Get(&id, db.Rebind("SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1"), accountID, tokenValue); err != nil {
				return nil, err
			}
		}
		groupCopy := tokenGroup
		createdRow := store.AccountToken{
			ID:          id,
			AccountID:   accountID,
			Name:        tokenName,
			Token:       tokenValue,
			TokenGroup:  &groupCopy,
			ValueStatus: nextValueStatus,
			Source:      "sync",
			Enabled:     nextEnabled,
			IsDefault:   false,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		existing = append(existing, createdRow)
		result.Created++
		index++
		if nextValueStatus == TokenValueStatusMaskedPending {
			result.MaskedPending++
			result.PendingTokenIDs = append(result.PendingTokenIDs, id)
		}
	}

	// Converge the default relay credential before repairing it: an operator who
	// rotated their upstream key must get a working default from the sync itself,
	// not from a second manual "set default" click (see the function comment).
	switched, skipReason, err := convergeDefaultTokenOnUpstreamAbsence(
		db, accountID, existing, upstreamTokens, listing.Complete, listing.DefaultSwitchUnsafe,
	)
	if err != nil {
		return nil, err
	}
	result.DefaultSwitch = switched
	result.DefaultSwitchSkipped = skipReason

	repaired, err := RepairDefaultToken(db, accountID)
	if err != nil {
		return nil, err
	}
	if repaired != nil {
		id := repaired.ID
		result.DefaultTokenID = &id
	}
	result.Total = len(existing)
	return result, nil
}

// convergeDefaultTokenOnUpstreamAbsence re-elects the account's default relay
// credential when the key it points at is provably gone upstream.
//
// Observed failure (v0.19 stability window, Aged run on master 693e4050): an
// operator deleted one New API key and created another. Re-logging the account
// in — the one recovery step the docs teach — synced the new key into
// account_tokens but left the revoked one as is_default, so accounts.api_token
// kept the dead value, the model refresh failed with "API key is invalid", the
// route rebuild was skipped with it, and relay stayed 401 then 503 "no available
// channels" until someone set the default by hand and rebuilt routes. The sync
// answered success:true the whole time.
//
// "Not in the listing" only proves revocation when the listing is the whole
// truth, so three guards gate the switch:
//   - only an explicit complete-listing contract may bypass the legacy page-limit
//     guard. Unknown adapters and incomplete sources keep the original >=100
//     behavior; list length or a partial final page never proves completeness;
//   - a legacy flat response retried through the old query contract may still
//     be unproven even when short, so it never authorizes a default switch;
//   - no listed key is a masked display value (a hydrated real key never equals
//     its own mask, so absence there would be a hydration artifact, #1179);
//   - the stored default is a ready row, i.e. its value is comparable at all.
//
// Operator intent is never overridden: the replacement must already be enabled,
// and an absent default with no enabled replacement is left exactly as it was —
// the relay failure stays loud instead of being traded for a silent guess.
func convergeDefaultTokenOnUpstreamAbsence(
	db *sqlx.DB,
	accountID int64,
	tokens []store.AccountToken,
	upstream []UpstreamAPIToken,
	listingComplete bool,
	defaultSwitchUnsafe bool,
) (*TokenDefaultSwitch, string, error) {
	var current *store.AccountToken
	for i := range tokens {
		if tokens[i].IsDefault {
			current = &tokens[i]
			break
		}
	}
	if current == nil || ResolveAccountTokenValueStatus(current) != TokenValueStatusReady {
		return nil, "", nil
	}
	if defaultSwitchUnsafe {
		return nil, "upstream_listing_completeness_unproven", nil
	}
	if !listingComplete && len(upstream) >= platform.UpstreamTokenListPageLimit {
		return nil, "upstream_listing_may_be_truncated", nil
	}

	listed := make(map[string]struct{}, len(upstream))
	for _, item := range upstream {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		if IsMaskedTokenValue(key) {
			return nil, "upstream_listing_is_masked", nil
		}
		listed[key] = struct{}{}
	}
	if _, ok := listed[strings.TrimSpace(current.Token)]; ok {
		return nil, "", nil
	}

	var candidate *store.AccountToken
	for i := range tokens {
		row := &tokens[i]
		if row.ID == current.ID || !row.Enabled {
			continue
		}
		if ResolveAccountTokenValueStatus(row) != TokenValueStatusReady {
			continue
		}
		if _, ok := listed[strings.TrimSpace(row.Token)]; !ok {
			continue
		}
		if candidate == nil || row.ID < candidate.ID {
			candidate = row
		}
	}
	if candidate == nil {
		return nil, "no_enabled_token_listed_upstream", nil
	}

	// SetDefaultToken is the existing owner of "which credential does this
	// account relay with": it clears the other defaults, enables the chosen row
	// and rewrites accounts.api_token in one transaction.
	switched, err := SetDefaultToken(db, candidate.ID)
	if err != nil {
		return nil, "", err
	}
	if !switched {
		return nil, "replacement_not_usable", nil
	}
	return &TokenDefaultSwitch{
		FromTokenID:   current.ID,
		FromTokenName: current.Name,
		ToTokenID:     candidate.ID,
		ToTokenName:   candidate.Name,
	}, "", nil
}
