package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

type downstreamResponseModelKey struct{}

func withDownstreamResponseModel(r *http.Request, ctx *Ctx) *http.Request {
	if ctx == nil || ctx.Policy.AccessPolicy == nil || ctx.Policy.AccessPolicy.MapModel(ctx.RequestedModel) == ctx.RequestedModel {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), downstreamResponseModelKey{}, ctx.RequestedModel))
}

func downstreamResponseModel(r *http.Request) string {
	model, _ := r.Context().Value(downstreamResponseModelKey{}).(string)
	return model
}

// Restore only protocol-owned model metadata. Never walk arbitrary tool
// arguments, user content or output objects looking for similarly named keys.
func restoreDownstreamResponseModel(raw []byte, model string) []byte {
	if model == "" {
		return raw
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return raw
	}
	changed := false
	for _, field := range []string{"model", "modelVersion"} {
		var existing string
		if json.Unmarshal(object[field], &existing) == nil && existing != "" && existing != model {
			object[field], _ = json.Marshal(model)
			changed = true
		}
	}
	for _, field := range []string{"response", "message"} {
		if value, ok := object[field]; ok {
			next := restoreDownstreamResponseModel(value, model)
			if !bytes.Equal(next, value) {
				object[field] = next
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(object)
	if err != nil {
		return raw
	}
	return out
}

func restoreBufferedDownstreamResponseModel(resp *http.Response, raw []byte, model string) []byte {
	if model == "" {
		return raw
	}
	out := restoreDownstreamResponseModel(raw, model)
	if !bytes.Equal(out, raw) {
		for _, h := range []string{"Content-Length", "ETag", "Content-MD5", "Digest"} {
			resp.Header.Del(h)
		}
	}
	return out
}
