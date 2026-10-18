import { ChevronDown, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toBcp47 } from '@/i18n/languages'
import { formatCurrency, formatDateTime } from '@/lib/format'

import { useRefreshAccountPricing } from '../api'
import type { CurrentPrice, PriceWorkspace } from '../types'

const dimensions = ['input', 'output', 'cacheRead', 'cacheWrite'] as const
function price(value: number | null | undefined, digits = 4) {
  if (value == null) return '—'
  return value > 0 && value < 10 ** -digits
    ? `$${value.toPrecision(4)}`
    : formatCurrency(value, { fractionDigits: digits })
}
function ratio(value: number | null | undefined) {
  return value == null
    ? '—'
    : `${value > 0 && value < 0.0001 ? value.toPrecision(4) : Number(value.toFixed(4))}×`
}
function factor(value: number | null) {
  return value == null ? '—' : String(value)
}

export function CurrentPrices({ data }: { data: PriceWorkspace }) {
  const { t, i18n } = useTranslation()
  const refresh = useRefreshAccountPricing()
  const accounts = data.pricingAccounts ?? []
  const items = data.currentPrices ?? []
  const models = [
    ...new Set([
      ...items.map((item) => item.model),
      ...Object.keys(data.officialPrices ?? {}),
    ]),
  ]
  return (
    <section className='space-y-4' aria-label={t('priceCompare.current.title')}>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h2 className='text-base font-semibold'>
          {t('priceCompare.current.title')}
        </h2>
        <p className='text-muted-foreground text-xs'>
          {t('priceCompare.current.units')}
        </p>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t('priceCompare.current.refreshHint')}
      </p>
      {accounts.length > 0 && (
        <Collapsible className='rounded-xl border'>
          <CollapsibleTrigger className='group flex w-full items-center justify-between gap-2 px-4 py-3 text-left text-sm font-medium'>
            {t('priceCompare.current.accounts', { count: accounts.length })}
            <ChevronDown
              aria-hidden='true'
              className='size-4 shrink-0 transition-transform group-data-[panel-open]:rotate-180'
            />
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className='divide-y border-t'>
              {accounts.map((account) => (
                <li
                  key={account.accountId}
                  className='flex flex-wrap items-center justify-between gap-3 px-4 py-3'
                >
                  <div className='min-w-0 space-y-1'>
                    <div className='text-sm break-words'>
                      {account.siteName} ·{' '}
                      {account.username || `#${account.accountId}`}
                    </div>
                    <p className='text-muted-foreground text-xs'>
                      {account.fetchedAt
                        ? t('priceCompare.current.fetched', {
                            time: formatDateTime(
                              account.fetchedAt,
                              toBcp47(i18n.language)
                            ),
                          })
                        : t('priceCompare.current.notFetched')}
                    </p>
                    {account.lastError && (
                      <p className='text-destructive text-xs'>
                        {t('priceCompare.current.refreshFailed')}
                      </p>
                    )}
                  </div>
                  {account.supported ? (
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={refresh.isPending}
                      onClick={() => refresh.mutate(account.accountId)}
                      aria-label={t('priceCompare.current.refreshAccount', {
                        name: account.siteName,
                      })}
                    >
                      <RefreshCw
                        aria-hidden='true'
                        className={
                          refresh.isPending &&
                          refresh.variables === account.accountId
                            ? 'animate-spin'
                            : undefined
                        }
                      />
                      {t('priceCompare.current.refresh')}
                    </Button>
                  ) : (
                    <span className='text-muted-foreground text-xs'>
                      {t('priceCompare.current.unsupported')}
                    </span>
                  )}
                </li>
              ))}
            </ul>
          </CollapsibleContent>
        </Collapsible>
      )}
      {refresh.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('priceCompare.current.refreshFailed')}
        </p>
      )}
      {models.map((model) => {
        const rows = items.filter((item) => item.model === model)
        const official = data.officialPrices?.[model] ?? rows[0]?.official
        return (
          <Card key={model}>
            <CardHeader className='gap-3'>
              <CardTitle>
                <ModelPill model={model} variant='inline' />
              </CardTitle>
              <div className='bg-muted/40 space-y-3 rounded-lg border p-3'>
                <div className='flex flex-wrap justify-between gap-2'>
                  <h3 className='text-sm font-medium'>
                    {t('priceCompare.current.official')}
                  </h3>
                  <span className='text-muted-foreground text-xs'>
                    {official?.source.startsWith('built-in presets')
                      ? t('priceCompare.current.builtIn', {
                          date:
                            official.source.match(/\d{4}-\d{2}-\d{2}/)?.[0] ??
                            '—',
                        })
                      : official?.source ||
                        t('priceCompare.current.noBaseline')}
                  </span>
                </div>
                <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
                  {dimensions.map((d) => (
                    <div key={d}>
                      <div className='text-muted-foreground text-xs'>
                        {t(`priceCompare.current.${d}`)}
                      </div>
                      <div className='mt-1 text-sm tabular-nums'>
                        {price(official?.rates[d])}
                      </div>
                    </div>
                  ))}
                </div>
                {official && (
                  <p className='text-muted-foreground text-xs break-words'>
                    {official.provider} / {official.model}
                    {official.aliased &&
                      ` · ${t('priceCompare.current.alias')}`}
                    {official.fetchedAt &&
                      ` · ${formatDateTime(official.fetchedAt, toBcp47(i18n.language))}`}
                    {official.updatedAt &&
                      ` · ${t('priceCompare.current.catalogUpdated', { date: official.updatedAt })}`}
                  </p>
                )}
              </div>
            </CardHeader>
            <CardContent className='space-y-4'>
              {rows.length === 0 && (
                <p className='text-muted-foreground text-sm'>
                  {t('priceCompare.current.noQuote')}
                </p>
              )}
              {rows.map((row) => (
                <AccountModelPrice
                  key={`${row.accountId}-${row.model}`}
                  row={row}
                />
              ))}
            </CardContent>
          </Card>
        )
      })}
      {models.length === 0 && (
        <p className='text-muted-foreground rounded-xl border border-dashed p-5 text-sm'>
          {t('priceCompare.current.noQuote')}
        </p>
      )}
    </section>
  )
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
        <div className='divide-y rounded-lg border'>
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
                  {dimensions.map((d) => (
                    <div key={d}>
                      <p className='text-muted-foreground text-xs'>
                        {t(`priceCompare.current.${d}`)}
                      </p>
                      <p className='mt-1 text-sm tabular-nums'>
                        {price(group.rates[d])}
                      </p>
                      <p className='text-muted-foreground mt-0.5 text-xs tabular-nums'>
                        {t('priceCompare.current.relative')}{' '}
                        {ratio(group.relative[d])}
                      </p>
                    </div>
                  ))}
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
