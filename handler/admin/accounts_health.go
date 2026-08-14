package admin

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/service"
	balanceService "github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/jmoiron/sqlx"
)

func (h *accountsHandler) healthRefresh(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountHealthRefreshPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid health refresh payload.")
		return
	}

	wait := body.Wait != nil && *body.Wait

	var accountIDs []int64
	if body.AccountID != nil {
		if *body.AccountID <= 0 {
			writeError(w, http.StatusBadRequest, "Invalid accountId. Expected positive number.")
			return
		}
		accountID := int64(*body.AccountID)
		if _, err := service.GetAccountWithSiteByID(h.db, accountID); err != nil {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		accountIDs = []int64{accountID}
	} else {
		ids, err := listAccountIDsForHealthRefresh(h.db)
		if err != nil {
			slog.Error("Failed to list accounts for health refresh", "err", err)
			writeError(w, http.StatusInternalServerError, "Failed to list accounts")
			return
		}
		accountIDs = ids
	}

	if wait {
		summary, results := h.runAccountHealthRefresh(accountIDs)
		globalAccountsCache.clear()
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"summary": summary,
			"results": results,
			"message": formatHealthRefreshMessage(summary),
		})
		return
	}

	// Async path: in-process task registry (no durable multi-instance job store).
	title := "刷新账号运行健康状态"
	dedupeKey := "refresh-all-account-runtime-health"
	if body.AccountID != nil {
		title = fmt.Sprintf("刷新账号 #%d 运行健康状态", *body.AccountID)
		dedupeKey = fmt.Sprintf("refresh-account-runtime-health-%d", *body.AccountID)
	}

	ids := append([]int64(nil), accountIDs...)
	db := h.db
	cfg := h.cfg
	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      "account-runtime-health-refresh",
		Title:     title,
		DedupeKey: dedupeKey,
	}, func() (any, error) {
		handler := &accountsHandler{db: db, cfg: cfg}
		summary, results := handler.runAccountHealthRefresh(ids)
		globalAccountsCache.clear()
		return map[string]any{
			"summary": summary,
			"results": results,
		}, nil
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  reused,
		"jobId":   task.ID,
		"taskId":  task.ID,
		"status":  string(task.Status),
		"message": "已开始刷新账号运行健康状态，请稍后查看账号列表",
	})
}

func listAccountIDsForHealthRefresh(db *sqlx.DB) ([]int64, error) {
	var ids []int64
	if err := db.Select(&ids, "SELECT id FROM accounts ORDER BY id"); err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, nil
}

func (h *accountsHandler) runAccountHealthRefresh(accountIDs []int64) (healthRefreshSummary, []healthRefreshResultItem) {
	results := make([]healthRefreshResultItem, 0, len(accountIDs))
	summary := healthRefreshSummary{}

	for _, accountID := range accountIDs {
		item := h.refreshOneAccountHealth(accountID)
		results = append(results, item)
		summary.Total++
		switch item.Status {
		case "success":
			summary.Success++
		case "failed":
			summary.Failed++
		default:
			summary.Skipped++
		}
		switch service.RuntimeHealthState(item.State) {
		case service.HealthHealthy:
			summary.Healthy++
		case service.HealthUnhealthy:
			summary.Unhealthy++
		case service.HealthDegraded:
			summary.Degraded++
		case service.HealthDisabled:
			summary.Disabled++
		default:
			summary.Unknown++
		}
	}

	if results == nil {
		results = []healthRefreshResultItem{}
	}
	return summary, results
}

func (h *accountsHandler) refreshOneAccountHealth(accountID int64) healthRefreshResultItem {
	row, err := service.GetAccountWithSiteByID(h.db, accountID)
	if err != nil {
		return healthRefreshResultItem{
			AccountID: accountID,
			Status:    "failed",
			State:     string(service.HealthUnknown),
			Reason:    "account not found",
			Message:   "account not found",
		}
	}

	caps := service.BuildCapabilitiesForAccount(&row.Account)
	sessionCapable := caps.CanRefreshBalance

	// proxy-only / OAuth / pure API-key: no balance probe path — honest skip.
	if caps.ProxyOnly || !caps.CanRefreshBalance || service.IsAPIKeyConnection(&row.Account) {
		health := service.BuildRuntimeHealthForAccount(service.RuntimeHealthInput{
			AccountStatus:  row.Account.Status,
			SiteStatus:     row.Site.Status,
			ExtraConfig:    row.Account.ExtraConfig,
			SessionCapable: &sessionCapable,
			OAuthProvider:  row.Account.OAuthProvider,
		})
		return healthRefreshResultItem{
			AccountID: accountID,
			Status:    "skipped",
			State:     string(health.State),
			Reason:    "proxy_only",
			Message:   "proxy-only account skipped runtime balance probe",
			ProxyOnly: true,
		}
	}

	result, refreshErr := balanceService.RefreshBalance(h.cfg, h.db, accountID)

	// Re-read after refresh so state reflects persisted runtimeHealth / status.
	if updated, loadErr := service.GetAccountWithSiteByID(h.db, accountID); loadErr == nil {
		row = updated
	}
	sessionCapable = service.BuildCapabilitiesForAccount(&row.Account).CanRefreshBalance
	health := service.BuildRuntimeHealthForAccount(service.RuntimeHealthInput{
		AccountStatus:  row.Account.Status,
		SiteStatus:     row.Site.Status,
		ExtraConfig:    row.Account.ExtraConfig,
		SessionCapable: &sessionCapable,
		OAuthProvider:  row.Account.OAuthProvider,
	})

	if result == nil && refreshErr == nil {
		return healthRefreshResultItem{
			AccountID: accountID,
			Status:    "failed",
			State:     string(service.HealthUnknown),
			Reason:    "account not found or platform not supported",
			Message:   "account not found or platform not supported",
		}
	}
	if refreshErr != nil {
		state := string(health.State)
		if state == "" || state == string(service.HealthUnknown) {
			state = string(service.HealthUnhealthy)
		}
		reason := health.Reason
		if reason == "" {
			reason = refreshErr.Error()
		}
		return healthRefreshResultItem{
			AccountID: accountID,
			Status:    "failed",
			State:     state,
			Reason:    reason,
			Message:   refreshErr.Error(),
		}
	}
	if result.Skipped {
		state := string(health.State)
		if state == "" {
			state = string(service.HealthUnknown)
		}
		reason := result.Reason
		if reason == "" {
			reason = health.Reason
		}
		return healthRefreshResultItem{
			AccountID: accountID,
			Status:    "skipped",
			State:     state,
			Reason:    reason,
			Message:   reason,
			ProxyOnly: reason == "proxy_only",
		}
	}

	state := string(health.State)
	if state == "" {
		state = string(service.HealthHealthy)
	}
	reason := health.Reason
	if reason == "" {
		reason = "余额刷新成功"
	}
	return healthRefreshResultItem{
		AccountID: accountID,
		Status:    "success",
		State:     state,
		Reason:    reason,
		Message:   reason,
	}
}

func formatHealthRefreshMessage(summary healthRefreshSummary) string {
	return fmt.Sprintf(
		"账号健康刷新完成：成功 %d，失败 %d，跳过 %d（共 %d）",
		summary.Success, summary.Failed, summary.Skipped, summary.Total,
	)
}

// ---- Refresh Balance ----
