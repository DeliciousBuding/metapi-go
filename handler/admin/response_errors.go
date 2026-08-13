package admin

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

// writeError writes a unified admin API error: non-2xx status + camelCase JSON
// {"error":"..."}. Prefer this over writeJSON(..., {"success":false,...}) for
// mutation failure paths so clients never see HTTP 200 with an error body.
func writeError(w http.ResponseWriter, code int, message string) {
	shared.WriteError(w, code, message)
}
