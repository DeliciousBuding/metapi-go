# BLOCKED

无

## 决策记录（无需裁决，仅留档）

- **全价兜底档位语义**：`usage.Found == true` 且 `CacheReadTokens == 0 && CacheCreationTokens == 0`
  时进入新档位 `CalculateModelUsageFullPrice`——全部输入 token × 输入单价 + 输出 token ×
  输出单价，不做缓存减免。语义对齐 octopus `internal/relay/metrics.go:53-57`
  （缓存明细缺失 → nonCachedTokens = promptTokens 全价），落在 fallback_token_cost
  （除数额）与三段分价（ratio 公式）之间。
- **单价复用**：新档位经 `CacheAwarePerMillionRates` 取
  `inputPerMillion = modelRatio × 2 × multiplier` / `outputPerMillion = modelRatio ×
  completionRatio × 2 × multiplier`，未硬编新公式；与既有 breakdown 在无缓存 token 时
  数值一致（不回归），有真实 ratio 时区别于除数额。
- **pricing.source 标记**：新档位记 `full_price_fallback`，且不输出 `cache_ratio` /
  `cache_creation_ratio` 键（无缓存明细，不打折、不虚构）。三段分价仍记
  `model_ratio_defaults`；无 token 仍记 `fallback_token_cost`。
- **仅补「缺失」档，不移植 oversized-cache 守卫**：octopus 的
  `nonCachedTokens < 0 → 全价` 负值守卫（缓存明细超过总输入时保守回退）不在本任务范围；
  三段分价对 oversized 缓存仍沿用现有 `billable < 0 → 0` 钳制（不回归要求）。如需对齐
  可另开任务。
- **无 split 的 total-only 用量**：全价档将 total 全部按输入单价计（与
  `normalizeUsageBreakdownInput` 的 effectivePrompt 选择一致，保守）。
- **web/dist**：worktree 不含 gitignored 的 SPA 预构建产物，已从主工作区拷贝同源 dist
  （本 PR 仅改 Go 代码，前端内容与 master 一致）以便本地 `go build`/pre-push 通过。
