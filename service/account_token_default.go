package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// SetDefaultToken sets a token as the default for its account.
// Mirrors TS setDefaultToken().
func SetDefaultToken(db *sqlx.DB, tokenID int64) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var token store.AccountToken
	if err := tx.Get(&token, tx.Rebind("SELECT * FROM account_tokens WHERE id = ?"), tokenID); err != nil {
		return false, err
	}
	if !IsUsableAccountToken(&token) {
		return false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if err := setDefaultTokenTx(tx, token.AccountID, &token, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func setDefaultTokenTx(tx *sqlx.Tx, accountID int64, token *store.AccountToken, now string) error {
	// Clear all defaults for this account
	if _, err := tx.Exec(tx.Rebind("UPDATE account_tokens SET is_default = ?, updated_at = ? WHERE account_id = ?"), false, now, accountID); err != nil {
		return err
	}

	// Set this token as default
	if _, err := tx.Exec(tx.Rebind("UPDATE account_tokens SET is_default = ?, enabled = ?, updated_at = ? WHERE id = ?"), true, true, now, token.ID); err != nil {
		return err
	}

	// Update account api_token
	if _, err := tx.Exec(tx.Rebind("UPDATE accounts SET api_token = ?, updated_at = ? WHERE id = ?"), token.Token, now, accountID); err != nil {
		return err
	}
	return nil
}

// RepairDefaultToken finds and sets a new default token for an account.
// Mirrors TS repairDefaultToken().
func RepairDefaultToken(db *sqlx.DB, accountID int64) (*store.AccountToken, error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var tokens []store.AccountToken
	if err := tx.Select(&tokens, tx.Rebind("SELECT * FROM account_tokens WHERE account_id = ?"), accountID); err != nil {
		return nil, err
	}

	// Find usable tokens
	var usable []store.AccountToken
	for i := range tokens {
		if IsUsableAccountToken(&tokens[i]) {
			usable = append(usable, tokens[i])
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if len(usable) == 0 {
		// Clear api_token
		if _, err := tx.Exec(tx.Rebind("UPDATE accounts SET api_token = NULL, updated_at = ? WHERE id = ?"), now, accountID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		committed = true
		return nil, nil
	}

	// Find current default or pick first usable
	selected := usable[0]
	for i := range usable {
		if usable[i].IsDefault {
			selected = usable[i]
			break
		}
	}

	if err := setDefaultTokenTx(tx, accountID, &selected, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	return &selected, nil
}

// EnsureDefaultTokenForAccount ensures a token exists and is default for a given account.
// Mirrors TS ensureDefaultTokenForAccount(), with preserve semantics:
// on an existing token match by value, operator-set name and enabled are kept
// (callers often pass name="default"; that must not clobber a real key name).
func EnsureDefaultTokenForAccount(db *sqlx.DB, accountID int64, tokenValue string, name, source, tokenGroup string, enabled bool) (int64, error) {
	normalized := strings.TrimSpace(tokenValue)
	if normalized == "" || IsMaskedTokenValue(normalized) {
		return 0, nil
	}
	if tokenGroup == "" {
		tokenGroup = "default"
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var tokens []store.AccountToken
	if err := tx.Select(&tokens, tx.Rebind("SELECT * FROM account_tokens WHERE account_id = ?"), accountID); err != nil {
		return 0, err
	}

	// Find existing token by value
	var target *store.AccountToken
	for i := range tokens {
		if tokens[i].Token == normalized {
			target = &tokens[i]
			break
		}
	}

	if target == nil {
		// Create new
		tokenName := name
		if tokenName == "" {
			if len(tokens) == 0 {
				tokenName = "default"
			} else {
				tokenName = fmt.Sprintf("token-%d", len(tokens)+1)
			}
		}
		src := source
		if src == "" {
			src = "manual"
		}

		result, err := tx.Exec(
			tx.Rebind(`INSERT INTO account_tokens (account_id, name, token, token_group, value_status, source, enabled, is_default, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			accountID, tokenName, normalized, tokenGroup, TokenValueStatusReady, src, enabled, true, now, now,
		)
		if err != nil {
			return 0, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			// Fallback for Postgres which doesn't support LastInsertId.
			if err := tx.Get(&id, tx.Rebind("SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1"), accountID, normalized); err != nil {
				return 0, err
			}
		}
		targetID := id

		// Clear other defaults
		if _, err := tx.Exec(tx.Rebind("UPDATE account_tokens SET is_default = ?, updated_at = ? WHERE account_id = ? AND id != ?"), false, now, accountID, targetID); err != nil {
			return 0, err
		}

		// Update account api_token
		if _, err := tx.Exec(tx.Rebind("UPDATE accounts SET api_token = ?, updated_at = ? WHERE id = ?"), normalized, now, accountID); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		committed = true
		return targetID, nil
	}

	// Update existing: preserve operator-set name/enable (upstream / ).
	// Refresh/sync callers pass name="default" and enabled=true; those must not clobber.
	updateName := target.Name
	if strings.TrimSpace(updateName) == "" {
		if name != "" {
			updateName = name
		} else {
			updateName = "default"
		}
	}
	updateGroup := tokenGroup
	if target.TokenGroup != nil && strings.TrimSpace(*target.TokenGroup) != "" {
		updateGroup = *target.TokenGroup
	}
	src := source
	if src == "" {
		src = target.Source
	}
	if src == "" {
		src = "manual"
	}
	_, err = tx.Exec(
		tx.Rebind(`UPDATE account_tokens SET name = ?, token_group = ?, value_status = ?, source = ?, enabled = ?, is_default = ?, updated_at = ?
		 WHERE id = ?`),
		updateName, updateGroup, TokenValueStatusReady, src, target.Enabled, true, now, target.ID,
	)
	if err != nil {
		return 0, err
	}

	// Clear other defaults
	if _, err := tx.Exec(tx.Rebind("UPDATE account_tokens SET is_default = ?, updated_at = ? WHERE account_id = ? AND id != ?"), false, now, accountID, target.ID); err != nil {
		return 0, err
	}

	// Update account api_token
	if _, err := tx.Exec(tx.Rebind("UPDATE accounts SET api_token = ?, updated_at = ? WHERE id = ?"), normalized, now, accountID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return target.ID, nil
}

// GetDefaultTokenForAccount returns the default token for an account.
func GetDefaultTokenForAccount(db *sqlx.DB, accountID int64) (*store.AccountToken, error) {
	var token store.AccountToken
	err := db.Get(&token,
		db.Rebind(`SELECT t.* FROM account_tokens t
		 INNER JOIN accounts a ON t.account_id = a.id
		 WHERE t.account_id = ? AND t.is_default = TRUE`), accountID,
	)
	if err != nil {
		return nil, err
	}
	return &token, nil
}
