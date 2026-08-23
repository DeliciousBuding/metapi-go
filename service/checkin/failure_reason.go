package checkin

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/service/alert"
)

// FailureReasonCategory classifies the nature of a failure.
type FailureReasonCategory string

const (
	CategoryVerification FailureReasonCategory = "verification"
	CategoryAuth         FailureReasonCategory = "auth"
	CategoryNetwork      FailureReasonCategory = "network"
	CategorySite         FailureReasonCategory = "site"
	CategoryState        FailureReasonCategory = "state"
	CategoryUnknown      FailureReasonCategory = "unknown"
)

// FailureReasonCode identifies the specific failure.
type FailureReasonCode string

const (
	CodeSiteDisabled             FailureReasonCode = "site_disabled"
	CodeCheckinNotSupported      FailureReasonCode = "checkin_not_supported"
	CodeManualTurnstileRequired  FailureReasonCode = "manual_turnstile_required"
	CodeCloudflareTunnelUnavail  FailureReasonCode = "cloudflare_tunnel_unavailable"
	CodeCloudflareChallenge      FailureReasonCode = "cloudflare_challenge"
	CodeTokenExpired             FailureReasonCode = "token_expired"
	CodeAlreadyCheckedIn         FailureReasonCode = "already_checked_in"
	CodeNetworkTimeout           FailureReasonCode = "network_timeout"
	CodeUpstreamError            FailureReasonCode = "upstream_error"
	CodeUnknownError             FailureReasonCode = "unknown_error"
)

// FailureReason describes why an operation failed.
type FailureReason struct {
	Code       FailureReasonCode     `json:"code"`
	Category   FailureReasonCategory `json:"category"`
	Title      string                `json:"title"`
	ActionHint string                `json:"actionHint"`
	DetailHint string                `json:"detailHint"`
}

// ClassifyFailureInput is the input to classifyFailureReason.
type ClassifyFailureInput struct {
	Message    string
	Status     string
	HTTPStatus int
}

// ClassifyFailureReason classifies a failure into a structured reason.
// Mirrors TS classifyFailureReason().
func ClassifyFailureReason(input ClassifyFailureInput) FailureReason {
	rawMessage := strings.TrimSpace(input.Message)
	text := strings.ToLower(rawMessage)
	status := strings.ToLower(input.Status)
	httpStatus := input.HTTPStatus

	// Priority 1: Site disabled
	if status == "skipped" && includesAny(text, []string{"site disabled"}) {
		return FailureReason{
			Code: CodeSiteDisabled, Category: CategorySite,
			Title: "Site disabled", ActionHint: "Re-enable the site and retry",
			DetailHint: "The site of this account is disabled; the task will be skipped automatically.",
		}
	}

	// Priority 2: Checkin not supported
	if includesAny(text, []string{
		"checkin endpoint not found", "签到端点不存在", "站点不支持签到",
		"not support checkin", "check-in is not supported",
		"checkin is not supported", "does not support checkin",
	}) {
		return FailureReason{
			Code: CodeCheckinNotSupported, Category: CategorySite,
			Title: "Check-in not supported", ActionHint: "No retry needed (not a failure)",
			DetailHint: "This site does not provide a check-in endpoint; the account will be skipped automatically.",
		}
	}

	// Priority 3: Manual Turnstile required
	if includesAny(text, []string{"turnstile token 为空", "turnstile"}) &&
		includesAny(text, []string{"校验", "token", "验证", "manual"}) {
		return FailureReason{
			Code: CodeManualTurnstileRequired, Category: CategoryVerification,
			Title: "Manual verification required", ActionHint: "Sign in manually in a browser once",
			DetailHint: "The site requires Turnstile verification; automated check-in cannot pass directly.",
		}
	}

	// Priority 4: Cloudflare tunnel unavailable
	if includesAny(text, []string{"cloudflare tunnel error", "error 1033", "unable to resolve it"}) {
		return FailureReason{
			Code: CodeCloudflareTunnelUnavail, Category: CategoryNetwork,
			Title: "Site tunnel unavailable", ActionHint: "Retry later or contact the site operator",
			DetailHint: "The Cloudflare Tunnel is currently unreachable, usually due to site-side network or tunnel process issues.",
		}
	}

	// Priority 5: Cloudflare challenge
	if alert.IsCloudflareChallenge(rawMessage) {
		return FailureReason{
			Code: CodeCloudflareChallenge, Category: CategoryVerification,
			Title: "Cloudflare challenge triggered", ActionHint: "Slow down and retry later",
			DetailHint: "The request triggered a protection challenge; retry later or switch to a stable site.",
		}
	}

	// Priority 6: Token expired
	var tokStatus int
	if httpStatus > 0 {
		tokStatus = httpStatus
	}
	if alert.IsTokenExpiredError(tokStatus, rawMessage) {
		return FailureReason{
			Code: CodeTokenExpired, Category: CategoryAuth,
			Title: "Token expired", ActionHint: "Re-login or refresh the token",
			DetailHint: "The account access token may be expired or invalid; update the credentials.",
		}
	}

	// Priority 7: Already checked in
	if includesAny(text, []string{"already checked in", "already signed", "今天已经签到", "今日已签到", "已经签到"}) {
		return FailureReason{
			Code: CodeAlreadyCheckedIn, Category: CategoryState,
			Title: "Already checked in today", ActionHint: "Nothing to do",
			DetailHint: "This account has already checked in today; repeated requests will be rejected or skipped.",
		}
	}

	// Priority 8: Network timeout
	if includesAny(text, []string{"timeout", "timed out", "etimedout", "请求超时"}) {
		return FailureReason{
			Code: CodeNetworkTimeout, Category: CategoryNetwork,
			Title: "Request timed out", ActionHint: "Retry later and check the network",
			DetailHint: "The request did not finish within the timeout; possible network fluctuation or slow site response.",
		}
	}

	// Priority 9: Upstream error
	if httpStatus >= 500 || includesAny(text, []string{"http 5", "upstream", "internal server error"}) {
		return FailureReason{
			Code: CodeUpstreamError, Category: CategorySite,
			Title: "Upstream site error", ActionHint: "Retry later",
			DetailHint: "The site returned a server-side error; it usually succeeds only after the site recovers.",
		}
	}

	// Priority 10: Unknown
	if status == "success" {
		return FailureReason{
			Code: CodeUnknownError, Category: CategoryUnknown,
			Title: "Success", ActionHint: "No action needed",
			DetailHint: "The task completed successfully.",
		}
	}
	return FailureReason{
		Code: CodeUnknownError, Category: CategoryUnknown,
		Title: "Unknown error", ActionHint: "Check detailed logs and retry",
		DetailHint: "No specific error type recognized; investigate using the raw message.",
	}
}

func includesAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
