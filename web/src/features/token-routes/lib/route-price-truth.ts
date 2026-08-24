// metapi-go/features/token-routes — pure route allocation and price truth
// helpers. The price rows come from the models/price-compare feature; this
// module only joins that existing contract to route channels for display.

import {
  normalizePriceGrade,
  type PriceCompareItem,
  type PriceGrade,
} from '@/features/models/price-compare/types'

import type { RouteChannel, RouteSummaryRow } from '../types'
import { isExactModelPattern, isExplicitGroupRoute } from '../utils'

const EM_DASH = '—'

export type RouteChannelAllocation = {
  channelId: number
  configuredWeight: number
  enabledWeightShare: number | null
}

type RoutePriceProvenance = {
  source: PriceGrade | null
  ratesSource: PriceGrade | null
}

export type RouteChannelPriceTruth = {
  concreteModel: string | null
  price: PriceCompareItem | null
  inputPerMillion: number | null
  outputPerMillion: number | null
  provenance: RoutePriceProvenance
}

type RouteModelContext = Pick<RouteSummaryRow, 'modelPattern' | 'routeMode'>
type RouteChannelModel = Pick<RouteChannel, 'sourceModel'>
type RouteChannelWeight = Pick<RouteChannel, 'id' | 'enabled' | 'weight'>
type RouteChannelAccount = Pick<RouteChannel, 'accountId'>

function normalizeConcreteModelName(
  modelName: string | null | undefined
): string {
  return (modelName ?? '').trim()
}

export function normalizeModelKey(
  modelName: string | null | undefined
): string {
  return normalizeConcreteModelName(modelName).toLowerCase()
}

function isConcreteModelName(modelName: string): boolean {
  return isExactModelPattern(modelName)
}

export function resolveConcreteModelForChannel(
  route: RouteModelContext,
  channel: RouteChannelModel
): string | null {
  const sourceModel = normalizeConcreteModelName(channel.sourceModel)
  if (sourceModel) {
    return isConcreteModelName(sourceModel) ? sourceModel : null
  }

  if (isExplicitGroupRoute(route)) return null

  const routeModel = normalizeConcreteModelName(route.modelPattern)
  return isConcreteModelName(routeModel) ? routeModel : null
}

export function resolveDistinctConcreteModels(
  route: RouteModelContext,
  channels: readonly RouteChannelModel[]
): string[] {
  const seenModelKeys = new Set<string>()
  const concreteModels: string[] = []

  for (const channel of channels) {
    const concreteModel = resolveConcreteModelForChannel(route, channel)
    const modelKey = normalizeModelKey(concreteModel)
    if (!modelKey || seenModelKeys.has(modelKey)) continue
    seenModelKeys.add(modelKey)
    concreteModels.push(concreteModel as string)
  }

  return concreteModels
}

export function calculateRouteChannelAllocations(
  channels: readonly RouteChannelWeight[]
): RouteChannelAllocation[] {
  const enabledWeightTotal = channels.reduce(
    (weightTotal, channel) =>
      channel.enabled ? weightTotal + channel.weight : weightTotal,
    0
  )

  return channels.map((channel) => ({
    channelId: channel.id,
    configuredWeight: channel.weight,
    enabledWeightShare:
      !channel.enabled || enabledWeightTotal <= 0
        ? null
        : channel.weight / enabledWeightTotal,
  }))
}

function findRowsForModel(
  concreteModel: string,
  priceRowsByModel: ReadonlyMap<string, readonly PriceCompareItem[]>
): readonly PriceCompareItem[] {
  return priceRowsByModel.get(normalizeModelKey(concreteModel)) ?? []
}

function findPriceCompareRowForChannel(
  channel: RouteChannelAccount,
  concreteModel: string | null,
  priceRowsByModel: ReadonlyMap<string, readonly PriceCompareItem[]>
): PriceCompareItem | null {
  const modelKey = normalizeModelKey(concreteModel)
  if (!modelKey || channel.accountId <= 0) return null

  const matchingRows = findRowsForModel(
    concreteModel as string,
    priceRowsByModel
  )
  return (
    matchingRows.find(
      (priceRow) =>
        priceRow.accountId === channel.accountId &&
        normalizeModelKey(priceRow.model) === modelKey
    ) ?? null
  )
}

function resolveUsablePriceValue(
  price: PriceCompareItem | null,
  field: 'inputPerMillion' | 'outputPerMillion'
): number | null {
  if (!price || price.missingPrice) return null
  const value = price[field]
  return Number.isFinite(value) ? value : null
}

function deriveRoutePriceProvenance(
  price: PriceCompareItem | null
): RoutePriceProvenance {
  if (!price || price.missingPrice) {
    return { source: null, ratesSource: null }
  }
  return {
    source: normalizePriceGrade(price.source),
    ratesSource: normalizePriceGrade(price.ratesSource),
  }
}

export function resolveRouteChannelPriceTruth(
  route: RouteModelContext,
  channel: RouteChannel,
  priceRowsByModel: ReadonlyMap<string, readonly PriceCompareItem[]>
): RouteChannelPriceTruth {
  const concreteModel = resolveConcreteModelForChannel(route, channel)
  const price = findPriceCompareRowForChannel(
    channel,
    concreteModel,
    priceRowsByModel
  )
  return {
    concreteModel,
    price,
    inputPerMillion: resolveUsablePriceValue(price, 'inputPerMillion'),
    outputPerMillion: resolveUsablePriceValue(price, 'outputPerMillion'),
    provenance: deriveRoutePriceProvenance(price),
  }
}

export function formatRouteWeightShare(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return EM_DASH
  return `${(value * 100).toFixed(1)}%`
}
