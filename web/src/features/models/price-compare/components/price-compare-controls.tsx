import { useNavigate } from '@tanstack/react-router'
import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { PriceWorkspace } from '../types'

export function PriceCompareSearch({ modelParam }: { modelParam: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <form
      className='flex w-full gap-2 sm:w-72'
      onSubmit={(event) => {
        event.preventDefault()
        const next = String(
          new FormData(event.currentTarget).get('model') ?? ''
        ).trim()
        void navigate({
          href: next
            ? `/price-compare?${new URLSearchParams({ model: next })}`
            : '/price-compare',
        })
      }}
    >
      <Input
        key={modelParam}
        name='model'
        defaultValue={modelParam}
        placeholder={t('priceCompare.page.searchPlaceholder')}
        aria-label={t('priceCompare.page.searchPlaceholder')}
      />
      <Button
        type='submit'
        variant='outline'
        size='icon'
        aria-label={t('priceCompare.current.search')}
      >
        <Search aria-hidden='true' />
      </Button>
    </form>
  )
}

export function PriceComparePagination({
  currentPage,
  isFetching,
}: {
  currentPage: NonNullable<PriceWorkspace['currentPage']>
  isFetching: boolean
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <nav
      className='flex flex-wrap items-center justify-between gap-3'
      aria-label={t('priceCompare.current.pagination')}
    >
      <p className='text-muted-foreground text-sm tabular-nums'>
        {t('priceCompare.current.pageSummary', {
          page: currentPage.page,
          pages: Math.max(
            1,
            Math.ceil(currentPage.totalModels / currentPage.pageSize)
          ),
          count: currentPage.totalModels,
        })}
      </p>
      <div className='flex gap-2'>
        {[-1, 1].map((delta) => (
          <Button
            key={delta}
            variant='outline'
            size='sm'
            disabled={
              isFetching ||
              (delta < 0
                ? currentPage.page <= 1
                : currentPage.page * currentPage.pageSize >=
                  currentPage.totalModels)
            }
            onClick={() => {
              const params = new URLSearchParams(window.location.search)
              params.set('pricePage', String(currentPage.page + delta))
              void navigate({ href: `/price-compare?${params}` })
            }}
          >
            {t(
              delta < 0
                ? 'priceCompare.current.previous'
                : 'priceCompare.current.next'
            )}
          </Button>
        ))}
      </div>
    </nav>
  )
}
