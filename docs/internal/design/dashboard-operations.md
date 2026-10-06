# Operator overview

The overview serves an operator managing multiple upstreams while downstream
clients are actively making requests. Its order follows the decisions that
operator needs to make, rather than the order in which features were built.

## Questions and hierarchy

1. **Are downstream calls succeeding?** Show recorded request volume, success
   rate with its failure count, effective tokens, and estimated call cost over
   the same rolling 24-hour window. A zero-request window has no success rate.
2. **Where should I look?** Put the seven-day request/reliability trend beside
   actionable account, balance and runtime notifications. No notifications is
   not proof that the request path is healthy.
3. **Which upstreams and models carry the impact?** Rank enabled upstreams by
   failures, then volume; retain both the count and rate. Show the model cost
   distribution as a separate seven-day aggregate. Every drill-down must use
   a real route and preserve the scope of the number the operator clicked.
4. **What resources and maintenance support this?** Account/site counts,
   active-site balance, check-in and scheduled jobs belong below the operating
   signals. An account-count card must not use an unrelated balance sparkline.

## Metric contracts

| Surface | Source | Scope and limits |
| --- | --- | --- |
| Request metrics | `getProxyLogsMeta({from,to})` | All recorded requests without a site/status filter, including unassigned failures and records from disabled sites. Freeze these exact bounds in the result for log links. |
| Request trend | `getLatencyTrend(7)` | Seven UTC calendar days, including today; daily volume and success rate. Distinct from the rolling 24-hour KPI window. |
| Upstream health | `getDashboardSnapshot({view:'insights'})` | Enabled upstreams only, last 24 hours. Anchor log links to `generatedAt`. Zero traffic and failed/missing aggregates are not healthy results. |
| Attention | `getAttention(6)` | Account, balance and runtime notices with entity links. It does not measure downstream availability. |
| Model usage | `getModelCostDistribution(7,5)` | Backend aggregate, not a sample of recent logs. Preserve the Other bucket and never search for it as a real model. |
| Resources | `getDashboardSnapshot()` | Configured counts and normalized balances; not request-health metrics. |

The legacy summary's `proxy24h` and `performance` fields filter by enabled
sites. They are not reused as a global downstream-request verdict. Existing
API semantics are preserved rather than silently changing old consumers.

Estimated cost is not revenue or profit. The overview does not divide pooled
upstream balances by usage to promise remaining days: route eligibility,
balance freshness and provider restrictions differ. There is no complete
downstream-key leaderboard here until an explicit aggregate API exists.
First-byte time, complete-request latency and model generation speed must not
be substituted for one another.

## Ownership

`overview-section.tsx` composes the operating surfaces and the compact resource
row. Components under its `components/` directory own their bounded query and
presentation. `maintenance-panel.tsx` owns scheduled-job controls and history;
it does not decide service health. Existing attention-label localization and
target resolution are reused.

## Reference implementation review

Official source was reviewed on 2026-10-06, pinned to these revisions:

- [AxonHub dashboard](https://github.com/looplj/axonhub/blob/5c78997798b40e44788c38bd6720c0b9c156f26b/frontend/src/features/dashboard/index.tsx)
  places request metrics above a request-trend/channel-success split and
  progressively discloses deeper analytics. Its channel table retains success
  and failure counts beside percentages. Borrow that decision structure, not
  arbitrary success thresholds or cumulative/today metric mixing.
- [Octopus overview at the reviewed revision](https://github.com/bestruirui/octopus/tree/0538c3e715e529e3a1f3a2b1229addb8ca4357ea)
  uses trend and ranking components that
  use compact period summaries and cost/request/token dimensions. Its
  year-long activity display is better suited to an analysis view than this
  overview's incident-oriented first screen.
- [New API performance overview](https://github.com/QuantumNous/new-api/blob/973cf8ef4600947a4270e95ada7916740fa8264c/web/src/features/dashboard/components/overview/performance-health-panel.tsx)
  and [flow analysis](https://github.com/QuantumNous/new-api/blob/973cf8ef4600947a4270e95ada7916740fa8264c/web/src/features/dashboard/components/flow/flow-charts.tsx)
  connect service quality, model use and upstream/downstream relationships.
  Its unified user wallet and runway assumptions do not transfer directly to
  independent upstream balances in Metapi.

These references motivate the hierarchy; the data contracts above remain the
authority for what Metapi can actually show.
