package admin

import (
	"log/slog"
	"net/http"
)

// ---- Proxy Log Detail ----
// GET /api/stats/proxy-logs/:id
func (h *statsHandler) proxyLogDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row, err := queryRow(h.db,
		`SELECT pl.*, a.username, s.id as site_id, s.name as site_name, s.url as site_url,
		 dk.id AS downstream_key_id, dk.name AS downstream_key_name
		 FROM proxy_logs pl
		 LEFT JOIN accounts a ON pl.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id
		 LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id
		 WHERE pl.id = ?`, id)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load proxy log")
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "proxy log not found"})
		return
	}

	projectProxyAttemptDetail(row)
	row["requestAttempts"] = h.buildRequestAttempts(row)

	writeJSON(w, http.StatusOK, row)
}

// buildRequestAttempts assembles the request-level attempt view for one log
// row (route-decision D batch). Every dispatch attempt — including the final
// one — persists its own proxy_logs row; request_id correlates the rows of
// one client request and retry_count is the retry-loop ordinal.
//
// The answer is read-only over already-persisted rows: sibling attempts
// ordered by ordinal with minimal decision metadata (channel/site identity,
// status, error, latency), the last recorded attempt, and an
// honest partial marker when the persisted set is known incomplete:
//
//   - requestIdMissing: the row predates request_id correlation, so siblings
//     cannot be verified; no attempts list is returned rather than presenting
//     one unverifiable row as the whole request.
//   - ordinalGap: the ordinal sequence has holes (e.g. a selected channel was
//     skipped on site-concurrency saturation without dispatching). The gap is
//     reported, never back-filled with invented attempts.
//
// An attempt does not establish the gateway response or client receipt. The
// legacy terminal field is retained for wire compatibility only; new consumers
// use lastAttempt, which remains null when correlation or its query is missing.
func (h *statsHandler) buildRequestAttempts(current map[string]any) map[string]any {
	currentID := coerceInt64(current["id"])
	currentOrdinal := coerceInt(current["retryCount"])
	result := map[string]any{
		"requestId":      nil,
		"currentOrdinal": currentOrdinal,
		"partial":        false,
		"partialReasons": []string{},
		"attempts":       []any{},
		"lastAttempt":    nil,
		"terminal": map[string]any{
			"status":     current["status"],
			"httpStatus": current["httpStatus"],
			"logId":      currentID,
		},
	}
	requestID := coerceString(current["requestId"])
	gatewayRequestID := coerceString(current["gatewayRequestId"])
	if requestID == "" && gatewayRequestID == "" {
		result["partial"] = true
		result["partialReasons"] = []string{"requestIdMissing"}
		return result
	}
	if requestID != "" {
		result["requestId"] = requestID
	}
	// An external ID may repeat across independent HTTP requests or WS turns.
	// New observations always correlate by the gateway-owned identity; retain
	// external-ID grouping only for historical rows that lack that observation.
	correlationColumn, correlationID := "pl.request_id", requestID
	if gatewayRequestID != "" {
		correlationColumn, correlationID = "pl.gateway_request_id", gatewayRequestID
	}

	rows, err := queryRowsErr(h.db, `
		SELECT pl.id, pl.retry_count, pl.status, pl.http_status, pl.channel_id,
		 pl.model_actual, pl.error_message, pl.latency_ms, pl.created_at,
		 a.username, s.id AS site_id, s.name AS site_name
		FROM proxy_logs pl
		LEFT JOIN accounts a ON pl.account_id = a.id
		LEFT JOIN sites s ON a.site_id = s.id
		WHERE `+correlationColumn+` = ?
		ORDER BY pl.retry_count ASC, pl.id ASC`, correlationID)
	if err != nil {
		slog.Warn("request attempts query failed", "logId", currentID, "error", err)
		result["partial"] = true
		result["partialReasons"] = []string{"attemptQueryFailed"}
		return result
	}

	attempts := make([]any, 0, len(rows))
	seenOrdinals := make(map[int]bool, len(rows))
	maxOrdinal := -1
	for _, sib := range rows {
		ordinal := coerceInt(sib["retryCount"])
		seenOrdinals[ordinal] = true
		if ordinal > maxOrdinal {
			maxOrdinal = ordinal
		}
		sibID := coerceInt64(sib["id"])
		attempts = append(attempts, map[string]any{
			"id":              sibID,
			"ordinal":         ordinal,
			"status":          coerceString(sib["status"]),
			"httpStatus":      coerceInt(sib["httpStatus"]),
			"channelId":       sib["channelId"],
			"siteId":          sib["siteId"],
			"siteName":        coerceString(sib["siteName"]),
			"accountUsername": coerceString(sib["username"]),
			"modelActual":     coerceString(sib["modelActual"]),
			"errorMessage":    coerceString(sib["errorMessage"]),
			"latencyMs":       coerceInt64(sib["latencyMs"]),
			"createdAt":       coerceString(sib["createdAt"]),
			"isCurrent":       sibID == currentID,
		})
	}
	result["attempts"] = attempts

	// Ordinal gaps mean at least one loop iteration left no row (the
	// concurrency-skip path deliberately does not log).
	for ordinal := 0; ordinal <= maxOrdinal; ordinal++ {
		if !seenOrdinals[ordinal] {
			result["partial"] = true
			result["partialReasons"] = []string{"ordinalGap"}
			break
		}
	}

	if len(rows) > 0 {
		last := rows[len(rows)-1]
		result["terminal"] = map[string]any{
			"status":     coerceString(last["status"]),
			"httpStatus": coerceInt(last["httpStatus"]),
			"logId":      coerceInt64(last["id"]),
		}
		result["lastAttempt"] = map[string]any{
			"status":     coerceString(last["status"]),
			"httpStatus": last["httpStatus"],
			"logId":      coerceInt64(last["id"]),
		}
	}
	return result
}
