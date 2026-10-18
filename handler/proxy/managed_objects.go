package proxyhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
)

// processManagedProxyResponse applies ownership and id-rewrite rules to a
// buffered 2xx response from a shared (non-direct) upstream. It returns an
// error when the response cannot be made safe to relay; the caller turns that
// into 502 instead of passing the upstream body through.
//
// The Files list is the case that must never fall back to a pass-through: it
// contains the whole upstream account's inventory. Upload responses create the
// ownership record, and deletes drop it only after this 2xx was observed.
func processManagedProxyResponse(
	ctx *Ctx,
	selected *routing.SelectedChannel,
	method string,
	upstreamPath string,
	body []byte,
) ([]byte, error) {
	path := strings.TrimSpace(upstreamPath)
	if path == "" && ctx != nil {
		path = strings.TrimSpace(ctx.DownstreamPath)
	}
	path, _, _ = strings.Cut(path, "?")
	path = strings.TrimSuffix(path, "/")

	pac := ctxAuth(ctx)

	// Shared-upstream video deletion: the handler resolved the mapping before
	// dispatch, so the public id is the one the client used. The mapping is
	// removed here, after upstream 2xx, so a failed upstream delete never
	// strands the task without its rewrite entry.
	if method == http.MethodDelete && ctx != nil && ctx.managedVideoTaskID != "" {
		DeleteProxyVideoTaskByPublicID(ctx.managedVideoTaskID)
		return body, nil
	}

	switch {
	case method == http.MethodPost && strings.EqualFold(path, "/v1/files"):
		if id, ok := proxyFileIDFromResponse(body); ok {
			recordProxyFileObject(pac, id)
		} else {
			slog.Warn("proxy files: upload response has no usable file id; ownership not recorded")
		}
		return body, nil
	case method == http.MethodGet && strings.EqualFold(path, "/v1/files"):
		return filterProxyFileList(body, proxyFileOwner(pac))
	case method == http.MethodDelete:
		if fileID, ok := managedFileIDFromPath(path); ok {
			deleteProxyFileObject(fileID)
		}
		return body, nil
	case method == http.MethodPost && strings.EqualFold(path, "/v1/videos"):
		return maybeRewriteVideosCreateResponse(ctx, selected, upstreamPath, body), nil
	}
	return body, nil
}

func ctxAuth(ctx *Ctx) *auth.ProxyAuthContext {
	if ctx == nil {
		return nil
	}
	return ctx.Auth
}

// managedFileIDFromPath returns the file id of an id-addressed Files path.
// Only the exact /v1/files/{id} shape qualifies; list and content paths never
// delete ownership here.
func managedFileIDFromPath(path string) (string, bool) {
	const prefix = "/v1/files/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(path, prefix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}
