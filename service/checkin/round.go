package checkin

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/jmoiron/sqlx"
)

// CheckinAllResult is a result entry for CheckinAll.
type CheckinAllResult struct {
	AccountID int64
	Username  string
	Site      string
	Result    CheckinResult
}

// CheckinAll performs checkin for all eligible accounts.
// Mirrors TS checkinAll(). Per-account failure notifications are suppressed
// here (SkipNotification) and aggregated into ONE round notification sent at
// the end (issue #667): a 50-account failure burst now yields a single
// "Checkin round <id>: N ok, M failed" alert instead of 50 separate ones.
func CheckinAll(cfg *config.Config, db *sqlx.DB, accountIDs []int64, scheduleMode string) []CheckinAllResult {
	return CheckinAllContext(context.Background(), cfg, db, accountIDs, scheduleMode)
}

// CheckinAllContext is CheckinAll with a caller-supplied context. The same-site
// pacing wait between two accounts of one site is the only place a round blocks
// without an upstream in flight, so it is where a cancellation has to land: a
// finished ctx stops the round there instead of paying the pacing delay plus the
// next upstream call for every remaining account. CheckinAll keeps its signature
// (and its context.Background() behaviour) for the callers that have no ctx to
// hand yet.
//
// A per-account checkin still derives its own budget inside CheckinAccount,
// which has no ctx-taking variant: cancelling the round therefore stops the next
// account, while the in-flight one finishes inside its own checkinTimeout.
func CheckinAllContext(ctx context.Context, cfg *config.Config, db *sqlx.DB, accountIDs []int64, scheduleMode string) []CheckinAllResult {
	query := `SELECT a.id AS "accounts.id", a.site_id AS "accounts.site_id", a.username AS "accounts.username",
		a.access_token AS "accounts.access_token", a.balance AS "accounts.balance",
		a.balance_used AS "accounts.balance_used", a.quota AS "accounts.quota",
		a.status AS "accounts.status", a.checkin_enabled AS "accounts.checkin_enabled",
		a.last_checkin_at AS "accounts.last_checkin_at", a.extra_config AS "accounts.extra_config",
		s.id AS "sites.id", s.name AS "sites.name", s.url AS "sites.url",
		s.platform AS "sites.platform", s.status AS "sites.status"
		FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		WHERE a.checkin_enabled = TRUE AND a.status = 'active'`

	var rows []struct {
		Accounts struct {
			ID             int64   `db:"id"`
			SiteID         int64   `db:"site_id"`
			Username       *string `db:"username"`
			AccessToken    string  `db:"access_token"`
			Balance        float64 `db:"balance"`
			BalanceUsed    float64 `db:"balance_used"`
			Quota          float64 `db:"quota"`
			Status         string  `db:"status"`
			CheckinEnabled bool    `db:"checkin_enabled"`
			LastCheckinAt  *string `db:"last_checkin_at"`
			ExtraConfig    *string `db:"extra_config"`
		} `db:"accounts"`
		Sites struct {
			ID       int64  `db:"id"`
			Name     string `db:"name"`
			URL      string `db:"url"`
			Platform string `db:"platform"`
			Status   string `db:"status"`
		} `db:"sites"`
	}

	if err := db.Select(&rows, db.Rebind(query)); err != nil {
		slog.Error("CheckinAll: failed to query accounts", "error", err)
		return nil
	}

	// Filter by accountIDs if provided
	scopedIDs := make(map[int64]bool)
	if len(accountIDs) > 0 {
		for _, id := range accountIDs {
			scopedIDs[id] = true
		}
	}

	// Group by siteId
	grouped := make(map[int64][]int)
	for i, row := range rows {
		if len(scopedIDs) > 0 && !scopedIDs[row.Accounts.ID] {
			continue
		}
		siteID := row.Sites.ID
		grouped[siteID] = append(grouped[siteID], i)
	}

	var results []CheckinAllResult
	var mu sync.Mutex

	// Different sites: parallel. Same site: serial.
	var wg sync.WaitGroup
	for siteID, indices := range grouped {
		wg.Add(1)
		go func(siteID int64, indices []int) {
			defer wg.Done()
			for i, idx := range indices {
				if i > 0 {
					// Same-site pacing: space consecutive checkins on the same site
					// to avoid tripping upstream rate limits (429). Cross-site
					// parallelism is preserved by the per-site goroutine above.
					// The wait is cancellable so a finished round ctx stops the
					// site here instead of paying the delay plus the next
					// account's upstream call.
					if err := sleepCtx(ctx, sameSitePacingDelay()); err != nil {
						slog.Warn("CheckinAll: round context ended during same-site pacing; stopping this site",
							"siteID", siteID, "accountsLeft", len(indices)-i, "error", err)
						return
					}
				}
				row := rows[idx]
				result := CheckinAccount(cfg, db, row.Accounts.ID, &CheckinOptions{
					SkipEvent:        true,
					SkipNotification: true,
					ScheduleMode:     scheduleMode,
				})

				username := ""
				if row.Accounts.Username != nil {
					username = *row.Accounts.Username
				}

				mu.Lock()
				results = append(results, CheckinAllResult{
					AccountID: row.Accounts.ID,
					Username:  username,
					Site:      row.Sites.Name,
					Result:    result,
				})
				mu.Unlock()
			}
		}(siteID, indices)
	}
	wg.Wait()

	sendCheckinRoundNotification(cfg, results)

	return results
}

// sameSitePacingDelay returns a ~1s delay (with minor jitter) used between
// consecutive same-site checkins in CheckinAll to avoid upstream rate limits
// (issue #668: same-site pacing).
func sameSitePacingDelay() time.Duration {
	return time.Second + time.Duration(rand.IntN(250))*time.Millisecond
}

// notifySend is the notification dispatch function used by round aggregation.
// It defaults to notifypkg.SendNotification; tests swap it to assert call
// counts and message contents without touching real notification channels.
var notifySend = notifypkg.SendNotification

// checkinFailure captures a single failed account within a checkin round, for
// inclusion in the aggregated round notification.
type checkinFailure struct {
	accountID   int64
	accountName string
	site        string
	error       string
}

// checkinRoundResult is the aggregated outcome of one CheckinAll round: the
// round identifier, the collected failures, and the success count. It backs
// the single consolidated notification that replaces N per-account alerts.
type checkinRoundResult struct {
	roundID   string
	failures  []checkinFailure
	successes int
}

// accountLabel resolves a display name for a CheckinAllResult, falling back
// to an ID-based label when the username is empty (mirrors orUsername but
// works off the already-resolved string carried by CheckinAllResult).
func accountLabel(username string, id int64) string {
	if strings.TrimSpace(username) != "" {
		return username
	}
	return fmt.Sprintf("ID:%d", id)
}

// firstLine returns the first line of a message, trimmed of surrounding
// whitespace. Keeps the aggregated notification body compact when an adapter
// returns a multi-line error blob.
func firstLine(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		return strings.TrimSpace(message[:idx])
	}
	return strings.TrimSpace(message)
}

// buildCheckinRoundResult aggregates CheckinAll results into a round summary.
// Skipped accounts (unsupported endpoint / manual verification / disabled)
// are neither successes nor failures for notification purposes. Returns nil
// when there are no failures, matching the current behavior of NOT alerting
// on a fully-clean round (all success or all skipped).
func buildCheckinRoundResult(results []CheckinAllResult) *checkinRoundResult {
	var failures []checkinFailure
	successes := 0
	for _, r := range results {
		switch r.Result.Status {
		case CheckinFailed:
			failures = append(failures, checkinFailure{
				accountID:   r.AccountID,
				accountName: accountLabel(r.Username, r.AccountID),
				site:        r.Site,
				error:       firstLine(r.Result.Message),
			})
		case CheckinSuccess:
			successes++
		}
	}
	if len(failures) == 0 {
		return nil
	}
	// Deterministic ordering across goroutine scheduling order so the
	// notification body is stable and diffable.
	sort.SliceStable(failures, func(i, j int) bool {
		if failures[i].accountName != failures[j].accountName {
			return failures[i].accountName < failures[j].accountName
		}
		return failures[i].accountID < failures[j].accountID
	})
	return &checkinRoundResult{
		roundID:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		successes: successes,
		failures:  failures,
	}
}

// maxFailuresPerNotification caps how many failure lines are rendered in the
// body so a 50-account outage stays a readable summary, not a wall of text.
const maxFailuresPerNotification = 25

// notificationTitle renders the round notification title:
// "Checkin round <id>: <ok> ok, <failed> failed".
func (rr *checkinRoundResult) notificationTitle() string {
	return fmt.Sprintf("Checkin round %s: %d ok, %d failed", rr.roundID, rr.successes, len(rr.failures))
}

// notificationBody renders one "account @ site: error" line per failure,
// truncated to maxFailuresPerNotification entries with a tail count.
func (rr *checkinRoundResult) notificationBody() string {
	var builder strings.Builder
	for i, f := range rr.failures {
		if i > 0 {
			builder.WriteString("\n")
		}
		if i >= maxFailuresPerNotification {
			fmt.Fprintf(&builder, "…and %d more", len(rr.failures)-maxFailuresPerNotification)
			break
		}
		fmt.Fprintf(&builder, "%s @ %s: %s", f.accountName, f.site, f.error)
	}
	return builder.String()
}

// sendCheckinRoundNotification aggregates a CheckinAll round's results into a
// single failure notification. No-op on a clean round (no failures). This is
// the issue #667 fix: one failed round → one notification, not N.
func sendCheckinRoundNotification(cfg *config.Config, results []CheckinAllResult) {
	round := buildCheckinRoundResult(results)
	if round == nil {
		return
	}
	notifySend(config.RuntimeSafe(), round.notificationTitle(), round.notificationBody(), "error", nil)
}
