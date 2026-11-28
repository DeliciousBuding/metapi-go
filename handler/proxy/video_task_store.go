package proxyhandler

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ProxyVideoTask holds a video task mapping (publicId -> upstreamVideoId).

// Create saves a process-local cache entry and dual-writes to
// `proxy_video_tasks` when a database is configured so multi-instance /
// restart can resolve publicId. Owner is the downstream key identity
// (store.DirectVideoTaskOwner); GET/DELETE require a mapping whose owner
// matches the caller, so a missing or ownerless row is a 404 rather than a
// pass-through of a client-supplied upstream id.
type ProxyVideoTask struct {
	PublicID        string `json:"publicId"`
	UpstreamVideoID string `json:"upstreamVideoId"`
	// Owner is an internal opaque identity hash; it is never serialized.
	Owner          string `json:"-"`
	SiteURL        string `json:"siteUrl"`
	TokenValue     string `json:"tokenValue"`
	RequestedModel string `json:"requestedModel"`
	ActualModel    string `json:"actualModel"`
	ChannelID      int64  `json:"channelId"`
	AccountID      int64  `json:"accountId"`
}

func upsertProxyVideoTaskDB(db *store.DB, task *ProxyVideoTask, at time.Time) error {
	if db == nil || task == nil {
		return nil
	}
	if strings.TrimSpace(task.PublicID) == "" || strings.TrimSpace(task.UpstreamVideoID) == "" {
		return nil
	}
	now := at.UTC().Format(time.RFC3339)
	// store.DB rebinds ? → $N for postgres.
	const q = `
INSERT INTO proxy_video_tasks (
	public_id, upstream_video_id, owner, site_url, token_value,
	requested_model, actual_model, channel_id, account_id,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(public_id) DO UPDATE SET
	upstream_video_id = excluded.upstream_video_id,
	owner = excluded.owner,
	site_url = excluded.site_url,
	token_value = excluded.token_value,
	requested_model = excluded.requested_model,
	actual_model = excluded.actual_model,
	channel_id = excluded.channel_id,
	account_id = excluded.account_id,
	updated_at = excluded.updated_at
`
	var reqModel, actModel any
	if strings.TrimSpace(task.RequestedModel) != "" {
		reqModel = task.RequestedModel
	}
	if strings.TrimSpace(task.ActualModel) != "" {
		actModel = task.ActualModel
	}
	var channelID, accountID any
	if task.ChannelID > 0 {
		channelID = task.ChannelID
	}
	if task.AccountID > 0 {
		accountID = task.AccountID
	}
	_, err := db.Exec(q,
		task.PublicID,
		task.UpstreamVideoID,
		nullableString(task.Owner),
		task.SiteURL,
		task.TokenValue,
		reqModel,
		actModel,
		channelID,
		accountID,
		now,
		now,
	)
	return err
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func loadProxyVideoTaskDB(db *store.DB, publicID string) (*ProxyVideoTask, error) {
	if db == nil {
		return nil, nil
	}
	const q = `
SELECT public_id, upstream_video_id, owner, site_url, token_value,
       requested_model, actual_model, channel_id, account_id
FROM proxy_video_tasks
WHERE public_id = ?
LIMIT 1
`
	var (
		rowPublic, upstream, siteURL, token string
		owner, reqModel, actModel           *string
		channelID, accountID                *int64
	)
	err := db.QueryRow(q, publicID).Scan(
		&rowPublic, &upstream, &owner, &siteURL, &token,
		&reqModel, &actModel, &channelID, &accountID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	task := &ProxyVideoTask{
		PublicID:        rowPublic,
		UpstreamVideoID: upstream,
		SiteURL:         siteURL,
		TokenValue:      token,
	}
	if owner != nil {
		task.Owner = *owner
	}
	if reqModel != nil {
		task.RequestedModel = *reqModel
	}
	if actModel != nil {
		task.ActualModel = *actModel
	}
	if channelID != nil {
		task.ChannelID = *channelID
	}
	if accountID != nil {
		task.AccountID = *accountID
	}
	return task, nil
}

func deleteProxyVideoTaskDB(db *store.DB, publicID string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id = ?`, publicID)
	return err
}
