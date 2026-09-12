// metapi-go/features/proxy-logs — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
//
// The `/proxy-logs` route reads the validateSearch schema and its type from
// here and loads `ProxyLogsPage` directly; the observability drill-in registry
// reads the query keys. `ProxyLogsPage` is deliberately not exported here. The
// row badges stay inside the feature — nothing outside it renders a proxy log
// row, and the one badge two features did need (HTTP status) lives in
// `@/components/common/http-status-badge`.

export {
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from './lib/proxy-logs-schema'

export { proxyLogsKeys } from './types'
