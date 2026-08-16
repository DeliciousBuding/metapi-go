# BLOCKED

无

## 决策记录（无需裁决，仅留档）——半开探测（half-open probing）

任务：给站点/模型运行时熔断器加「半开探测」，对标 octopus 三态熔断
（`internal/relay/balancer/circuit.go`）。此前冷却到期后全量放行；现在到期后
仅放行单个恢复探针，探针成功才完全放行。

### 三态语义

| 状态 | 判定 | 准入行为 |
|------|------|----------|
| closed（关闭） | `BreakerUntilMs == nil` 或已清零 | 正常放行 |
| open（熔断） | `BreakerUntilMs > now` | 拒绝（对应 octopus `StateOpen`） |
| half-open（半开） | 冷却已到期 + `HalfOpenSinceMs != nil`（探针在途） | 拒绝其余请求（对应 octopus `StateHalfOpen`） |
| expired（到期待探针） | 冷却已到期 + `HalfOpenSinceMs == nil` | 本次请求成为恢复探针：写入 `HalfOpenSinceMs` 并放行 |

- 每个阻塞状态（global / model）同一时刻只放行 **1 个探针**；其余请求在探针
  结果落定前一律拒绝（`TryAdmitSiteModelRuntimeRequest` 在 `finalizeDispatch`
  处做最终门禁，软过滤层只负责避让，不承担探针计数）。
- 状态同时按 **站点（global）与模型（model）** 两级维护：任一级 open/half-open
  即拒绝；两级均 expired 时同一请求同时授予两级探针。

### 探针结果转移

- **成功**（`applyRuntimeHealthSuccess`）：清 `HalfOpenSinceMs`、`BreakerLevel=0`、
  `BreakerUntilMs=nil` → closed，全量恢复放行。
- **瞬态失败**（上游 5xx/超时等，`applyRuntimeHealthFailure` + `IsTransientSiteRuntimeFailure`）：
  清 `HalfOpenSinceMs`，`BreakerLevel+1`（封顶），按新等级重新计冷却 → open
  （扩展退避，对应 octopus `TripCount++` 冷却翻倍）。
- **非瞬态失败**（客户端 4xx 等）：清 `HalfOpenSinceMs`，不升级、不重开
  （客户端错误证明不了上游健康）→ 回到 expired，下一个请求重新试探。
- **探针超时**：`SiteRuntimeHalfOpenProbeTimeoutMs = 10min`，若探针结果始终未落定
  （进程重启/请求丢失），准入门禁在写路径惰性释放过期 `HalfOpenSinceMs`，
  回到 expired 由下一请求重试，避免站点被永久隔离。上游 HTTP 超时远低于此界。

### 有意的行为变更：open 期间不再 fail-open

此前「空过滤全量回退」（单候选短路 + 全坏全量池）会在所有候选都熔断时照常派出
请求（防饿死）。本次对准 octopus 三态后，**dispatch 门禁对 hard-open 一律拒绝**：
open 期间零流量，直到冷却到期由半开探针恢复。软过滤层的全量池回退仍保留
（仍能选出候选池），但 `finalizeDispatch` 的门禁会否决 hard-open 派发。
这是刻意的三态语义收敛，路由层 `SelectChannel` 会返回 nil（上游报
"No available channels"）。

### 其余

- **持久化**：`HalfOpenSinceMs` 纳入 state 持久化（`shouldPersist` 判定 +
  `cloneSiteRuntimeHealthState` 深拷贝 + JSON 字段 `halfOpenSinceMs`），
  重启后探针窗口不丢失（10min 超时兜底）。
- **展示**：`GetSiteRuntimeHealthDetails` 暴露 `GlobalHalfOpen` / `ModelHalfOpen`；
  `ChannelRuntimeStatus` 与软过滤把 half-open 视同 breaker-open 避让
  （避让文案「站点/模型半开探测中，优先避让」）；half-open 期间 multiplier
  压到 `SiteRuntimeMinMultiplier`。
- **测试**：`routing/half_open_breaker_test.go` 新增 11 个用例（单探针授予、
  成功复位、瞬态失败扩展退避、非瞬态失败重试、超时释放、模型级授予、
  hard-open 拒绝、multiplier 压底、过滤避让、持久化往返、dispatch 集成）。
- **`web/dist`**：worktree 含 gitignored 的 SPA 预构建产物（同源拷贝），
  本 PR 仅改 Go 代码。
