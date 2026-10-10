package proxyhandler

import "net/http"

func HandleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	handleMediaSurface(w, r, "/v1/audio/speech", true, "")
}
func HandleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	handleMediaSurface(w, r, "/v1/audio/transcriptions", true, "")
}
func HandleAudioTranslations(w http.ResponseWriter, r *http.Request) {
	handleMediaSurface(w, r, "/v1/audio/translations", true, "")
}
func HandleModerations(w http.ResponseWriter, r *http.Request) {
	handleMediaSurface(w, r, "/v1/moderations", false, "omni-moderation-latest")
}

func handleMediaSurface(w http.ResponseWriter, r *http.Request, path string, required bool, defaultModel string) {
	ctx, failure := PrepareCtx(r, SurfConfig{Endpoint: "media", DownstreamPath: path, RequireModel: required, DefaultModel: defaultModel})
	if failure != nil {
		writeJSONError(w, failure.Status, failure.Error, failure.ErrorType)
		return
	}
	dispatchUpstream(w, r, ctx)
}
