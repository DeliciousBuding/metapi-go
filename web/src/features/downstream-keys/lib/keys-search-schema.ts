import { z } from 'zod'

import { stringSearchParam } from '@/lib/helpers/searchParams'

// One-shot edit drill-down used by diagnostic surfaces: `edit` identifies the
// downstream key and `locateField` optionally names a `data-key-scope` anchor
// in the edit sheet. Invalid values degrade to unset instead of throwing a
// route error, matching the other tolerant list-page search contracts.
export const downstreamKeysSearchSchema = z.object({
  edit: z.coerce.number().int().positive().optional().catch(undefined),
  locateField: stringSearchParam,
})
