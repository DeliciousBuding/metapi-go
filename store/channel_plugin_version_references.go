package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/jmoiron/sqlx"
)

var ErrChannelPluginReferences = errors.New("channel_plugin_references_unreadable")

type ChannelPluginVersionReference struct {
	Key         string
	Fingerprint string
}

// Completed tasks still need their program for later content retrieval. This
// is a diagnostic/recovery index, NOT proof that a zero-reference row is safe
// to delete: an in-flight submit may have captured it before persisting a task.
func ChannelPluginVersionReferences(ctx context.Context, db sqlx.ExtContext) (map[ChannelPluginVersionReference]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT direct_identity FROM proxy_video_tasks WHERE direct_identity IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[ChannelPluginVersionReference]int64)
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		// Extract only the program reference. The task/authorization owner keeps
		// the full identity; no owner, vendor task ID or binding data leaves here.
		var identity struct {
			Version     int    `json:"version"`
			Key         string `json:"driverKey"`
			Fingerprint string `json:"driverFingerprint"`
		}
		if len(raw) > 256<<10 || json.Unmarshal([]byte(raw), &identity) != nil || identity.Version < 1 || identity.Version > 3 {
			return nil, ErrChannelPluginReferences
		}
		if identity.Version < 3 {
			if identity.Key != "" || identity.Fingerprint != "" {
				return nil, ErrChannelPluginReferences
			}
			continue
		}
		if !contract.ValidKey(identity.Key) || !contract.ValidFingerprint(identity.Fingerprint) {
			return nil, ErrChannelPluginReferences
		}
		ref := ChannelPluginVersionReference{Key: identity.Key, Fingerprint: identity.Fingerprint}
		out[ref]++
		// Bound unique metadata, not task count. A damaged/imported database
		// must fail explicitly rather than allocate an unbounded admin response.
		if len(out) > 1024 {
			return nil, ErrChannelPluginReferences
		}
	}
	return out, rows.Err()
}
