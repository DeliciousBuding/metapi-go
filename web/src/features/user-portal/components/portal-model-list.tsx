import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'

import type { PersonalModel } from '../api'
import { ModelSearchToolbar, type useModelSearch } from './portal-model-search'

function ModelRow({
  model,
  selectModel,
}: {
  model: PersonalModel
  selectModel: (model: PersonalModel) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between'>
      <div className='min-w-0 flex-1 space-y-2'>
        <ModelPill model={model.id} variant='inline' />
        <div className='flex flex-wrap gap-1.5'>
          {model.protocols.map((p) => (
            <Badge key={p} variant='secondary' className='font-mono text-xs'>
              {p}
            </Badge>
          ))}
        </div>
      </div>
      <Button
        variant='outline'
        size='sm'
        className='w-fit shrink-0'
        onClick={() => selectModel(model)}
        aria-label={t('userPortal.catalog.connectModel', {
          model: model.id,
        })}
      >
        {t('userPortal.catalog.details')}
        <ArrowRight aria-hidden='true' />
      </Button>
    </div>
  )
}

export function PortalModelList({
  items,
  state,
  selectModel,
}: {
  items: PersonalModel[]
  state: ReturnType<typeof useModelSearch>
  selectModel: (model: PersonalModel) => void
}) {
  const { t } = useTranslation()
  const filtered = items.filter((item) =>
    item.id.toLowerCase().includes(state.globalFilter.trim().toLowerCase())
  )
  const pageCount = Math.max(1, Math.ceil(filtered.length / 20))
  const pageIndex = Math.min(state.pagination.pageIndex, pageCount - 1)
  return (
    <>
      <ModelSearchToolbar
        search={state.globalFilter}
        count={filtered.length}
        onSearch={state.onGlobalFilterChange}
      />
      {filtered.length === 0 ? (
        <Notice tone='info'>
          <span>{t('userPortal.catalog.noMatches')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => state.onGlobalFilterChange('')}
          >
            {t('userPortal.catalog.clearSearch')}
          </Button>
        </Notice>
      ) : (
        <div className='bg-card divide-border divide-y rounded-xl border'>
          {filtered.slice(pageIndex * 20, (pageIndex + 1) * 20).map((model) => (
            <ModelRow
              key={model.routeId + ':' + model.id}
              model={model}
              selectModel={selectModel}
            />
          ))}
        </div>
      )}
      {pageCount > 1 && (
        <div className='flex flex-wrap items-center justify-between gap-3 text-sm'>
          <span className='text-muted-foreground tabular-nums'>
            {t('userPortal.pagination', {
              page: pageIndex + 1,
              total: pageCount,
              count: filtered.length,
            })}
          </span>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={pageIndex === 0}
              onClick={() =>
                state.onPaginationChange({
                  pageIndex: pageIndex - 1,
                  pageSize: 20,
                })
              }
            >
              {t('userPortal.previous')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              disabled={pageIndex + 1 >= pageCount}
              onClick={() =>
                state.onPaginationChange({
                  pageIndex: pageIndex + 1,
                  pageSize: 20,
                })
              }
            >
              {t('userPortal.next')}
            </Button>
          </div>
        </div>
      )}
    </>
  )
}
