package admin

import (
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/go-chi/chi/v5"
)

// RegisterNotifyRoutes registers the /api/settings/notify/test route.
func RegisterNotifyRoutes(r chi.Router) {
	r.Post("/api/settings/notify/test", testNotify)
}

// POST /api/settings/notify/test
// Dispatches a real connectivity test through all configured notification channels.
// Returns a clear 400 failure when no channel is configured or all sends fail.
func testNotify(w http.ResponseWriter, r *http.Request) {
	// Notification channels live entirely in the runtime snapshot; take one
	// atomic snapshot so a concurrent settings update cannot half-apply
	// mid-dispatch.
	rt := config.Runtime()

	result, err := notify.SendNotification(
		rt,
		"Test notification",
		"This is a connectivity test notification from system settings; your notification configuration is working correctly!",
		string(notify.LevelInfo),
		&notify.SendNotificationOptions{
			BypassThrottle: true,
			RequireChannel: true,
			ThrowOnFailure: true,
		},
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	message := fmt.Sprintf("test notification sent (%d/%d succeeded)", result.Succeeded, result.Attempted)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": message,
	})
}
