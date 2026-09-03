package admin

import (
	"log/slog"
	"net/http"
	"strings"

	balanceService "github.com/deliciousbuding/metapi-go/service/balance"
)

// ---- Health Refresh ----

// healthRefreshResultItem is one account outcome from POST /api/accounts/health/refresh.
type healthRefreshResultItem struct {
	AccountID int64  `json:"accountId"`
	Status    string `json:"status"` // success | failed | skipped
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	ProxyOnly bool   `json:"proxyOnly,omitempty"`
}

// healthRefreshSummary aggregates wait-mode / background-task results.
type healthRefreshSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Degraded  int `json:"degraded"`
	Disabled  int `json:"disabled"`
	Unknown   int `json:"unknown"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func (h *accountsHandler) refreshBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	result, err := balanceService.RefreshBalance(h.cfg, h.db, id)
	if result == nil && err == nil {
		writeErrorWithRequest(w, r, http.StatusNotFound, "account not found or platform not supported")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			writeErrorWithRequest(w, r, http.StatusNotFound, "account not found or platform not supported")
			return
		}
		slog.Warn("Balance refresh failed", "err", err, "account_id", id)
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "balance refresh failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"balance":     result.Balance,
		"balanceUsed": result.Used,
		"quota":       result.Quota,
		"skipped":     result.Skipped,
		"reason":      result.Reason,
	})
}
