import { useTranslation } from 'react-i18next'

import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import type { CurrentPrice } from '../types'
import { dimensions, price } from './current-price-format'

function ratio(value: number | null | undefined) {
  return value == null
    ? '—'
    : `${value > 0 && value < 0.0001 ? value.toPrecision(4) : Number(value.toFixed(4))}×`
}
function factor(value: number | null) {
  return value == null ? '—' : String(value)
}

function groupRateClass(
  dimension: (typeof dimensions)[number],
  value: number | null | undefined
): string {
  if (value == null) return 'text-foreground'
  if (dimension === 'cacheRead') return 'text-info-soft-fg'
  if (dimension === 'cacheWrite') return 'text-warning-soft-fg'
  return 'text-foreground'
}

export function AccountModelPrice({ row }: { row: CurrentPrice }) {
  const { t, i18n } = useTranslation()
  const nonlinear =
    Boolean(row.pricing.billingExpression) ||
    !['', 'flat', 'ratio'].includes(row.pricing.billingMode) ||
    ![0, 1].includes(row.pricing.quotaType)
  const perRequest = row.pricing.quotaType === 1
  return (
    <section className='space-y-3'>
      <div className='flex flex-wrap justify-between gap-2'>
        <h3 className='text-sm font-medium'>
          {row.siteName} · {row.username || `#${row.accountId}`}
        </h3>
        <span className='text-muted-foreground text-xs'>
          {formatDateTime(row.fetchedAt, toBcp47(i18n.language))}
        </span>
      </div>
      {row.lastError && (
        <p className='text-destructive text-xs'>
          {t('priceCompare.current.stale')}
        </p>
      )}
      {nonlinear ? (
        <div className='bg-muted/40 space-y-2 rounded-lg p-3'>
          <p className='text-sm'>{t('priceCompare.current.nonLinear')}</p>
          <pre className='text-xs break-all whitespace-pre-wrap'>
            {row.pricing.billingExpression || row.pricing.billingMode}
          </pre>
        </div>
      ) : (
        <p className='text-muted-foreground text-xs'>
          {perRequest
            ? t('priceCompare.current.perRequestFormula', {
                price: price(row.pricing.pricePerRequest),
              })
            : t('priceCompare.current.formula', {
                model: factor(row.pricing.modelRatio),
                output: factor(row.pricing.completionRatio),
                read: factor(row.pricing.cacheRatio),
                write: factor(row.pricing.cacheWriteRatio),
              })}
        </p>
      )}
      {row.groups.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('priceCompare.current.noGroups')}
        </p>
      ) : (
        <div className='bg-background divide-y rounded-lg border'>
          {row.groups.map((group) => (
            <div key={group.group} className='space-y-3 p-3'>
              <div className='flex flex-wrap items-baseline justify-between gap-2'>
                <div className='min-w-0 text-sm'>
                  <span className='font-medium'>{group.group}</span>
                  <span className='text-muted-foreground ml-2'>
                    {group.description}
                  </span>
                </div>
                <span className='text-muted-foreground text-xs'>
                  {t('priceCompare.current.groupRatio')}{' '}
                  <span className='text-foreground tabular-nums'>
                    {ratio(group.multiplier)}
                  </span>
                </span>
              </div>
              {perRequest ? (
                <div className='text-sm tabular-nums'>
                  {t('priceCompare.current.perRequest')}{' '}
                  {price(group.rates.perRequest)}
                </div>
              ) : (
                <div className='grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-4'>
                  {dimensions.map((d) => {
                    const valueClass = groupRateClass(d, group.rates[d])
                    return (
                      <div key={d}>
                        <p className='text-muted-foreground text-xs'>
                          {t(`priceCompare.current.${d}`)}
                        </p>
                        <p
                          className={`mt-1 text-sm font-medium tabular-nums ${valueClass}`}
                        >
                          {price(group.rates[d])}
                        </p>
                        <p className='text-muted-foreground mt-0.5 text-xs tabular-nums'>
                          {t('priceCompare.current.relative')}{' '}
                          {ratio(group.relative[d])}
                        </p>
                      </div>
                    )
                  })}
                </div>
              )}
              <p className='text-muted-foreground border-t pt-2 text-xs'>
                {perRequest
                  ? t('priceCompare.current.oneCall')
                  : t('priceCompare.current.sample')}{' '}
                <span className='text-foreground tabular-nums'>
                  {price(group.sampleCost, 6)}
                </span>
              </p>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
