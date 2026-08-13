// metapi-go/features/models/price-compare — domain types for cross-site
// effective price comparison. Mirrors handler/admin/model_price_compare.go
// and routing.PriceCompareRow (camelCase wire contract).

import { z } from 'zod'

/** Provenance grades for a price, mirroring routing price-source labels. */
export const priceGradeValues = [
  'billing_details',
  'observed',
  'configured',
  'fallback',
] as const
export type PriceGrade = (typeof priceGradeValues)[number]

const priceCompareItemSchema = z.object({
  siteId: z.coerce.number().default(0),
  siteName: z.string().catch(''),
  platform: z.string().catch(''),
  model: z.string().catch(''),
  accountId: z.coerce.number().default(0),
  username: z.string().nullish().default(null),
  inputPerMillion: z.coerce.number().default(0),
  outputPerMillion: z.coerce.number().default(0),
  source: z.string().catch('fallback'),
  ratesSource: z.string().catch('fallback'),
  estimatedCostSample: z.coerce.number().default(0),
  observedSamples: z.coerce.number().default(0),
  configuredUnitCost: z.number().nullish(),
  missingPrice: z.coerce.boolean().default(false),
  recommended: z.coerce.boolean().default(false),
})
export type PriceCompareItem = z.infer<typeof priceCompareItemSchema>

export const priceCompareResponseSchema = z.object({
  model: z.string().catch(''),
  days: z.coerce.number().default(30),
  limit: z.coerce.number().default(50),
  sampleUsage: z
    .object({
      promptTokens: z.coerce.number().default(1000),
      completionTokens: z.coerce.number().default(1000),
      totalTokens: z.coerce.number().default(2000),
    })
    .catch({ promptTokens: 1000, completionTokens: 1000, totalTokens: 2000 }),
  items: z.array(priceCompareItemSchema).catch([]),
  meta: z.unknown().nullish(),
})

export const priceCompareQueryKeys = {
  all: ['models', 'price-compare'] as const,
  list: (params: {
    model?: string
    days?: number
    limit?: number
    topModels?: number
  }) => [...priceCompareQueryKeys.all, params] as const,
}
