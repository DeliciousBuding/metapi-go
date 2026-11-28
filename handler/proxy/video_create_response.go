package proxyhandler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
)

// maybeRewriteVideosCreateResponse rewrites a successful POST /v1/videos body so
// clients see an opaque publicId, and seeds the process-local + durable mapping
// used by GET/DELETE path rewrite and ownership checks. Best-effort: any
// parse/shape miss returns body unchanged, which leaves the upstream id
// unmapped and therefore unreadable (fail closed).
func maybeRewriteVideosCreateResponse(
	ctx *Ctx,
	selected *routing.SelectedChannel,
	upstreamPath string,
	body []byte,
) []byte {
	if ctx == nil || selected == nil || len(body) == 0 {
		return body
	}
	path := strings.TrimSpace(upstreamPath)
	if path == "" {
		path = strings.TrimSpace(ctx.DownstreamPath)
	}
	path = strings.TrimSuffix(path, "/")
	if !strings.EqualFold(path, "/v1/videos") {
		return body
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	upstreamID, _ := payload["id"].(string)
	upstreamID = strings.TrimSpace(upstreamID)
	if upstreamID == "" {
		return body
	}

	publicID := newPublicVideoID()
	actualModel := selected.ActualModel
	if actualModel == "" {
		actualModel = ctx.RequestedModel
	}
	SaveProxyVideoTask(ctx.db, &ProxyVideoTask{
		PublicID:        publicID,
		UpstreamVideoID: upstreamID,
		Owner:           directVideoOwner(ctx.Auth),
		SiteURL:         selected.Site.URL,
		TokenValue:      selected.TokenValue,
		RequestedModel:  ctx.RequestedModel,
		ActualModel:     actualModel,
		ChannelID:       selected.Channel.ID,
		AccountID:       selected.Account.ID,
	})

	payload["id"] = publicID
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

func newPublicVideoID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "video_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return "video_" + hex.EncodeToString(b[:])
}
