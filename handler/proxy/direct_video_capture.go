package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// A pending row is a durable program pin, not a vendor task. The existing
// loader rejects its empty upstream ID. No secret or response state is stored.
type directVideoCapture struct {
	db       *store.DB
	publicID string
	identity string
}

func captureDirectVideoDriver(ctx context.Context, tx *sqlx.Tx, proxyCtx *Ctx, direct *store.DirectUpstreamCandidate, model, fingerprint string) (string, error) {
	if proxyCtx == nil || proxyCtx.videoCapture != nil || store.GetDB() == nil {
		return "", errors.New("video capture is unavailable")
	}
	snapshot := *proxyCtx
	snapshot.videoDriverFingerprint = fingerprint
	identity := videoIdentity(&snapshot, direct)
	if proxyCtx.videoTask != nil {
		identity.SourcePublicID = proxyCtx.videoTask.PublicID
	}
	if identity.Version != 3 || identity.DriverFingerprint != fingerprint {
		return "", errors.New("video capture identity is unavailable")
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	capture := &directVideoCapture{db: store.GetDB(), publicID: directVideoIDPrefix + strings.TrimPrefix(newPublicVideoID(), "video_"), identity: string(raw)}
	// Keep the handle even if Commit confirmation is lost. The dispatcher has
	// not sent anything yet and will only release a matching pending row.
	proxyCtx.videoCapture = capture
	now := videoTaskNow().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,requested_model,actual_model,channel_id,direct_identity,accounting_state,created_at,updated_at)
		VALUES(?,'','','',?,?,?,?,'{"version":1}',?,?)`), capture.publicID, proxyCtx.RequestedModel, model, -direct.ItemID, capture.identity, now, now)
	return capture.publicID, err
}

// The attempt owner calls this only after all its response/task processing has
// ended. Failure leaves the pin in storage; age or a lease never revokes it.
func releaseDirectVideoCapture(proxyCtx *Ctx, requestID string) {
	if proxyCtx == nil || proxyCtx.videoCapture == nil {
		return
	}
	capture := proxyCtx.videoCapture
	proxyCtx.videoCapture = nil
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := capture.db.ExecContext(ctx, `DELETE FROM proxy_video_tasks WHERE public_id=? AND upstream_video_id='' AND direct_identity=?`, capture.publicID, capture.identity)
	if err != nil {
		slog.Warn("pending video capture could not be released", "request_id", requestID)
	}
}
