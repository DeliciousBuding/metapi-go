// metapi-go/features/proxy-logs — barrel re-exports.
//
// The page component is the primary surface; the rest is exported for the
// future `/proxy-logs` route file (validateSearch schema + types) and for
// cross-feature deep linking (the latency/status badges are reusable
// presentational components for any feature that renders a proxy log row).

export { ProxyLogsPage } from './components/proxy-logs-page'
export { ProxyLogDetailSheet } from './components/proxy-log-detail-sheet'
export { useProxyLogsColumns } from './components/proxy-logs-columns'
export { LatencyBadge, type LatencyBadgeProps } from './components/latency-badge'
export { StatusBadge, type StatusBadgeProps } from './components/status-badge'

export {
  useProxyLogs,
  useProxyLog,
  useProxyLogsMeta,
} from './api'

export {
  proxyLogsSearchSchema,
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  SORTING_ITEM_SCHEMA,
  type ProxyLogsSearch,
} from './lib/proxy-logs-schema'

export {
  proxyLogsKeys,
  type ProxyLog,
  type ProxyLogDetail,
  type ProxyLogFilters,
  type ProxyLogBillingDetails,
  type ProxyLogClientConfidence,
  type ProxyLogClientOption,
  type ProxyLogStatusFilter,
  type ProxyLogUsageSource,
  type ProxyLogsQuery,
  type ProxyLogsResponse,
  type ProxyLogsSummary,
} from './types'
