import { z } from 'zod'

import {
  asBoolean,
  asNumber,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

// Loose client-side shape guard: comma-separated codes ("401") and
// inclusive ranges ("500-599"). Range bounds and order are validated
// server-side (PUT /api/settings/runtime returns 400 for bad specs).
const statusRangesSpec = z
  .string()
  .refine(
    (value) =>
      value === '' || /^(\d{3}(-\d{3})?)(,\s*\d{3}(-\d{3})?)*$/.test(value),
    { message: 'settings.proxyModels.routing.schema.statusRangesInvalid' }
  )

export const routingSchema = z.object({
  routingFallbackUnitCost: z.coerce.number().min(0),
  tokenRouterFailureCooldownMaxSec: z.coerce.number().int().min(0),
  proxyFirstByteTimeoutSec: z.coerce.number().int().min(0),
  proxyRetryStatusRanges: statusRangesSpec,
  proxyDisableStatusRanges: statusRangesSpec,
  disableCrossProtocolFallback: z.boolean(),
  routingWeights: z.object({
    baseWeightFactor: z.coerce.number(),
    valueScoreFactor: z.coerce.number(),
    costWeight: z.coerce.number(),
    balanceWeight: z.coerce.number(),
    usageWeight: z.coerce.number(),
  }),
})

export type RoutingFormValues = z.infer<typeof routingSchema>

type RoutingPreset = {
  id: 'balanced' | 'stable' | 'cost'
  weights: NonNullable<RoutingFormValues['routingWeights']>
}

export const ROUTING_PRESETS: readonly RoutingPreset[] = [
  {
    id: 'balanced',
    weights: {
      baseWeightFactor: 0.5,
      valueScoreFactor: 0.5,
      costWeight: 0.4,
      balanceWeight: 0.3,
      usageWeight: 0.3,
    },
  },
  {
    id: 'stable',
    weights: {
      baseWeightFactor: 0.7,
      valueScoreFactor: 0.3,
      costWeight: 0.2,
      balanceWeight: 0.5,
      usageWeight: 0.2,
    },
  },
  {
    id: 'cost',
    weights: {
      baseWeightFactor: 0.3,
      valueScoreFactor: 0.4,
      costWeight: 0.7,
      balanceWeight: 0.2,
      usageWeight: 0.4,
    },
  },
]

const DEFAULT_WEIGHTS: RoutingPreset['weights'] = ROUTING_PRESETS[0].weights

export const DEFAULT_VALUES: RoutingFormValues = {
  routingFallbackUnitCost: 1,
  tokenRouterFailureCooldownMaxSec: 30 * 24 * 60 * 60,
  proxyFirstByteTimeoutSec: 0,
  proxyRetryStatusRanges: '',
  proxyDisableStatusRanges: '',
  disableCrossProtocolFallback: false,
  routingWeights: { ...DEFAULT_WEIGHTS },
}

export function deriveServerValues(
  data: RuntimeSettings | undefined
): RoutingFormValues | null {
  if (!data) {
    return null
  }
  const incomingWeights = (data.routingWeights ?? {}) as Record<string, unknown>
  return {
    routingFallbackUnitCost: asNumber(data.routingFallbackUnitCost) ?? 1,
    tokenRouterFailureCooldownMaxSec:
      asNumber(data.tokenRouterFailureCooldownMaxSec) ?? 30 * 24 * 60 * 60,
    proxyFirstByteTimeoutSec: asNumber(data.proxyFirstByteTimeoutSec) ?? 0,
    proxyRetryStatusRanges: data.proxyRetryStatusRanges ?? '',
    proxyDisableStatusRanges: data.proxyDisableStatusRanges ?? '',
    disableCrossProtocolFallback: asBoolean(data.disableCrossProtocolFallback),
    routingWeights: {
      baseWeightFactor:
        asNumber(incomingWeights.baseWeightFactor) ??
        DEFAULT_WEIGHTS.baseWeightFactor,
      valueScoreFactor:
        asNumber(incomingWeights.valueScoreFactor) ??
        DEFAULT_WEIGHTS.valueScoreFactor,
      costWeight:
        asNumber(incomingWeights.costWeight) ?? DEFAULT_WEIGHTS.costWeight,
      balanceWeight:
        asNumber(incomingWeights.balanceWeight) ??
        DEFAULT_WEIGHTS.balanceWeight,
      usageWeight:
        asNumber(incomingWeights.usageWeight) ?? DEFAULT_WEIGHTS.usageWeight,
    },
  }
}
