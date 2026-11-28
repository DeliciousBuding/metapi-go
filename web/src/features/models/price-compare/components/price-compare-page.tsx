import { useNavigate, useSearch } from '@tanstack/react-router'
import { Scale } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ZodError } from 'zod'

import { PageHeader } from '@/components/common/page-header'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { Spinner } from '@/components/ui/spinner'
import { asStringParam } from '@/lib/helpers/searchParams'

import { usePriceWorkspace } from '../api'
import { CurrentPrices } from './current-prices'
import {
  PriceCompareSearch,
  PriceComparePagination,
} from './price-compare-controls'
import { ModelGroupRow } from './price-history-group-row'
import { groupPriceHistory } from './price-history-groups'
export { PriceRow } from './price-history-row'

export function PriceComparePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { model, pricePage } = useSearch({
    from: '/_authenticated/price-compare',
  })
  const modelParam = asStringParam(model)?.trim() ?? ''
  const query = usePriceWorkspace({
    model: modelParam || undefined,
    limit: 200,
    pricePage: pricePage ?? 1,
  })

  const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(
    () => new Set()
  )
  const toggleGroup = useCallback((key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const groups = useMemo(
    () => groupPriceHistory(query.data?.items ?? []),
    [query.data]
  )

  const currentPage = query.data?.currentPage

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t('priceCompare.page.title')}
        description={t('priceCompare.page.description')}
        actions={<PriceCompareSearch modelParam={modelParam} />}
      />

      {currentPage && currentPage.totalModels > 0 && (
        <PriceComparePagination
          currentPage={currentPage}
          isFetching={query.isFetching}
        />
      )}

      {query.data?.pricingAccounts && !query.error && (
        <CurrentPrices data={query.data} />
      )}

      {query.isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Spinner />
          {t('common.loading')}
        </div>
      )}

      <QueryErrorBanner
        error={query.error as Error | null}
        messageKey={
          query.error instanceof ZodError
            ? 'priceCompare.page.invalidResponse'
            : 'priceCompare.page.loadError'
        }
        onRetry={() => query.refetch()}
        isRetrying={query.isFetching}
      />

      {!query.isLoading &&
        !query.error &&
        groups.length === 0 &&
        (query.data?.pricingAccounts?.length ?? 0) === 0 && (
          // flex-none: the Empty base is flex-1, which would stretch the box
          // across the whole viewport — a content-sized empty state instead.
          <Empty className='flex-none border'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <Scale className='size-6' />
              </EmptyMedia>
              <EmptyDescription>
                {t('priceCompare.page.emptyDescription')}
              </EmptyDescription>
            </EmptyHeader>
            <Button
              variant='outline'
              onClick={() => void navigate({ to: '/accounts' })}
            >
              {t('priceCompare.page.emptyAction')}
            </Button>
          </Empty>
        )}

      {groups.length > 0 && (
        <section className='flex flex-col gap-2.5'>
          <div className='flex flex-wrap items-baseline justify-between gap-2'>
            <h2 className='text-base font-semibold'>
              {t('priceCompare.current.history')}
            </h2>
            <p className='text-muted-foreground text-xs'>
              {t('priceCompare.group.description')}
            </p>
          </div>

          <div className='bg-card divide-y overflow-hidden rounded-xl border'>
            {groups.map((group) => (
              <ModelGroupRow
                key={group.key}
                group={group}
                expanded={expandedKeys.has(group.key)}
                onToggle={() => toggleGroup(group.key)}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
