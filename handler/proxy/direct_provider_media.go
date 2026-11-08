package proxyhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func prepareDirectMediaDriver(ctx context.Context, proxyCtx *Ctx, cfg *UpstreamConfig, wire *directProviderWire, direct *store.DirectUpstreamCandidate, endpoint *store.DirectEndpoint, protocol, kind, path, method, model string) error {
	hook := ""
	switch {
	case method == http.MethodPost && path == "/v1/videos":
		hook = "submit"
	case method == http.MethodGet && isDirectVideoContentPath(path):
		hook = "content"
	case method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/"):
		hook = "query"
	default:
		return channelplugin.ErrManifest
	}
	inputBody := ""
	if hook == "submit" {
		if proxyCtx.Multipart && wire.DriverUpload == nil {
			return channelplugin.ErrManifest
		}
		var object map[string]json.RawMessage
		if requestoverride.Decode(wire.Body, &object) != nil || object == nil || requestoverride.RejectDuplicateJSONKeys(wire.Body) != nil {
			return channelplugin.ErrResult
		}
		inputBody = string(wire.Body)
	}
	fingerprint, taskID := "", ""
	if proxyCtx.videoTask != nil {
		fingerprint = proxyCtx.videoTask.Identity.DriverFingerprint
		taskID = proxyCtx.videoTask.UpstreamID
	}
	if hook != "submit" && fingerprint == "" {
		return channelplugin.ErrManifest
	}
	requestKind := "json"
	if hook == "submit" && wire.DriverUpload != nil {
		requestKind = "multipart"
	}
	var invocation channelplugins.DriverInvocation
	var err error
	if hook == "submit" {
		invocation, err = cfg.ChannelPlugins.CaptureTaskDriver(ctx, *direct.RequestConfig.Provider, protocol, kind, requestKind, func(tx *sqlx.Tx, captured string) (string, error) {
			return captureDirectVideoDriver(ctx, tx, proxyCtx, direct, model, captured)
		})
	} else {
		invocation, err = cfg.ChannelPlugins.ResolveTaskDriver(ctx, *direct.RequestConfig.Provider, protocol, kind, fingerprint, requestKind)
	}
	if err != nil {
		return err
	}
	input := channelplugin.DriverInput{Body: inputBody, Model: model, TaskID: taskID}
	if hook == "submit" && wire.DriverUpload != nil {
		input.BodyKind = "multipart"
		input.Files = wire.DriverUpload.metadata
	}
	raw, err := invocation.Call(ctx, hook, input)
	if err != nil {
		return err
	}
	operationPath := path
	if hook == "content" {
		var decision struct {
			Query bool `json:"query"`
		}
		if requestoverride.Decode(raw, &decision) == nil && decision.Query {
			wire.DriverContentQuery = true
			operationPath = strings.TrimSuffix(path, "/content")
			raw, err = invocation.Call(ctx, "query", channelplugin.DriverInput{Model: model, TaskID: taskID})
			if err != nil {
				return err
			}
		}
	}
	var descriptor channelplugin.RequestDescriptor
	if requestoverride.Decode(raw, &descriptor) != nil || descriptor.Method != method || method == http.MethodGet && descriptor.Body != "" {
		return channelplugin.ErrResult
	}
	prepared, err := directDriverDescriptorBody(descriptor, wire)
	if err != nil {
		return err
	}
	if method == http.MethodPost && !sameDriverRequestControls(protocol, wire.Body, prepared) {
		return channelplugin.ErrResult
	}
	address, _ := directRequestURL(direct, operationPath, model, false, endpoint)
	if validateDriverOperationURL(descriptor, address) != nil {
		return channelplugin.ErrResult
	}
	if descriptor.ContentType != "" && descriptor.ContentType != "application/json" {
		return channelplugin.ErrResult
	}
	wire.Driver, wire.DriverURL, wire.DriverContentType = &invocation, address, descriptor.ContentType
	wire.DriverOperation, wire.DriverTaskID = hook, taskID
	wire.PluginProtocol, wire.PluginModel = protocol, model
	wire.Body = prepared
	if wire.Headers == nil {
		wire.Headers = make(http.Header)
	}
	for key, value := range descriptor.Headers {
		wire.Headers.Set(key, value)
	}
	proxyCtx.videoDriverFingerprint = invocation.Fingerprint()
	return nil
}

// Raw identity establishes the task relationship before executable parsing.
// A throwing decoder cannot erase a received, already-verified usage delta.
func captureDirectMediaTaskUsage(ctx context.Context, proxyCtx *Ctx, selected *routing.SelectedChannel, wire *directProviderWire, usage ParsedUsage) error {
	if wire == nil || wire.DriverOperation == "" || proxyCtx == nil || proxyCtx.videoTask == nil || usage.invalidForUserBudget || usage.upstreamTaskID != proxyCtx.videoTask.UpstreamID {
		return nil
	}
	proxyCtx.videoUsageVerified = true
	_, err := accountDirectVideoUsage(ctx, proxyCtx, selected, usage)
	return err
}

// A known task ID is retained even when its first response cannot be decoded.
// The request remains failed; its billing details link the recoverable public
// task. No response state or successful generation is invented here.
func retainFailedDirectMediaSubmission(ctx context.Context, proxyCtx *Ctx, selected *routing.SelectedChannel, wire *directProviderWire, usage ParsedUsage) error {
	if wire == nil || wire.DriverOperation != "submit" || proxyCtx == nil || proxyCtx.videoTask != nil || usage.invalidForUserBudget || !validObservedVideoTaskID(usage.upstreamTaskID) {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"id": usage.upstreamTaskID})
	if _, err := processVideoTaskResponse(proxyCtx, selected, http.MethodPost, "/v1/videos", body); err != nil {
		return err
	}
	_, err := accountDirectVideoUsage(ctx, proxyCtx, selected, usage)
	return err
}
