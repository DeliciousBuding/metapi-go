package admin

import "net/http"

// noStoreCacheControl is the response policy for admin reads that can carry
// credentials, masked secrets, or operator-only configuration. A browser,
// shared proxy, or service worker must not retain the body.
const noStoreCacheControl = "no-store, no-cache, must-revalidate, private, max-age=0"

// setNoStore marks the current admin response as uncacheable. Call it before
// writing the status line.
func setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", noStoreCacheControl)
}
