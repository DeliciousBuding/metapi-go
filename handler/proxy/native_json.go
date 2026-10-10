package proxyhandler

import "net/http"

func HandleSystemOne(w http.ResponseWriter, r *http.Request) {
	handleNativeJSONSurface(w, r, "/v1/systemone")
}

func HandleAlphaSearch(w http.ResponseWriter, r *http.Request) {
	handleNativeJSONSurface(w, r, "/v1/alpha/search")
}

// These APIs have independent grants. Preserve their native JSON envelopes,
// including provider extensions and numeric precision, without Chat conversion.
func handleNativeJSONSurface(w http.ResponseWriter, r *http.Request, path string) {
	ctx, failure := PrepareCtx(r, SurfConfig{Endpoint: "native-json", DownstreamPath: path, RequireModel: true})
	if failure != nil {
		writeJSONError(w, failure.Status, failure.Error, failure.ErrorType)
		return
	}
	stream, present := ctx.Body["stream"]
	if ctx.IsStream || ctx.Multipart || present && stream != false {
		writeJSONError(w, http.StatusBadRequest, "This endpoint requires a non-streaming JSON request", "invalid_request_error")
		return
	}
	dispatchUpstream(w, r, ctx)
}
