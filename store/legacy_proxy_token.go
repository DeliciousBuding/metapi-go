package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

const LegacyProxyTokenMigrationKey = "downstream.root_token_migrated_v1"

// DirectVideoTaskOwner keeps durable video ownership stable across key rotation.
func DirectVideoTaskOwner(source, token string, keyID *int64) string {
	owner := source + ":" + token
	if source == "managed" && keyID != nil && *keyID > 0 {
		owner = "managed:" + strconv.FormatInt(*keyID, 10)
	} else if token == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(digest[:])
}

// MigrateLegacyProxyToken imports the effective old root key once. Claiming the
// marker, creating the key, and transferring video ownership are one transaction.
// Existing keys retain their permissions and enabled state.
func MigrateLegacyProxyToken(db *DB, rt *config.RuntimeSettings, rpm int) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(tx.Rebind(`INSERT INTO settings (key, value) VALUES (?, '0') ON CONFLICT (key) DO NOTHING`), LegacyProxyTokenMigrationKey)
	if err != nil {
		return fmt.Errorf("claim root key migration: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		rt.ProxyToken = ""
		return nil
	}
	token := strings.TrimSpace(rt.ProxyToken)
	var id int64
	if token != "" && token != config.DefaultProxyToken {
		var maxRPM any
		if rpm > 0 {
			maxRPM = rpm
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.Exec(tx.Rebind(`INSERT INTO downstream_api_keys
			(name, key, enabled, supported_models, max_rpm, created_at, updated_at)
			VALUES (?, ?, TRUE, '["*"]', ?, ?, ?) ON CONFLICT (key) DO NOTHING`),
			"Migrated access token", token, maxRPM, now, now)
		if err != nil {
			return fmt.Errorf("import root key: %w", err)
		}
		if err = tx.Get(&id, tx.Rebind(`SELECT id FROM downstream_api_keys WHERE key = ?`), token); err != nil {
			return fmt.Errorf("resolve migrated key: %w", err)
		}
		oldOwner := DirectVideoTaskOwner("global", token, nil)
		newOwner := DirectVideoTaskOwner("managed", token, &id)
		var tasks []struct {
			ID       string `db:"public_id"`
			Identity string `db:"direct_identity"`
		}
		if err = tx.Select(&tasks, `SELECT public_id, direct_identity FROM proxy_video_tasks WHERE direct_identity IS NOT NULL`); err != nil {
			return fmt.Errorf("load video ownership: %w", err)
		}
		for _, task := range tasks {
			var identity map[string]json.RawMessage
			if err = json.Unmarshal([]byte(task.Identity), &identity); err != nil {
				// Unreadable pre-existing tasks cannot be attributed to this token.
				continue
			}
			var owner string
			if json.Unmarshal(identity["owner"], &owner) != nil || owner != oldOwner {
				continue
			}
			identity["owner"], _ = json.Marshal(newOwner)
			raw, err := json.Marshal(identity)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(tx.Rebind(`UPDATE proxy_video_tasks SET direct_identity = ? WHERE public_id = ?`), string(raw), task.ID); err != nil {
				return fmt.Errorf("transfer video ownership: %w", err)
			}
		}
	}
	if _, err = tx.Exec(tx.Rebind(`UPDATE settings SET value = ? WHERE key = ?`), strconv.FormatInt(id, 10), LegacyProxyTokenMigrationKey); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM settings WHERE key = 'proxy_token'`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	rt.ProxyToken = ""
	return nil
}
