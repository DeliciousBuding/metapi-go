import { ChevronDown, RefreshCw } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import { useRefreshAccountPricing } from '../api'
import type { PriceWorkspace } from '../types'
import { PublishedPriceRow } from './published-price-row'
export { AccountModelPrice } from './account-model-price'

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

  // Single-model view starts expanded so targeted search/single-model tests
  // immediately reveal full group details; multi-model lists stay compact
  // until expanded via RowDisclosure.
  const [expandedModels, setExpandedModels] = useState<ReadonlySet<string>>(
    () => new Set(models.length === 1 ? models : [])
  )
  const toggleModel = useCallback((model: string) => {
    setExpandedModels((prev) => {
      const next = new Set(prev)
      if (next.has(model)) next.delete(model)
      else next.add(model)
      return next
    })
  }, [])

  return (
    <section className='space-y-3' aria-label={t('priceCompare.current.title')}>
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
        <Collapsible className='bg-card rounded-xl border'>
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
      {models.length > 0 ? (
        <div className='bg-card divide-y overflow-hidden rounded-xl border'>
          {models.map((model) => {
            const rows = items.filter((item) => item.model === model)
            const official = data.officialPrices?.[model] ?? rows[0]?.official
            return (
              <PublishedPriceRow
                key={model}
                model={model}
                rows={rows}
                official={official}
                isExpanded={expandedModels.has(model)}
                onToggle={() => toggleModel(model)}
              />
            )
          })}
        </div>
      ) : (
        <p className='text-muted-foreground rounded-xl border border-dashed p-5 text-sm'>
          {t('priceCompare.current.noQuote')}
        </p>
      )}
    </section>
  )
}
