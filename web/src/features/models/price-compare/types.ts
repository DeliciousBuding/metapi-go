// metapi-go/features/models/price-compare — domain types for cross-site
// effective price comparison. Mirrors handler/admin/model_price_compare.go
// and routing.PriceCompareRow (camelCase wire contract).

import { z } from 'zod'

/** Provenance grades for a price, mirroring routing price-source labels. */
const priceGradeValues = [
  'billing_details',
  'observed',
  'configured',
  'fallback',
] as const
export type PriceGrade = (typeof priceGradeValues)[number]

function isPriceGrade(value: string): value is PriceGrade {
  return (priceGradeValues as readonly string[]).includes(value)
}

export function normalizePriceGrade(
  value: string | null | undefined
): PriceGrade {
  return value && isPriceGrade(value) ? value : 'fallback'
}

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

const rate = z.number().finite().nonnegative().nullable()
const ratesSchema = z.object({
  input: rate,
  output: rate,
  cacheRead: rate,
  cacheWrite: rate,
  perRequest: rate,
})
const officialSchema = z.object({
  model: z.string(),
  provider: z.string(),
  aliased: z.boolean(),
  source: z.string(),
  fetchedAt: z.string(),
  updatedAt: z.string(),
  known: z.boolean(),
  rates: ratesSchema,
})
export const currentPriceSchema = z.object({
  accountId: z.number(),
  siteName: z.string(),
  username: z.string(),
  model: z.string(),
  fetchedAt: z.string(),
  lastError: z.string(),
  pricing: z.object({
    model: z.string(),
    quotaType: z.number(),
    modelRatio: rate,
    completionRatio: rate,
    cacheRatio: rate,
    cacheWriteRatio: rate,
    pricePerRequest: rate,
    groups: z.array(z.string()).nullable(),
    billingMode: z.string(),
    billingExpression: z.string(),
  }),
  groups: z.array(
    z.object({
      group: z.string(),
      description: z.string(),
      multiplier: rate,
      rates: ratesSchema,
      relative: ratesSchema,
      sampleCost: rate,
    })
  ),
  official: officialSchema.nullable(),
})
export type CurrentPrice = z.infer<typeof currentPriceSchema>
const pricingAccountSchema = z.object({
  accountId: z.number(),
  siteName: z.string(),
  username: z.string(),
  platform: z.string(),
  fetchedAt: z.string(),
  attemptedAt: z.string(),
  lastError: z.string(),
  supported: z.boolean(),
})

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
  items: z.array(priceCompareItemSchema),
  pricingAccounts: z.array(pricingAccountSchema).optional(),
  currentPrices: z.array(currentPriceSchema).optional(),
  officialPrices: z.record(z.string(), officialSchema.nullable()).optional(),
  currentPage: z
    .object({
      page: z.number().int().positive(),
      pageSize: z.number().int().positive(),
      totalModels: z.number().int().nonnegative(),
    })
    .optional(),
  meta: z.unknown().nullish(),
})
export type PriceWorkspace = z.infer<typeof priceCompareResponseSchema>

export const priceCompareQueryKeys = {
  all: ['models', 'price-compare'] as const,
  list: (params: {
    model?: string
    days?: number
    limit?: number
    topModels?: number
    exactModel?: boolean
  }) => [...priceCompareQueryKeys.all, params] as const,
}
