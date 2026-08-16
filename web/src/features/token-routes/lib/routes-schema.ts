// metapi-go/features/token-routes/lib — RHF + Zod form schema for the
// add/edit route dialog, plus the URL-state schema for the routes list page.
//
// Error messages are i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

import { stringSearchParam } from '@/lib/helpers/searchParams'

import type { RouteFormPayload, RouteMode, RouteSummaryRow } from '../types'
import {
  getModelPatternError,
  isRegexModelPattern,
  normalizeRouteDisplayIconValue,
} from '../utils'

// ---------------------------------------------------------------------------
// Form schema factory
// ---------------------------------------------------------------------------

export function getRouteFormSchema() {
  return z
    .object({
      routeMode: z.enum(['pattern', 'explicit_group']),
      modelPattern: z.string().trim().optional(),
      displayName: z.string().trim().optional(),
      displayIcon: z.string().trim().optional(),
      contextLength: z.string().trim().optional(),
      sourceRouteIds: z.array(z.number().int().positive()).optional(),
      routingStrategy: z
        .enum([
          'weighted',
          'round_robin',
          'stable_first',
          'least_busy',
          'lowest_latency',
          'lowest_cost',
        ])
        .optional(),
      modelMapping: z.string().trim().optional(),
      channelDrafts: z
        .array(
          z.object({
            accountId: z.number().int().positive(),
            tokenId: z.number().int().positive().optional(),
            sourceModel: z.string().trim().optional(),
          })
        )
        .optional(),
    })
    .superRefine((value, ctx) => {
      if (value.routeMode === 'pattern') {
        const pattern = (value.modelPattern || '').trim()
        if (!pattern) {
          ctx.addIssue({
            code: 'custom',
            path: ['modelPattern'],
            message: 'tokenRoutes.schema.modelPatternRequired',
          })
        } else if (isRegexModelPattern(pattern)) {
          const error = getModelPatternError(pattern)
          if (error) {
            ctx.addIssue({
              code: 'custom',
              path: ['modelPattern'],
              message: error,
            })
          }
        }
      } else {
        const displayName = (value.displayName || '').trim()
        if (!displayName) {
          ctx.addIssue({
            code: 'custom',
            path: ['displayName'],
            message: 'tokenRoutes.schema.displayNameRequired',
          })
        }
        if (!value.sourceRouteIds || value.sourceRouteIds.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['sourceRouteIds'],
            message: 'tokenRoutes.schema.sourceRoutesRequired',
          })
        }
      }

      if (value.contextLength) {
        if (!/^\d+$/.test(value.contextLength.trim())) {
          ctx.addIssue({
            code: 'custom',
            path: ['contextLength'],
            message: 'tokenRoutes.schema.contextLengthPositive',
          })
        }
      }
    })
}

export type RouteFormValues = z.infer<ReturnType<typeof getRouteFormSchema>>

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

export function getRouteFormDefaultValues(
  routeMode: RouteMode = 'explicit_group'
): RouteFormValues {
  return {
    routeMode,
    modelPattern: '',
    displayName: '',
    displayIcon: '',
    contextLength: '',
    sourceRouteIds: [],
    routingStrategy: undefined,
    modelMapping: '',
    channelDrafts: [],
  }
}

// ---------------------------------------------------------------------------
// Transformers
// ---------------------------------------------------------------------------

function parseContextLength(
  raw: string | undefined
): number | null | undefined {
  const trimmed = (raw || '').trim()
  if (!trimmed) return undefined
  if (!/^\d+$/.test(trimmed)) return undefined
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined
  return parsed
}

export function transformFormToPayload(
  values: RouteFormValues
): RouteFormPayload {
  const trimmedDisplayName = (values.displayName || '').trim() || undefined
  const trimmedDisplayIcon = normalizeRouteDisplayIconValue(values.displayIcon)
  const contextLength = parseContextLength(values.contextLength)
  const trimmedModelMapping = (values.modelMapping || '').trim() || undefined

  const base: RouteFormPayload = {
    routeMode: values.routeMode,
    displayName: trimmedDisplayName,
    displayIcon:
      trimmedDisplayIcon && trimmedDisplayIcon !== ''
        ? trimmedDisplayIcon
        : undefined,
    contextLength,
    ...(values.routingStrategy
      ? { routingStrategy: values.routingStrategy }
      : {}),
    ...(trimmedModelMapping ? { modelMapping: trimmedModelMapping } : {}),
  }

  if (values.routeMode === 'pattern') {
    return {
      ...base,
      modelPattern: (values.modelPattern || '').trim() || undefined,
    }
  }

  return {
    ...base,
    sourceRouteIds: values.sourceRouteIds ?? [],
  }
}

export function transformRouteToFormValues(
  route: RouteSummaryRow
): Partial<RouteFormValues> {
  const routeMode =
    route.routeMode === 'explicit_group' ? 'explicit_group' : 'pattern'
  return {
    routeMode,
    modelPattern: route.modelPattern ?? '',
    displayName: route.displayName ?? '',
    displayIcon: normalizeRouteDisplayIconValue(route.displayIcon),
    contextLength: route.contextLength ? String(route.contextLength) : '',
    sourceRouteIds: route.sourceRouteIds ?? [],
    routingStrategy: route.routingStrategy ?? undefined,
    modelMapping: route.modelMapping ?? '',
    channelDrafts: [],
  }
}

// ---------------------------------------------------------------------------
// Channel-draft seed (account → route guided chain)
// ---------------------------------------------------------------------------

/**
 * Seed one `channelDrafts` entry from the guided-chain account deep link.
 * Returns a single-element draft when the account id is a positive integer
 * (the same validity contract as the channelDrafts schema), otherwise an
 * empty array — so direct route entry and edit flows keep their normal
 * defaults and never gain a phantom pre-selected channel.
 */
export function buildChannelDraftSeed(
  accountId: number | undefined
): Array<{ accountId: number }> {
  return accountId && Number.isInteger(accountId) && accountId > 0
    ? [{ accountId }]
    : []
}

// ---------------------------------------------------------------------------
// URL state schema
// ---------------------------------------------------------------------------

// Tolerant URL search contract: the router JSON-parses search values, so
// `?q=123` arrives as a number and `?enabled=true` as a boolean. Strings use
// `stringSearchParam` (normalized via `asStringParam` by the page) and the
// numerics fall back to sane defaults instead of throwing a route error.
export const routesSearchSchema = z.object({
  q: stringSearchParam,
  // The page encodes the enabled filter as a comma-separated string
  // (`enabled,disabled`), so the schema accepts a raw string and the page
  // splits it — a single enum would reject the page's own writes.
  enabled: stringSearchParam,
  accountId: z.coerce.number().int().positive().optional().catch(undefined),
  siteId: z.coerce.number().int().positive().optional().catch(undefined),
  page: z.coerce.number().int().positive().catch(1).default(1),
  pageSize: z.coerce.number().int().positive().catch(20).default(20),
})
