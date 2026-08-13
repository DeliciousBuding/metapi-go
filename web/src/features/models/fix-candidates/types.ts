// metapi-go/features/models/fix-candidates — domain types for redirect
// fix-candidate review/apply. Mirrors service.RedirectFixCandidate.

import { z } from 'zod'

export const redirectFixCandidateSchema = z.object({
  siteId: z.coerce.number().default(0),
  siteName: z.string().catch(''),
  accountId: z.coerce.number().default(0),
  modelName: z.string().catch(''),
  canonical: z.string().catch(''),
  actual: z.string().catch(''),
})
export type RedirectFixCandidate = z.infer<typeof redirectFixCandidateSchema>

export const redirectFixCandidatesResponseSchema = z.object({
  items: z.array(redirectFixCandidateSchema).catch([]),
  count: z.coerce.number().default(0),
})
export type RedirectFixCandidatesResponse = z.infer<
  typeof redirectFixCandidatesResponseSchema
>

export const redirectFixApplyResponseSchema = z.object({
  success: z.coerce.boolean().default(false),
  dryRun: z.coerce.boolean().default(false),
  removed: z.coerce.number().nullish(),
  count: z.coerce.number().nullish(),
})
export type RedirectFixApplyResponse = z.infer<
  typeof redirectFixApplyResponseSchema
>

export const fixCandidatesQueryKeys = {
  all: ['models', 'redirect-fix-candidates'] as const,
  list: () => [...fixCandidatesQueryKeys.all, 'list'] as const,
}
