package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// DownstreamTokenAuthResult is the result of authorizing a downstream token.
// On success (OK=true): Source, Token, Key, and Policy are populated.
// On failure (OK=false): StatusCode, Error, and Reason are populated.
type DownstreamTokenAuthResult struct {
	taskRead   bool
	OK         bool
	Source     string                  // "managed" (when OK)
	Token      string                  // normalized token (when OK)
	Key        *managedKeyView         // managed key view (when OK, source="managed")
	Policy     DownstreamRoutingPolicy // resolved policy (when OK)
	StatusCode int                     // HTTP status (when !OK)
	Error      string                  // error message (when !OK)
	Reason     string                  // "missing"|"invalid"|"disabled"|"expired"|"over_cost"|"over_requests"|"over_rpm"|"over_tpm"
	// QuotaReservationID is the admission hold created for an accessPolicy
	// quota window. The completion path settles it; the caller releases it if
	// the request ends without billable usage.
	QuotaReservationID string
}

// admissionOptions controls how the accessPolicy quota dimension is evaluated.
// The zero value keeps the historical read-only check.
type admissionOptions struct {
	// reserveQuota performs the atomic check-and-insert instead of a plain
	// read. Used by live HTTP admission and by per-message WebSocket turns.
	reserveQuota bool
	// estimateTokens supplies the input-token estimate for a token-limited
	// quota. It is called at most once and only when the quota has a token
	// limit; a nil function or a non-positive result reserves the 1-token
	// minimum that a chunked/unestimable body must not bypass.
	estimateTokens func() int64
	// An already admitted queued request refreshes identity and permissions
	// without consuming or reserving its request/budget a second time.
	authorizationOnly bool
}

// AuthorizeDownstreamToken validates a token against downstream_api_keys and
// enforces its enabled state, expiry and quota. Runtime root tokens are retired.
// It is the read-only admission variant: quota checks never create a hold.
func AuthorizeDownstreamToken(db *store.DB, token string, rt *config.RuntimeSettings) DownstreamTokenAuthResult {
	return authorizeDownstreamToken(db, token, rt, "", admissionOptions{})
}

func authorizeDownstreamToken(db *store.DB, token string, _ *config.RuntimeSettings, videoReadID string, opts admissionOptions) DownstreamTokenAuthResult {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return DownstreamTokenAuthResult{
			OK:         false,
			StatusCode: 401,
			Error:      "Missing Authorization or x-api-key header",
			Reason:     "missing",
		}
	}

	// ---- Check managed key table ----
	managed, err := getManagedKeyByToken(db, normalized)
	if err != nil {
		slog.Warn("downstream auth: failed to query API key", "error", err)
		return DownstreamTokenAuthResult{StatusCode: 503, Error: "API key authentication is temporarily unavailable", Reason: "unavailable"}
	}

	if managed != nil {
		// Invalid persisted restrictions must never become an unrestricted key.
		if managed.InvalidAccessPolicy || (managed.AccessPolicy != nil && managed.AccessPolicy.BlockReason != "") {
			return DownstreamTokenAuthResult{StatusCode: 403, Error: "API key access policy is blocked", Reason: "access_policy"}
		}

		// Check enabled
		if !managed.Enabled {
			return DownstreamTokenAuthResult{
				OK:         false,
				StatusCode: 403,
				Error:      "API key is disabled",
				Reason:     "disabled",
			}
		}
		if result := resolveManagedKeyUser(managed, db.DB); !result.OK {
			result.Key = managed
			return result
		}

		// Check expiration
		if managed.ExpiresAt != nil {
			expiresAt, parseErr := parseISO8601(*managed.ExpiresAt)
			if parseErr == nil && !expiresAt.After(time.Now()) {
				return DownstreamTokenAuthResult{
					OK:         false,
					StatusCode: 403,
					Error:      "API key is expired",
					Reason:     "expired",
				}
			}
		}

		taskRead := ownsDirectVideoTask(db, videoReadID, &ProxyAuthContext{Source: "managed", Token: normalized, KeyID: &managed.ID, OwnerUserID: managed.OwnerUserID})
		// Already admitted owned tasks remain retrievable when generation budget
		// is exhausted. Identity, expiry, IP and route policies still apply.
		//
		// max_cost is checked against the used_cost snapshot loaded above and
		// advanced only after a successful attempt (RecordManagedKeyCostUsage),
		// so concurrent in-flight requests can each pass this check and overshoot
		// the limit by roughly (concurrency x per-request cost). The increment
		// itself is a single atomic UPDATE; a pre-dispatch reservation is not
		// possible because the channel price is unknown before routing.
		if !opts.authorizationOnly && !taskRead && managed.MaxCost != nil && managed.UsedCost >= *managed.MaxCost {
			// 429 insufficient_quota (OpenAI convention for exhausted
			// per-key budget), not 403.
			return DownstreamTokenAuthResult{
				OK:         false,
				StatusCode: 429,
				Error:      "API key has exceeded max cost",
				Reason:     "over_cost",
			}
		}

		// Check max requests
		if !opts.authorizationOnly && !taskRead && managed.MaxRequests != nil && managed.UsedRequests >= *managed.MaxRequests {
			// 429 insufficient_quota (OpenAI convention for exhausted
			// per-key budget), not 403.
			return DownstreamTokenAuthResult{
				OK:         false,
				StatusCode: 429,
				Error:      "API key has exceeded max requests",
				Reason:     "over_requests",
			}
		}

		quotaReservationID := ""
		if !opts.authorizationOnly && !taskRead && managed.AccessPolicy != nil && managed.AccessPolicy.Quota != nil {
			if opts.reserveQuota {
				estimatedTokens := int64(1)
				if opts.estimateTokens != nil {
					if estimated := opts.estimateTokens(); estimated > 0 {
						estimatedTokens = estimated
					}
				}
				id, result := reserveManagedKeyQuota(db, managed.ID, managed.AccessPolicy.Quota, estimatedTokens)
				if !result.OK {
					return result
				}
				quotaReservationID = id
			} else if result := checkManagedKeyQuota(db, managed.ID, managed.AccessPolicy.Quota); !result.OK {
				return result
			}
		}

		// Success — build policy from managed key view
		policy := toPolicyFromView(managed)

		return DownstreamTokenAuthResult{
			OK:                 true,
			taskRead:           taskRead,
			Source:             "managed",
			Token:              normalized,
			Key:                managed,
			Policy:             policy,
			QuotaReservationID: quotaReservationID,
		}
	}

	// ---- No match ----
	// 401 (not 403): OpenAI/SDK convention treats an unknown key as an
	// authentication failure; 403 signals a non-retryable permission problem.
	return DownstreamTokenAuthResult{
		OK:         false,
		StatusCode: 401,
		Error:      "Invalid API key",
		Reason:     "invalid",
	}
}

// ReauthorizeManagedKeyForTurn re-runs the managed-key admission chain for one
// unit of work inside an already established session (a Responses WebSocket
// message). Long-lived connections otherwise keep the handshake snapshot: a
// disabled/expired key, a tightened accessPolicy, or an exhausted
// max_cost/quota/max_requests budget would stay usable until disconnect.
//
// The chain mirrors ProxyAuth exactly: reload by token (enabled, expiry,
// max_cost, max_requests, accessPolicy/quota), then IP rules, then RPM/TPM
// admission, then the atomic max_requests consume. A successful result
// therefore consumed exactly one request slot, reserved its quota slot, and
// carries the freshly loaded key + policy, which callers must use for the turn.
// A rejection after the quota reservation keeps the reservation ID so the
// caller can release it.
//
// estimatedTokens is reserved against maxTPM; when maxTPM is set a minimum of
// one token is reserved so an unestimable message cannot skip TPM accounting.
func ReauthorizeManagedKeyForTurn(ctx context.Context, db *store.DB, token, clientIP string, estimatedTokens int64) DownstreamTokenAuthResult {
	// RuntimeSafe keeps session-loop tests and early boot paths from panicking;
	// authorizeDownstreamToken independently loads the key from the store.
	// Each turn reserves its own quota slot so an open socket cannot let
	// concurrent messages exceed an accessPolicy quota window.
	result := authorizeDownstreamToken(db, token, config.RuntimeSafe(), "", admissionOptions{
		reserveQuota: true,
		estimateTokens: func() int64 {
			if estimatedTokens < 1 {
				return 1
			}
			return estimatedTokens
		},
	})
	if !result.OK || result.Source != "managed" || result.Key == nil {
		return result
	}

	// Later gates can still reject this turn. Keep the reservation ID on the
	// denial so the caller releases the slot it just acquired.
	deny := func(r DownstreamTokenAuthResult) DownstreamTokenAuthResult {
		r.QuotaReservationID = result.QuotaReservationID
		return r
	}

	if ok, denyReason := CheckDownstreamKeyIP(result.Key, clientIP); !ok {
		msg := "IP address not allowed for this API key"
		if denyReason == "ip_blocked" {
			msg = "IP address is blocked for this API key"
		}
		return deny(DownstreamTokenAuthResult{StatusCode: 403, Reason: denyReason, Error: msg})
	}

	if result.Key.MaxTPM != nil && *result.Key.MaxTPM > 0 && estimatedTokens < 1 {
		estimatedTokens = 1
	}
	adm := GlobalKeyAdmission.Allow(ctx, result.Key.ID, result.Key.MaxRPM, result.Key.MaxTPM, estimatedTokens)
	if !adm.Allowed {
		msg := "API key rate limit exceeded"
		if adm.Reason == "over_tpm" {
			msg = "API key token-per-minute limit exceeded"
		}
		return deny(DownstreamTokenAuthResult{StatusCode: 429, Reason: adm.Reason, Error: msg})
	}

	if !consumeManagedKeyRequest(db, result.Key.ID) {
		return deny(DownstreamTokenAuthResult{
			StatusCode: 429,
			Reason:     "over_requests",
			Error:      "API key has exceeded max requests",
		})
	}

	return result
}

// parseISO8601 parses an ISO 8601 / RFC 3339 timestamp string.
// Tries time.RFC3339 first, then time.RFC3339Nano.
func parseISO8601(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	// Try a few more common ISO 8601 variants
	layouts := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		t, err = time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse ISO 8601: %q", s)
}
