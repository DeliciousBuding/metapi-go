import { useTranslation } from 'react-i18next'

import { Separator } from '@/components/ui/separator'
import { toBcp47 } from '@/i18n/languages'
import { formatCurrency, formatInt } from '@/lib/format'

import type {
  ProxyLogBillingDetails,
  ProxyLogBillingToolSurcharge,
} from '../types'
import { JsonBlock } from './json-block'

function isPositiveNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function getToolLabel(
  tool: string,
  t: ReturnType<typeof useTranslation>['t']
): string {
  switch (tool) {
    case 'web_search':
      return t('proxyLogs.usage.tools.web_search')
    case 'file_search':
      return t('proxyLogs.usage.tools.file_search')
    case 'grounding':
      return t('proxyLogs.usage.tools.grounding')
    default:
      return tool
  }
}

function getToolSourceLabel(
  source: string | null | undefined,
  t: ReturnType<typeof useTranslation>['t']
): string | null {
  if (typeof source !== 'string') return null
  const normalized = source.trim().toLowerCase()
  if (!normalized) return null
  if (
    normalized === 'vendor_reported' ||
    normalized === 'provider' ||
    normalized === 'upstream' ||
    normalized === 'reported' ||
    normalized === 'provider_reported'
  ) {
    return t('proxyLogs.usage.toolSource.provider')
  }
  if (
    normalized === 'response_derived' ||
    normalized === 'inferred' ||
    normalized === 'estimated' ||
    normalized === 'response' ||
    normalized === 'response_inferred'
  ) {
    return t('proxyLogs.usage.toolSource.inferred')
  }
  return source.trim()
}

function formatToolUnitPrice(
  surcharge: ProxyLogBillingToolSurcharge,
  locale: string,
  t: ReturnType<typeof useTranslation>['t']
): string {
  if (
    surcharge.unitPrice === undefined ||
    surcharge.unitPrice === null ||
    !Number.isFinite(surcharge.unitPrice)
  ) {
    return t('proxyLogs.usage.toolPriceMissing')
  }
  if (surcharge.unitPrice === 0) {
    return t('proxyLogs.usage.toolPriceFree')
  }
  return formatCurrency(surcharge.unitPrice, { fractionDigits: 6, locale })
}

export function BillingSection({
  billing,
}: {
  billing: ProxyLogBillingDetails
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!billing) return null
  const usage = billing.usage
  const breakdown = billing.breakdown
  const hasBaseRows =
    usage &&
    breakdown &&
    (typeof usage.promptTokens === 'number' ||
      typeof usage.billablePromptTokens === 'number') &&
    typeof breakdown.inputCost === 'number'

  const rows: Array<{
    key: string
    label: string
    tokens: number
    cost: number
  }> = []

  if (hasBaseRows) {
    rows.push(
      {
        key: 'input',
        label: t(
          typeof usage.billablePromptTokens === 'number'
            ? 'proxyLogs.usage.billableInput'
            : 'proxyLogs.usage.input'
        ),
        tokens: usage.billablePromptTokens ?? usage.promptTokens,
        cost: breakdown.inputCost,
      },
      {
        key: 'output',
        label: t('proxyLogs.usage.output'),
        tokens: usage.completionTokens,
        cost: breakdown.outputCost,
      },
      {
        key: 'cacheRead',
        label: t('proxyLogs.usage.cacheRead'),
        tokens: usage.cacheReadTokens,
        cost: breakdown.cacheReadCost,
      }
    )

    const cache5mTokens = isPositiveNumber(usage.cacheCreation5mTokens)
      ? usage.cacheCreation5mTokens
      : 0
    const cache1hTokens = isPositiveNumber(usage.cacheCreation1hTokens)
      ? usage.cacheCreation1hTokens
      : 0
    const cache12hTokens = isPositiveNumber(usage.cacheCreation12hTokens)
      ? usage.cacheCreation12hTokens
      : 0
    const hasTieredCacheCreation =
      cache5mTokens > 0 || cache1hTokens > 0 || cache12hTokens > 0

    if (hasTieredCacheCreation) {
      if (cache5mTokens > 0) {
        rows.push({
          key: 'cacheCreation5m',
          label: t('proxyLogs.usage.cacheCreation5m'),
          tokens: cache5mTokens,
          cost: breakdown.cacheCreation5mCost ?? 0,
        })
      }
      if (cache1hTokens > 0) {
        rows.push({
          key: 'cacheCreation1h',
          label: t('proxyLogs.usage.cacheCreation1h'),
          tokens: cache1hTokens,
          cost: breakdown.cacheCreation1hCost ?? 0,
        })
      }
      if (cache12hTokens > 0) {
        rows.push({
          key: 'cacheCreation12h',
          label: t('proxyLogs.usage.cacheCreation12h'),
          tokens: cache12hTokens,
          cost: breakdown.cacheCreation12hCost ?? 0,
        })
      }

      const tieredTokenSum = cache5mTokens + cache1hTokens + cache12hTokens
      const totalCreationTokens =
        typeof usage.cacheCreationTokens === 'number' &&
        Number.isFinite(usage.cacheCreationTokens)
          ? usage.cacheCreationTokens
          : 0
      const remainderTokens = Math.max(0, totalCreationTokens - tieredTokenSum)
      if (remainderTokens > 0) {
        const tieredCostSum =
          (breakdown.cacheCreation5mCost ?? 0) +
          (breakdown.cacheCreation1hCost ?? 0) +
          (breakdown.cacheCreation12hCost ?? 0)
        const remainderCost = Math.max(
          0,
          (breakdown.cacheCreationCost ?? 0) - tieredCostSum
        )
        rows.push({
          key: 'cacheCreationStandard',
          label: t('proxyLogs.usage.cacheCreationStandard'),
          tokens: remainderTokens,
          cost: remainderCost,
        })
      }
    } else {
      rows.push({
        key: 'cacheCreation',
        label: t('proxyLogs.usage.cacheCreation'),
        tokens: usage.cacheCreationTokens,
        cost: breakdown.cacheCreationCost,
      })
    }
  }

  const rawToolSurcharges = Array.isArray(breakdown?.toolSurcharges)
    ? breakdown.toolSurcharges
    : []
  const surchargeCounts = new Map<string, number>()
  const toolSurcharges = rawToolSurcharges.map((item) => {
    const base = `${item.tool}:${item.unit}:${item.source}:${item.quantity}:${item.cost}`
    const seen = (surchargeCounts.get(base) ?? 0) + 1
    surchargeCounts.set(base, seen)
    return {
      item,
      key: seen === 1 ? base : `${base}#${seen}`,
    }
  })

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.sectionBilling')}
        </h3>
        {rows.length > 0 || toolSurcharges.length > 0 ? (
          <>
            <dl className='divide-border divide-y rounded-md border px-3'>
              {rows.map((row) => (
                <div
                  key={row.key}
                  className='grid grid-cols-[1fr_auto_auto] items-baseline gap-3 py-2 text-xs'
                >
                  <dt className='text-muted-foreground'>{row.label}</dt>
                  <dd className='tabular-nums'>
                    {formatInt(row.tokens, locale)}
                  </dd>
                  <dd className='tabular-nums'>
                    {formatCurrency(row.cost, { fractionDigits: 6, locale })}
                  </dd>
                </div>
              ))}
              {toolSurcharges.length > 0 && (
                <div
                  className='space-y-1.5 py-2'
                  data-testid='billing-tool-surcharges'
                >
                  <dt className='text-muted-foreground text-xs font-medium'>
                    {t('proxyLogs.usage.toolSurchargesTitle')}
                  </dt>
                  <dd className='space-y-1.5'>
                    {toolSurcharges.map(({ item, key }) => {
                      const sourceLabel = getToolSourceLabel(item.source, t)
                      return (
                        <div
                          key={key}
                          className='grid grid-cols-[1fr_auto_auto] items-baseline gap-3 text-xs'
                        >
                          <div className='min-w-0 space-y-0.5'>
                            <div className='flex flex-wrap items-center gap-1.5'>
                              <span className='font-medium'>
                                {getToolLabel(item.tool, t)}
                              </span>
                              {sourceLabel && (
                                <span className='bg-muted text-muted-foreground text-2xs rounded-4xl border px-1.5 py-0.5'>
                                  {sourceLabel}
                                </span>
                              )}
                            </div>
                            <div className='text-muted-foreground tabular-nums'>
                              {formatInt(item.quantity, locale)} {item.unit}
                            </div>
                          </div>
                          <span className='text-muted-foreground tabular-nums'>
                            {formatToolUnitPrice(item, locale, t)}
                          </span>
                          <span className='tabular-nums'>
                            {formatCurrency(item.cost, {
                              fractionDigits: 6,
                              locale,
                            })}
                          </span>
                        </div>
                      )
                    })}
                  </dd>
                </div>
              )}
              {typeof breakdown?.totalCost === 'number' && (
                <div className='flex justify-between gap-3 py-2 text-sm font-medium'>
                  <dt>{t('proxyLogs.usage.totalCost')}</dt>
                  <dd className='tabular-nums'>
                    {formatCurrency(breakdown.totalCost, {
                      fractionDigits: 6,
                      locale,
                    })}
                  </dd>
                </div>
              )}
            </dl>
            <details className='mt-3 text-xs'>
              <summary className='text-muted-foreground cursor-pointer py-1'>
                {t('proxyLogs.usage.rawBilling')}
              </summary>
              <JsonBlock value={billing} />
            </details>
          </>
        ) : (
          <JsonBlock value={billing} />
        )}
      </section>
    </>
  )
}
