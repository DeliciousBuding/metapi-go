package proxyhandler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

const directVideoIDPrefix = "video_direct_"

func isDirectVideoContentPath(path string) bool {
	if !strings.HasPrefix(path, "/v1/videos/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/v1/videos/"), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] == "content"
}

// Identity stores references and hashes only. In particular, credential values
// never enter proxy_video_tasks; the credential resolver owns rotation.
type directVideoIdentity struct {
	Version        int    `json:"version"`
	Protocol       int    `json:"protocol,omitempty"`
	Owner          string `json:"owner"`
	RouteID        int64  `json:"routeId"`
	GroupID        int64  `json:"groupId"`
	ItemID         int64  `json:"itemId"`
	ChannelID      int64  `json:"channelId"`
	GrantID        int64  `json:"grantId"`
	CredentialID   int64  `json:"credentialId"`
	ModelID        int64  `json:"modelId"`
	EndpointHash   string `json:"endpointHash"`
	SourcePublicID string `json:"sourcePublicId,omitempty"`
}

type directVideoTask struct {
	PublicID, UpstreamID, RequestedModel, ActualModel string
	Identity                                          directVideoIdentity
}

func directVideoOwner(pac *auth.ProxyAuthContext) string {
	return auth.DirectVideoTaskOwner(pac)
}

func directVideoTaskProtocol(identity directVideoIdentity) int {
	if identity.Version == 1 && identity.Protocol == 0 {
		return store.DirectProtocolVideo
	}
	return identity.Protocol
}

func directVideoSelectedEndpoint(d *store.DirectUpstreamCandidate) (int, *store.DirectEndpoint) {
	order := d.ProtocolOrder
	if len(order) == 0 {
		order = store.DirectProtocolOrder{store.DirectProtocolVideo, store.DirectProtocolSeedanceVideo, store.DirectProtocolZenmuxVideo}
	}
	for _, bit := range order {
		if bit&store.DirectVideoProtocols != 0 && d.Protocols&bit != 0 {
			if endpoint := d.Endpoints.ForProtocol(bit); endpoint != nil {
				return bit, endpoint
			}
		}
	}
	return 0, nil
}

// Pin a request-local candidate before the dispatcher chooses its wire adapter.
// Member reordering must not send an existing task to another provider format.
func pinDirectVideoTaskCandidate(ctx *Ctx, selected *routing.SelectedChannel) (*routing.SelectedChannel, error) {
	if ctx == nil || ctx.videoTask == nil {
		return selected, nil
	}
	if selected == nil || selected.Direct == nil {
		return nil, errDirectVideoUnavailable
	}
	bit := directVideoTaskProtocol(ctx.videoTask.Identity)
	d := selected.Direct
	if bit&store.DirectVideoProtocols == 0 || d.Protocols&bit == 0 || d.Endpoints.ForProtocol(bit) == nil {
		return nil, errDirectVideoUnavailable
	}
	if len(d.ProtocolOrder) != 0 {
		found := false
		for _, allowed := range d.ProtocolOrder {
			found = found || allowed == bit
		}
		if !found {
			return nil, errDirectVideoUnavailable
		}
	}
	copy, direct := *selected, *d
	direct.ProtocolOrder = store.DirectProtocolOrder{bit}
	copy.Direct = &direct
	return &copy, nil
}

func videoIdentity(ctx *Ctx, d *store.DirectUpstreamCandidate) directVideoIdentity {
	// Endpoint/auth/config changes may point the same task ID at another tenant.
	// Secret is deliberately excluded: rotating a credential keeps its identity.
	protocol, endpoint := directVideoSelectedEndpoint(d)
	config, _ := json.Marshal(struct {
		Endpoint                            *store.DirectEndpoint
		Provider, Kind, Headers, Parameters string
	}{endpoint, d.Provider, d.CredentialKind, d.CustomHeader, d.ParamOverride})
	hash := sha256.Sum256(config)
	identity := directVideoIdentity{Version: 2, Protocol: protocol, Owner: directVideoOwner(ctx.Auth),
		RouteID: d.RouteID, GroupID: d.GroupID, ItemID: d.ItemID,
		ChannelID: d.ChannelID, GrantID: d.GrantID, CredentialID: d.CredentialID,
		ModelID: d.ModelID, EndpointHash: hex.EncodeToString(hash[:])}
	if protocol == store.DirectProtocolVideo && (ctx.videoTask == nil || ctx.videoTask.Identity.Version == 1) {
		// Keep ordinary Video tasks readable by the previous binary during a
		// rolling upgrade; only native formats require the extended identity.
		identity.Version, identity.Protocol = 1, 0
	}
	if ctx.videoTask != nil {
		identity.SourcePublicID = ctx.videoTask.Identity.SourcePublicID
	}
	return identity
}

var errDirectVideoUnavailable = errors.New("video task is unavailable for this client or route")

func loadDirectVideoTask(db *store.DB, publicID string) (*directVideoTask, error) {
	if db == nil {
		return nil, errors.New("video task storage is unavailable")
	}
	task := &directVideoTask{PublicID: publicID}
	var identity, created string
	err := db.QueryRow(`SELECT upstream_video_id, requested_model, actual_model, direct_identity, created_at
		FROM proxy_video_tasks WHERE public_id = ? AND direct_identity IS NOT NULL`, publicID).
		Scan(&task.UpstreamID, &task.RequestedModel, &task.ActualModel, &identity, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errDirectVideoUnavailable
	}
	if err != nil {
		return nil, errors.New("video task storage is unavailable")
	}
	if err := json.Unmarshal([]byte(identity), &task.Identity); err != nil ||
		(task.Identity.Version != 1 && task.Identity.Version != 2) ||
		(task.Identity.Version == 1 && task.Identity.Protocol != 0) ||
		!store.ValidDirectProtocol(directVideoTaskProtocol(task.Identity)) || directVideoTaskProtocol(task.Identity)&store.DirectVideoProtocols == 0 ||
		task.Identity.Owner == "" || task.Identity.ItemID <= 0 || !validVideoUpstreamID(task.UpstreamID) {
		return nil, errDirectVideoUnavailable
	}
	createdAt, err := time.Parse(time.RFC3339, created)
	if err != nil || (videoTaskCacheTTL() > 0 && videoTaskNow().UTC().Sub(createdAt) >= videoTaskCacheTTL()) {
		return nil, errDirectVideoUnavailable
	}
	return task, nil
}

func validVideoUpstreamID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, "\r\n\x00")
}

func handleDirectVideoTask(w http.ResponseWriter, r *http.Request, suffix string) {
	pac := GetProxyAuth(r)
	if pac == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid_request_error")
		return
	}
	task, err := loadDirectVideoTask(store.GetDB(), chi.URLParam(r, "id"))
	if err != nil || task.Identity.Owner != directVideoOwner(pac) {
		status := http.StatusNotFound
		if err != nil && !errors.Is(err, errDirectVideoUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeJSONError(w, status, errDirectVideoUnavailable.Error(), "invalid_request_error")
		return
	}
	if suffix == "/content" && directVideoTaskProtocol(task.Identity) != store.DirectProtocolVideo &&
		!validDirectNativeVideoContentVariant(r.URL.Query().Get("variant")) {
		writeJSONError(w, http.StatusBadRequest, "unsupported native video content variant", "invalid_request_error")
		return
	}
	ctx, result := PrepareCtx(r, SurfConfig{Endpoint: "videos", DownstreamPath: "/v1/videos/" + url.PathEscape(task.UpstreamID) + suffix,
		DefaultModel: task.RequestedModel})
	if result != nil {
		writeJSONError(w, result.Status, result.Error, result.ErrorType)
		return
	}
	if ctx.RequestedModel != task.RequestedModel || ctx.IsStream {
		writeJSONError(w, http.StatusBadRequest, "video task model cannot change and streaming is unsupported", "invalid_request_error")
		return
	}
	ctx.videoTask = task
	channelID := -task.Identity.ItemID
	ctx.ForcedChannelID = &channelID
	dispatchUpstream(w, r, ctx)
}

// HandleVideosContent downloads content under the same task identity as polling.
func HandleVideosContent(w http.ResponseWriter, r *http.Request) {
	handleDirectVideoTask(w, r, "/content")
}

// HandleVideosRemix creates a separate owned task on the source task's grant.
func HandleVideosRemix(w http.ResponseWriter, r *http.Request) {
	handleDirectVideoTask(w, r, "/remix")
}

// Called after selection and before any upstream I/O. Reading the graph here
// also rejects disabled/deleted entities while the router cache is still warm.
func validateVideoTaskSelection(ctx *Ctx, selected *routing.SelectedChannel, path string) error {
	if ctx == nil || selected == nil || !strings.HasPrefix(path, "/v1/videos") {
		return nil
	}
	if selected.Direct == nil {
		if ctx.videoTask != nil {
			return errDirectVideoUnavailable
		}
		return nil
	}
	if path != "/v1/videos" && ctx.videoTask == nil {
		return errDirectVideoUnavailable
	}
	d := selected.Direct
	protocol, endpoint := directVideoSelectedEndpoint(d)
	if ctx.IsStream || directVideoOwner(ctx.Auth) == "" || endpoint == nil ||
		(protocol == store.DirectProtocolVideo && endpoint.Profile != "") ||
		(protocol == store.DirectProtocolSeedanceVideo && endpoint.Profile != "seedance-video") ||
		(protocol == store.DirectProtocolZenmuxVideo && endpoint.Profile != "zenmux-video") {
		return errDirectVideoUnavailable
	}
	want := videoIdentity(ctx, d)
	if ctx.videoTask != nil && (ctx.videoTask.Identity != want || selected.ActualModel != ctx.videoTask.ActualModel) {
		return errDirectVideoUnavailable
	}
	db := store.GetDB()
	if db == nil {
		return errors.New("video task storage is unavailable")
	}
	// Use the routing owner's full policy/matcher logic with a request-local
	// cache. Loading channels by their old route ID alone misses route renames
	// and explicit-group changes while another worker retains an old match.
	policy := routingPolicyFromAuth(ctx.Policy)
	policy.RequiredUpstreamProtocol = protocol
	policy.AllowUpstreamProtocolConversion = false
	selector := routing.NewChannelSelector(service.NewProxyRoutingStore(db), routing.NewRouteCache(0),
		0, routing.RoutingWeightsConfig{}, nil, 1, nil)
	fresh, err := selector.SelectPreferredChannel(context.Background(), ctx.RequestedModel, -d.ItemID, policy, nil)
	if err != nil || fresh == nil || fresh.Direct == nil {
		return errDirectVideoUnavailable
	}
	// The fresh selector checks current grant/member authorization. Its order
	// may differ; compare the same actual endpoint after that authorization.
	freshCopy, directCopy := *fresh, *fresh.Direct
	directCopy.ProtocolOrder = store.DirectProtocolOrder{protocol}
	freshCopy.Direct = &directCopy
	if videoIdentity(ctx, freshCopy.Direct) != want || freshCopy.ActualModel != selected.ActualModel {
		return errDirectVideoUnavailable
	}
	return nil
}

// Called only for upstream 2xx, before response headers or success accounting.
// A failed durable write is terminal: retrying a POST could create duplicates.
func processVideoTaskResponse(ctx *Ctx, selected *routing.SelectedChannel, method, path string, body []byte) ([]byte, error) {
	if ctx == nil || selected == nil || selected.Direct == nil {
		return maybeRewriteVideosCreateResponse(ctx, selected, path, body), nil
	}
	if !strings.HasPrefix(path, "/v1/videos") {
		return body, nil
	}
	if method == http.MethodGet && isDirectVideoContentPath(path) {
		return body, nil
	}
	if method == http.MethodDelete && ctx.videoTask != nil {
		if store.GetDB() == nil {
			return nil, errors.New("video task storage is unavailable")
		}
		ctx.videoUsageVerified = true
		if len(body) == 0 {
			return body, nil
		}
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, errors.New("invalid upstream video response")
	}
	if method == http.MethodPost {
		var upstreamID string
		_ = json.Unmarshal(payload["id"], &upstreamID)
		if !validVideoUpstreamID(upstreamID) {
			return nil, errors.New("upstream video response has no valid task ID")
		}
		publicID := directVideoIDPrefix + strings.TrimPrefix(newPublicVideoID(), "video_")
		taskIdentity := videoIdentity(ctx, selected.Direct)
		if ctx.videoTask != nil {
			taskIdentity.SourcePublicID = ctx.videoTask.PublicID
		}
		identity, err := json.Marshal(taskIdentity)
		if err != nil {
			return nil, errors.New("video task identity cannot be encoded")
		}
		db := store.GetDB()
		if db == nil {
			return nil, errors.New("video task storage is unavailable")
		}
		now := videoTaskNow().UTC().Format(time.RFC3339)
		_, err = db.Exec(`INSERT INTO proxy_video_tasks (public_id, upstream_video_id, site_url, token_value,
			requested_model, actual_model, channel_id, direct_identity, accounting_state, created_at, updated_at)
			VALUES (?, ?, '', '', ?, ?, ?, ?, '{"version":1}', ?, ?)`, publicID, upstreamID, ctx.RequestedModel, selected.ActualModel,
			-selected.Direct.ItemID, string(identity), now, now)
		if err != nil {
			return nil, errors.New("video task could not be persisted")
		}
		ctx.videoAccountingTask = &directVideoTask{PublicID: publicID, UpstreamID: upstreamID, RequestedModel: ctx.RequestedModel, ActualModel: selected.ActualModel, Identity: taskIdentity}
		ctx.videoUsageVerified = true
		payload["id"], _ = json.Marshal(publicID)
		if ctx.videoTask != nil {
			if _, ok := payload["remixed_from_video_id"]; ok {
				payload["remixed_from_video_id"], _ = json.Marshal(ctx.videoTask.PublicID)
			}
		}
	} else if ctx.videoTask != nil {
		var returnedID string
		_ = json.Unmarshal(payload["id"], &returnedID)
		if method == http.MethodGet && returnedID != ctx.videoTask.UpstreamID {
			return nil, fmt.Errorf("upstream returned a different video task")
		}
		ctx.videoUsageVerified = true
		payload["id"], _ = json.Marshal(ctx.videoTask.PublicID)
		if _, ok := payload["remixed_from_video_id"]; ok {
			if ctx.videoTask.Identity.SourcePublicID != "" {
				payload["remixed_from_video_id"], _ = json.Marshal(ctx.videoTask.Identity.SourcePublicID)
			} else {
				payload["remixed_from_video_id"] = json.RawMessage("null")
			}
		}
	}
	return json.Marshal(payload)
}
