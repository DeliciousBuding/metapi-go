// metapi-go/features/proxy-logs — barrel re-exports.
//
// The page component is the primary surface; the rest is exported for the
// future `/proxy-logs` route file (validateSearch schema + types) and for
// cross-feature deep linking (the latency/status badges are reusable
// presentational components for any feature that renders a proxy log row).

export {
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from './lib/proxy-logs-schema'

export { proxyLogsKeys } from './types'
