// metapi-go features/token-routes/lib — RHF + Zod form schema for the
// add/edit route dialog, plus the URL-state schema for the routes list page.
//
// Mirrors the accounts feature's `lib/accounts-schema.ts` pattern: a schema
// *factory* (so cross-field rules can run in `superRefine`), a default-values
// factory, and two transformers (form → API payload, summary row → form
// defaults). The form is mode-aware (pattern vs explicit_group) — conditional
// required-field rules live in `superRefine` so the RHF field array stays
// stable across mode switches.
//
// The legacy create payload (verified against master:
// `web/pages/TokenRoutes.tsx:530-552`) carries `routeMode`, `modelPattern`
// (pattern mode), `displayName`, `displayIcon`, `contextLength`, and
// `sourceRouteIds` (explicit_group mode). The rewrite form additionally
// exposes `routingStrategy` and `modelMapping` (sent alongside the core keys
// — the backend `addRoute`/`updateRoute` accept sparse `any` bodies) and a
// `channelDrafts` array that the form's submit handler forwards to
// `api.batchAddChannels` after the route is created/updated.

import { z } from 'zod'

import {
  getModelPatternError,
  isRegexModelPattern,
  normalizeRouteDisplayIconValue,
} from '../utils'
import {
  type RouteFormPayload,
  type RouteMode,
  type RouteSummaryRow,
} from '../types'

// ---------------------------------------------------------------------------
// Form schema factory
// ---------------------------------------------------------------------------

export function getRouteFormSchema() {
  // `z.number()` (not `z.coerce.number()`) and no `.default()` so the schema's
  // input and output types are identical — zodResolver + RHF otherwise infer a
  // divergent input. Defaults are supplied by the form's default-values factory.
  return z
    .object({
      routeMode: z.enum(['pattern', 'explicit_group']),
      modelPattern: z.string().trim().optional(),
      displayName: z.string().trim().optional(),
      displayIcon: z.string().trim().optional(),
      contextLength: z.string().trim().optional(),
      sourceRouteIds: z.array(z.number().int().positive()).optional(),
      routingStrategy: z.enum(['weighted', 'round_robin', 'stable_first']).optional(),
      modelMapping: z.string().trim().optional(),
      channelDrafts: z
        .array(
          z.object({
            accountId: z.number().int().positive(),
            tokenId: z.number().int().positive().optional(),
            sourceModel: z.string().trim().optional(),
          }),
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
            message: '请填写模型匹配规则',
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
            message: '请填写对外模型名',
          })
        }
        if (!value.sourceRouteIds || value.sourceRouteIds.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['sourceRouteIds'],
            message: '至少选择一个来源路由',
          })
        }
      }

      if (value.contextLength) {
        if (!/^\d+$/.test(value.contextLength.trim())) {
          ctx.addIssue({
            code: 'custom',
            path: ['contextLength'],
            message: '上下文长度需为正整数',
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
  routeMode: RouteMode = 'explicit_group',
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
  raw: string | undefined,
): number | null | undefined {
  const trimmed = (raw || '').trim()
  if (!trimmed) return undefined
  if (!/^\d+$/.test(trimmed)) return undefined
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined
  return parsed
}

/**
 * Convert form values into the POST/PUT /api/routes payload. Pattern mode
 * sends `modelPattern`; explicit_group mode sends `sourceRouteIds`. Empty
 * `displayIcon` / `contextLength` / `modelMapping` are omitted
 * (JSON.stringify drops undefined keys, so the backend treats them as
 * "no change" / "unknown"). `routingStrategy` is forwarded when set so the
 * route is created/updated with the chosen strategy in one round-trip.
 */
export function transformFormToPayload(
  values: RouteFormValues,
): RouteFormPayload {
  const trimmedDisplayName = (values.displayName || '').trim() || undefined
  const trimmedDisplayIcon = normalizeRouteDisplayIconValue(values.displayIcon)
  const contextLength = parseContextLength(values.contextLength)
  const trimmedModelMapping = (values.modelMapping || '').trim() || undefined

  const base: RouteFormPayload = {
    routeMode: values.routeMode,
    displayName: trimmedDisplayName,
    displayIcon:
      trimmedDisplayIcon && trimmedDisplayIcon !== '' ? trimmedDisplayIcon : undefined,
    contextLength,
    ...(values.routingStrategy ? { routingStrategy: values.routingStrategy } : {}),
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

/**
 * Seed the form from an existing route (edit mode). The displayIcon sentinel
 * `__route_icon_none__` is preserved exactly so a "no icon" route does not
 * silently flip back to "auto" on save. `modelPattern` is only relevant for
 * pattern mode; `sourceRouteIds` only for explicit_group. Channel drafts start
 * empty on edit — the operator adds new channels via the form's channel
 * picker, and existing channels are managed in the detail sheet.
 */
export function transformRouteToFormValues(
  route: RouteSummaryRow,
): Partial<RouteFormValues> {
  const routeMode = route.routeMode === 'explicit_group' ? 'explicit_group' : 'pattern'
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
// URL state schema — routes list page (?q=&enabled=&site=&accountId=&page=&pageSize=)
// ---------------------------------------------------------------------------

export const routesSearchSchema = z.object({
  q: z.string().optional(),
  enabled: z.enum(['all', 'enabled', 'disabled']).optional(),
  site: z.string().optional(),
  accountId: z.coerce.number().int().positive().optional(),
  siteId: z.coerce.number().int().positive().optional(),
  page: z.coerce.number().int().positive().optional(),
  pageSize: z.coerce.number().int().positive().optional(),
})

export type RoutesSearch = z.infer<typeof routesSearchSchema>
