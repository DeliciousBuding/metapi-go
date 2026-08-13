// metapi-go/features/observability — Zod schema for the workspace URL state.
//
// The Observability workspace is a single route (`/observability`) whose
// active section lives in the `section` search param (mirrors the proxy-logs
// / models search-schema contract). `section` is validated against the three
// registered sections and falls back to `overview` when absent/unknown.

import { z } from 'zod'

export const observabilitySearchSchema = z.object({
  section: z.enum(['overview', 'health', 'proxy-logs']).optional(),
})
