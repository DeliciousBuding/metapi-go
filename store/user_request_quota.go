package store

import "errors"

// UserRequestQuota exposes only the request's durable token-credit snapshot,
// not upstream cost or current group pricing. Nil measurements remain unknown.
type UserRequestQuota struct {
	Unit            string   `json:"unit"`
	State           string   `json:"state"`
	TotalTokens     *int64   `json:"totalTokens"`
	GroupMultiplier float64  `json:"groupMultiplier"`
	Reserved        float64  `json:"reserved"`
	Charged         *float64 `json:"charged"`
}

// Request ownership is authoritative even when the independently persisted
// ledger is missing or has a conflicting owner. One join serves the whole page;
// neither Key existence nor current group membership can change historical data.
const userRequestSelect = `SELECT r.id,r.model_requested,r.terminal_status,r.latency_ms,r.started_at,r.ended_at,r.attempt_count,
	q.request_id,q.state,q.total_tokens,q.multiplier_units,q.reserved_units,q.charged_units
	FROM gateway_requests r LEFT JOIN gateway_user_budget_ledger q ON q.request_id=r.id AND q.user_id=r.owner_user_id`

func scanUserRequest(row rowScanner) (UserRequest, error) {
	var item UserRequest
	var requestID, state *string
	var tokens, multiplier, reserved, charged *int64
	if err := row.Scan(&item.ID, &item.ModelRequested, &item.TerminalStatus, &item.LatencyMs, &item.StartedAt, &item.EndedAt, &item.AttemptCount,
		&requestID, &state, &tokens, &multiplier, &reserved, &charged); err != nil {
		return UserRequest{}, err
	}
	if requestID == nil {
		return item, nil
	}
	// Stored corruption must fail the read, not turn an invalid quota into a
	// missing ledger or expose negative/unknown-as-zero measurements to clients.
	if state == nil || multiplier == nil || *multiplier <= 0 || reserved == nil || *reserved < 0 ||
		tokens != nil && *tokens < 0 || charged != nil && *charged < 0 {
		return UserRequest{}, errors.New("invalid stored request quota")
	}
	switch *state {
	case "pending", "settled", "released", "unknown":
	default:
		return UserRequest{}, errors.New("invalid stored request quota state")
	}
	quota := &UserRequestQuota{
		Unit: "token_credits", State: *state, TotalTokens: tokens,
		GroupMultiplier: float64(*multiplier) / float64(userBudgetScale),
		Reserved:        float64(*reserved) / float64(userBudgetScale),
	}
	if charged != nil {
		value := float64(*charged) / float64(userBudgetScale)
		quota.Charged = &value
	}
	item.Quota = quota
	return item, nil
}
