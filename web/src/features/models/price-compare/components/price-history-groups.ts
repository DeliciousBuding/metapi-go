import type { PriceCompareItem } from '../types'

export type ModelGroup = {
  key: string
  model: string
  rows: PriceCompareItem[]
  bestRow: PriceCompareItem
  minSample: number | null
  avgSample: number | null
  spreadPct: number | null
}

function summarizeGroup(key: string, rows: PriceCompareItem[]): ModelGroup {
  const bestRow =
    rows.find((row) => row.recommended && row.source === 'billing_details') ??
    rows[0]
  if (!bestRow) {
    throw new Error(`Expected non-empty price group for ${key}`)
  }
  const sampleCosts = rows
    .filter(
      (row) =>
        !row.missingPrice &&
        Number.isFinite(row.estimatedCostSample) &&
        row.estimatedCostSample > 0
    )
    .map((row) => row.estimatedCostSample)

  const minSample = sampleCosts.length > 0 ? Math.min(...sampleCosts) : null
  const avgSample =
    sampleCosts.length > 0
      ? sampleCosts.reduce((sum, value) => sum + value, 0) / sampleCosts.length
      : null
  const spreadPct =
    minSample !== null && avgSample !== null && avgSample > minSample
      ? Math.round(((avgSample - minSample) / avgSample) * 100)
      : null

  return {
    key,
    model: bestRow.model,
    rows,
    bestRow,
    minSample,
    avgSample,
    spreadPct,
  }
}

export function groupPriceHistory(items: PriceCompareItem[]): ModelGroup[] {
  const map = new Map<string, PriceCompareItem[]>()
  for (const item of items) {
    if (item.missingPrice) continue
    const key = item.model.trim().toLowerCase() || '__unknown__'
    const list = map.get(key) ?? []
    list.push(item)
    map.set(key, list)
  }
  return [...map.entries()].map(([key, rows]) => summarizeGroup(key, rows))
}
