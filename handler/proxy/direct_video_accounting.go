package proxyhandler

import (
	"context"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

type directVideoAccounting struct {
	TaskID     string
	Observed   ParsedUsage
	Delta      ParsedUsage
	CostDelta  float64
	Legacy     bool
	Unverified bool
}
type directVideoAccountingOperationKey struct{}
type directVideoAccountingLogKey struct{}

func accountDirectVideoUsage(requestCtx context.Context, ctx *Ctx, selected *routing.SelectedChannel, observed ParsedUsage) (ParsedUsage, error) {
	if ctx == nil || selected == nil || selected.Direct == nil {
		return observed, nil
	}
	if ctx.videoAccountingError != nil {
		return observed, ctx.videoAccountingError
	}
	if ctx.videoAccounting != nil {
		return applyDirectVideoAccounting(ctx, observed), nil
	}
	task := ctx.videoAccountingTask
	if task == nil {
		task = ctx.videoTask
	}
	if task == nil || !ctx.videoUsageVerified {
		return observed, nil
	}
	keyID := int64(0)
	if ctx.Auth != nil && ctx.Auth.KeyID != nil {
		keyID = *ctx.Auth.KeyID
	}
	usage := store.VideoTaskUsage{Found: observed.Found, PromptTokens: observed.PromptTokens,
		CompletionTokens: observed.CompletionTokens, TotalTokens: observed.TotalTokens,
		CacheReadTokens: observed.CacheReadTokens, CacheCreationTokens: observed.CacheCreationTokens, ReasoningTokens: observed.ReasoningTokens}
	cost := EstimateBillingCostFromUsage(task.RequestedModel, selectedBillingPlatform(selected), observed).EstimatedCost
	// The upstream work has already happened. Client cancellation must not
	// cancel the short local ledger transaction and make consumption disappear.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), 5*time.Second)
	defer cancel()
	result, err := store.AccountVideoTask(writeCtx, store.GetDB(), task.PublicID, directVideoOwner(ctx.Auth), task.Identity.GrantID, keyID, usage, cost)
	if err != nil {
		ctx.videoAccountingError = fmt.Errorf("video task accounting is unavailable")
		if keyID > 0 && store.GetDB() != nil {
			// Match durable quota accounting's fail-closed policy. Do not let a
			// missing billable event increase the key's remaining budget.
			_, _ = store.GetDB().ExecContext(writeCtx, `UPDATE downstream_api_keys SET enabled=? WHERE id=?`, false, keyID)
		}
		return observed, ctx.videoAccountingError
	}
	delta := ParsedUsage{Found: result.Delta.Found, Source: observed.Source,
		PromptTokens: result.Delta.PromptTokens, CompletionTokens: result.Delta.CompletionTokens,
		TotalTokens: result.Delta.TotalTokens, CacheReadTokens: result.Delta.CacheReadTokens,
		CacheCreationTokens: result.Delta.CacheCreationTokens, ReasoningTokens: result.Delta.ReasoningTokens}
	ctx.videoAccounting = &directVideoAccounting{TaskID: task.PublicID, Observed: observed, Delta: delta, CostDelta: result.CostDelta, Legacy: result.Legacy}
	return applyDirectVideoAccounting(ctx, observed), nil
}

func applyDirectVideoAccounting(ctx *Ctx, observed ParsedUsage) ParsedUsage {
	if ctx == nil {
		return observed
	}
	if ctx.videoAccounting == nil {
		if ctx.videoTask == nil && ctx.videoAccountingTask == nil {
			return observed
		}
		task := ctx.videoTask
		if ctx.videoAccountingTask != nil {
			task = ctx.videoAccountingTask
		}
		return ParsedUsage{Source: usageSourceUnknown, videoAccounting: &directVideoAccounting{TaskID: task.PublicID, Observed: observed, Unverified: true}}
	}
	out := ctx.videoAccounting.Delta
	out.UpstreamReportedModel = observed.UpstreamReportedModel
	out.FirstOutputLatencyMs = observed.FirstOutputLatencyMs
	out.videoAccounting = ctx.videoAccounting
	return out
}

func directVideoBillingResult(model, platform string, usage ParsedUsage) BillingCostResult {
	billing := EstimateBillingCostFromUsage(model, platform, usage)
	if accounting := usage.videoAccounting; accounting != nil {
		billing.EstimatedCost = accounting.CostDelta
		observed := accounting.Observed
		billing.BillingDetails["observedUsage"] = map[string]any{
			"found": observed.Found, "source": observed.Source, "promptTokens": observed.PromptTokens,
			"completionTokens": observed.CompletionTokens, "totalTokens": observed.TotalTokens,
			"cacheReadTokens": observed.CacheReadTokens, "cacheCreationTokens": observed.CacheCreationTokens,
			"reasoningTokens": observed.ReasoningTokens, "upstreamReportedModel": observed.UpstreamReportedModel,
		}
		billing.BillingDetails["observedEstimatedCost"] = EstimateBillingCostFromUsage(model, platform, accounting.Observed).EstimatedCost
		billing.BillingDetails["accountedCost"] = accounting.CostDelta
		billing.BillingDetails["videoTaskId"] = accounting.TaskID
		billing.BillingDetails["accountingMode"] = "task_cumulative"
		if accounting.Legacy {
			billing.BillingDetails["accountingMode"] = "legacy_task_untracked"
		}
		if accounting.Unverified {
			billing.BillingDetails["accountingMode"] = "unverified_task_response"
		}
	}
	return billing
}

func directVideoLogContext(ctx context.Context, proxyCtx *Ctx) context.Context {
	// Owned task reads and already-accounted creates never enter the generic
	// per-HTTP-event quota sink, including failures with diagnostic usage.
	if proxyCtx != nil && (proxyCtx.videoTask != nil || proxyCtx.videoAccounting != nil || proxyCtx.videoAccountingTask != nil) {
		return context.WithValue(ctx, directVideoAccountingLogKey{}, true)
	}
	return ctx
}
