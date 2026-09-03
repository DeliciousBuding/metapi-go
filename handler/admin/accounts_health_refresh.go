package admin

import (
	"log/slog"
	"net/http"
	"strings"

	balanceService "github.com/deliciousbuding/metapi-go/service/balance"
)

// ---- Health Refresh ----

// balanceRefreshFailedMessage is the stable prefix of the on-demand balance
// refresh failure body. Scripts and the UI match on it, so a classified reason is
// appended after it rather than replacing it (#1210).
const balanceRefreshFailedMessage = "balance refresh failed"

// batchBalanceRefreshFailedMessage is the same contract on the batch surface. Its
// capitalisation predates #1210 and is kept so existing UI strings still match;
// both prefixes stay stable and only the appended reason changes.
const batchBalanceRefreshFailedMessage = "Balance refresh failed"

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
		// #1210: the server already knew why this failed (the WARN line carried
		// `HTTP 401: Token has expired`) while the operator and any script
		// asserting on the body saw only a generic failure and had to go log
		// diving. Keep the stable prefix so existing assertions and the UI keep
		// matching, keep the raw upstream text server-side because it can carry a
		// URL, a token fragment or a request id, and append one classified reason.
		// When nothing safe can be named the bare prefix stays rather than an
		// invented cause.
		reason := balanceService.ExplainRefreshFailure(err)
		slog.Warn("Balance refresh failed", "err", err, "account_id", id, "reason", reason)
		message := balanceRefreshFailedMessage
		if reason != "" {
			message += ": " + reason
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": message})
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
