// metapi-go/features/observability — Zod schema for the workspace URL state.
//
// The Observability workspace is a single route (`/observability`) whose
// active section lives in the `section` search param (mirrors the proxy-logs
// / models search-schema contract). `section` is validated against the two
// registered sections and falls back to `overview` when absent/unknown —
// including stale `?section=proxy-logs` links, since proxy logs moved to
// the dedicated `/proxy-logs` workspace.

import { z } from 'zod'

export const observabilitySearchSchema = z.object({
  section: z.enum(['overview', 'health']).catch('overview').default('overview'),
})
