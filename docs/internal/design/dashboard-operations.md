# Operator overview

The overview serves an operator managing multiple upstreams while downstream
clients are actively making requests. Its order follows the decisions that
operator needs to make, rather than the order in which features were built.

## Questions and hierarchy

1. **Are downstream calls succeeding?** Show recorded request volume, success
   rate with its failure count, effective tokens, and estimated call cost over
   the same selected reporting window. A zero-request window has no success rate.
2. **Where should I look?** Put the selected request/reliability trend beside
   actionable account, balance and runtime notifications. No notifications is
   not proof that the request path is healthy.
3. **Which upstreams and models carry the impact?** Rank enabled upstreams by
   failures, then volume; retain both the count and rate. Show the model cost
   distribution over that same selected window. Every drill-down must use
   a real route and preserve the scope of the number the operator clicked.
4. **What resources and maintenance support this?** Account/site counts,
   active-site balance, check-in and scheduled jobs belong below the operating
   signals. An account-count card must not use an unrelated balance sparkline.

## Metric contracts

One `GET /api/stats/overview?period=24h|7d|30d|all` response owns the selected reporting window, summary, trend, model ranking and enabled-upstream performance. The UI defaults to the last seven days. Every analytical panel observes the same query key, and every diagnostic link preserves the returned `window.from` and `window.to`. All time means all retained request logs, not deleted history.

The summary, model ranking and trend include unassigned records and records from disabled sites. The upstream table explicitly includes enabled sites only. Empty traffic has no success rate. Missing buckets are zero requests and a gap in the success-rate line. The 24-hour range uses UTC hour buckets; other ranges use UTC days. The current first/last bucket may be partial.

The model ranking preserves an Other bucket. Costs are estimates, not revenue or profit. Aggregate data comes from grouped database queries, not a sample of recent requests. A query failure remains visible and is not rendered as a healthy zero. Existing dashboard/gallery endpoint contracts are unchanged.

Resource counts and balances use the current dashboard snapshot. Notices and scheduler health remain current-state information, visibly separate from historical analytics. Balances are not divided by usage to imply a shared wallet or runway. No consumer leaderboard is shown until a complete downstream-key aggregate exists.

## Ownership

`overview-section.tsx` owns the time-range controls and page composition. `use-overview-report.ts` shares one analytical query across the metric, trend, upstream and model panels. `overview-trend.ts` fills missing buckets. Maintenance and resource state keep their existing queries because their scope is current state, not the selected log interval.

The visual hierarchy follows a grouped KPI strip, a request-trend/model-use row, then upstream performance/current attention. Model identifiers use the shared aligned icon/text identity. Desktop density comes from grouping and alignment, not compressed glyphs or smaller body text.

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
