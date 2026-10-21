package auth

import (
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Key-context admission explanation (route-decision C batch).
//
// ManagedKeyContext is a read-only view of one managed downstream key for
// admin explanation surfaces. Loading and explaining never consume quota,
// increment rate windows, lease credentials, write decision snapshots, or
// call upstreams. The view never carries the key token itself.

// ManagedKeyAdmissionBlocker is one currently-blocking admission dimension.
// Code matches the live rejection reason of the real chain (snake_case), so a
// support answer can be traced to the exact production verdict.
type ManagedKeyAdmissionBlocker struct {
	Dimension string `json:"dimension"` // policy|state|expiry|budget|quota|ip|rate
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// ManagedKeyWindowUsage is one used/max pair; Max nil means unlimited.
type ManagedKeyWindowUsage struct {
	Used float64  `json:"used"`
	Max  *float64 `json:"max"`
}

// ManagedKeyIPCheck reports the IP dimension. Checked=false means no client IP
// was supplied, which is not the same as "allowed".
type ManagedKeyIPCheck struct {
	Checked bool   `json:"checked"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// ManagedKeyAdmissionExplanation is the full read-only admission answer for
// one key: every dimension of the real chain evaluated, all current blockers
// listed (the live chain stops at the first), plus the window numbers behind
// each budget/quota/rate verdict.
type ManagedKeyAdmissionExplanation struct {
	Verdict  string                       `json:"verdict"` // allow|deny
	Blockers []ManagedKeyAdmissionBlocker `json:"blockers"`
	Notes    []string                     `json:"notes"`
	IP       ManagedKeyIPCheck            `json:"ip"`
	// Windows are snapshots at evaluation time; they can change on the next
	// real request. The rate window is process-local (see Notes).
	Cost     *ManagedKeyWindowUsage      `json:"cost,omitempty"`
	Requests *ManagedKeyWindowUsage      `json:"requests,omitempty"`
	Rate     *ManagedKeyRateWindows      `json:"rate,omitempty"`
	Quota    *ManagedKeyQuotaExplanation `json:"quota,omitempty"`
}

// ManagedKeyRateWindows carries the sliding-window RPM/TPM snapshot.
type ManagedKeyRateWindows struct {
	RPM   ManagedKeyWindowUsage `json:"rpm"`
	TPM   ManagedKeyWindowUsage `json:"tpm"`
	Scope string                `json:"scope"` // always "processLocal" today
}

// ManagedKeyContext is the read-only managed-key view plus its resolved
// routing policy (the same derivation the proxy path uses).
type ManagedKeyContext struct {
	ID           int64
	Name         string
	Enabled      bool
	ExpiresAt    *string
	MaxCost      *float64
	UsedCost     float64
	MaxRequests  *int64
	UsedRequests int64
	MaxRPM       *int64
	MaxTPM       *int64
	// AccessPolicyBlocked mirrors the live chain's first gate: invalid
	// persisted JSON or an explicit blockReason. The two stay distinguishable.
	AccessPolicyInvalid bool
	AccessPolicyBlocked bool
	HasQuotaPolicy      bool
	Policy              DownstreamRoutingPolicy
	userAccess          DownstreamTokenAuthResult

	view *managedKeyView
	db   *sqlx.DB
}

// LoadManagedKeyContextByID returns the read-only context for one managed key,
// or (nil, nil) when the ID does not exist. The explicit handle keeps the
// read on the caller's database (tests scope their own; production passes the
// shared one).
func LoadManagedKeyContextByID(db *sqlx.DB, id int64) (*ManagedKeyContext, error) {
	view, err := loadManagedKeyFrom(db, "id", id)
	if err != nil || view == nil {
		return nil, err
	}
	userAccess := resolveManagedKeyUser(view, db)
	if !userAccess.OK {
		empty := []int64{}
		view.UserRouteIDs = &empty
	}
	ctx := &ManagedKeyContext{
		userAccess:   userAccess,
		ID:           view.ID,
		Name:         view.Name,
		Enabled:      view.Enabled,
		ExpiresAt:    view.ExpiresAt,
		MaxCost:      view.MaxCost,
		UsedCost:     view.UsedCost,
		MaxRequests:  view.MaxRequests,
		UsedRequests: view.UsedRequests,
		MaxRPM:       view.MaxRPM,
		MaxTPM:       view.MaxTPM,
		Policy:       toPolicyFromView(view),
		view:         view,
		db:           db,
	}
	ctx.AccessPolicyInvalid = view.InvalidAccessPolicy
	ctx.AccessPolicyBlocked = view.AccessPolicy != nil && view.AccessPolicy.BlockReason != ""
	ctx.HasQuotaPolicy = view.AccessPolicy != nil && view.AccessPolicy.Quota != nil
	return ctx, nil
}

// ExplainAdmission evaluates every admission dimension in the real chain's
// order and lists all current blockers. It is read-only by construction: no
// counter increment, no quota write, no lease, no upstream call.
func (c *ManagedKeyContext) ExplainAdmission(clientIP string) ManagedKeyAdmissionExplanation {
	now := time.Now()
	blockers := []ManagedKeyAdmissionBlocker{}
	notes := []string{"evaluationIsReadOnly"}

	add := func(dimension, code, message string) {
		blockers = append(blockers, ManagedKeyAdmissionBlocker{
			Dimension: dimension, Code: code, Message: message,
		})
	}

	// Mirror authorizeDownstreamToken's order.
	if !c.userAccess.OK && c.userAccess.Reason != "" {
		add("user", c.userAccess.Reason, c.userAccess.Error)
	}
	if c.AccessPolicyInvalid {
		add("policy", "access_policy", "API key access policy is blocked (unreadable stored policy; fails closed)")
	} else if c.AccessPolicyBlocked {
		add("policy", "access_policy", "API key access policy is blocked: "+c.view.AccessPolicy.BlockReason)
	}
	if !c.Enabled {
		add("state", "disabled", "API key is disabled")
	}
	if c.ExpiresAt != nil {
		if expiresAt, parseErr := parseISO8601(*c.ExpiresAt); parseErr == nil && !expiresAt.After(now) {
			add("expiry", "expired", "API key is expired ("+*c.ExpiresAt+")")
		}
	}

	var costWindow, requestsWindow *ManagedKeyWindowUsage
	if c.MaxCost != nil {
		max := *c.MaxCost
		costWindow = &ManagedKeyWindowUsage{Used: c.UsedCost, Max: &max}
		if c.UsedCost >= max {
			add("budget", "over_cost", "API key has exceeded max cost")
		}
	}
	if c.MaxRequests != nil {
		max := float64(*c.MaxRequests)
		requestsWindow = &ManagedKeyWindowUsage{Used: float64(c.UsedRequests), Max: &max}
		if c.UsedRequests >= *c.MaxRequests {
			add("budget", "over_requests", "API key has exceeded max requests")
		}
	}

	var quotaExplanation *ManagedKeyQuotaExplanation
	if c.HasQuotaPolicy {
		explained := ExplainManagedKeyQuota(c.db, c.ID, c.view.AccessPolicy.Quota)
		quotaExplanation = &explained
		if explained.Blocker != "" {
			add("quota", explained.Blocker, explained.Message)
		}
		if explained.HistoryMissing {
			notes = append(notes, "quotaHistoryIncomplete")
		}
	}

	ipCheck := ManagedKeyIPCheck{Checked: false}
	if strings.TrimSpace(clientIP) != "" {
		allowed, reason := CheckDownstreamKeyIP(c.view, clientIP)
		ipCheck = ManagedKeyIPCheck{Checked: true, Allowed: allowed, Reason: reason}
		if !allowed {
			add("ip", reason, "client IP is not permitted for this key")
		}
	} else if c.hasIPRules() {
		notes = append(notes, "ipRulesExistButUnchecked")
	}

	var rateWindows *ManagedKeyRateWindows
	if c.MaxRPM != nil || c.MaxTPM != nil {
		usedRPM, usedTPM := GlobalKeyAdmission.Snapshot(c.ID)
		rpm := ManagedKeyWindowUsage{Used: float64(usedRPM)}
		tpm := ManagedKeyWindowUsage{Used: float64(usedTPM)}
		if c.MaxRPM != nil {
			max := float64(*c.MaxRPM)
			rpm.Max = &max
			if usedRPM >= *c.MaxRPM {
				add("rate", "over_rpm", "API key RPM window is currently full")
			}
		}
		if c.MaxTPM != nil {
			max := float64(*c.MaxTPM)
			tpm.Max = &max
			if usedTPM >= *c.MaxTPM {
				add("rate", "over_tpm", "API key TPM window is currently full")
			}
		}
		rateWindows = &ManagedKeyRateWindows{RPM: rpm, TPM: tpm, Scope: "processLocal"}
		notes = append(notes, "rateWindowProcessLocal")
	}

	verdict := "allow"
	if len(blockers) > 0 {
		verdict = "deny"
	}
	return ManagedKeyAdmissionExplanation{
		Verdict:  verdict,
		Blockers: blockers,
		Notes:    notes,
		IP:       ipCheck,
		Cost:     costWindow,
		Requests: requestsWindow,
		Rate:     rateWindows,
		Quota:    quotaExplanation,
	}
}

// hasIPRules reports whether the key carries any IP allow/block rule.
func (c *ManagedKeyContext) hasIPRules() bool {
	return len(splitIPList(c.view.IPAllowlist)) > 0 || len(splitIPList(c.view.IPBlocklist)) > 0
}
