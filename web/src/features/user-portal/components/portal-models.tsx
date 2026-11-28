import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { SoftToneBadge } from '@/components/common/soft-tone-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { type PersonalModel, userPortalApi } from '../api'
import { modelContextHref, readModelContext } from '../connection-context'
import { ModelConnection } from './portal-model-connection'
import { PortalModelList } from './portal-model-list'
import { useModelSearch } from './portal-model-search'

const capabilityKeys = [
  'boundedText',
  'functionTools',
  'media',
  'hostedTools',
  'sessions',
] as const

export function PortalModels({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const state = useModelSearch()
  const locationSearch = useLocation({ select: (loc) => loc.searchStr })
  const navigate = useNavigate()
  const context = readModelContext(locationSearch)
  const query = useQuery({
    queryKey: ['userPortal', userId, 'models'],
    queryFn: userPortalApi.models,
    refetchOnMount: 'always',
  })
  // Do not keep a stale model's permissions or connection example after refresh.
  const selected = !query.isError
    ? query.data?.items.find((item) => item.id === context.model)
    : undefined
  function selectModel(model: PersonalModel) {
    const requestedProtocol = context.protocol
    const protocol = model.protocols.includes(requestedProtocol as never)
      ? requestedProtocol
      : undefined
    void navigate({
      href: modelContextHref(locationSearch, { model: model.id, protocol }),
    })
  }
  function renderCatalogBody() {
    if (query.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    if (query.isError) {
      return (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      )
    }
    const multiplierText = new Intl.NumberFormat(i18n.language).format(
      query.data.quota.groupMultiplier
    )
    return (
      <>
        {context.model && !selected && !query.isFetching && (
          <Notice tone='warning'>
            <p className='wrap-anywhere'>
              {t('userPortal.catalog.selectedUnavailable', {
                model: context.model,
              })}
            </p>
            <Button
              size='sm'
              variant='outline'
              onClick={() =>
                void navigate({ href: modelContextHref(locationSearch, {}) })
              }
            >
              {t('userPortal.catalog.clearSelection')}
            </Button>
          </Notice>
        )}
        <div className='grid gap-4 lg:grid-cols-2'>
          <Card size='sm'>
            <CardContent className='space-y-2.5'>
              <h2 className='text-sm font-medium'>
                {t('userPortal.capabilities.title')}
              </h2>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t('userPortal.capabilities.intro')}
              </p>
              <ul className='grid gap-1.5 text-xs leading-relaxed'>
                {capabilityKeys.map((key) => (
                  <li className='flex gap-2' key={key}>
                    <span aria-hidden='true' className='text-primary mt-0.5'>
                      •
                    </span>
                    <span>{t('userPortal.capabilities.' + key)}</span>
                  </li>
                ))}
              </ul>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t('userPortal.capabilities.protocols')}
              </p>
            </CardContent>
          </Card>
          <Card size='sm'>
            <CardContent className='space-y-2.5'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <h2 className='text-sm font-medium'>
                  {t('userPortal.catalog.quotaTitle')}
                </h2>
                <SoftToneBadge tone='info'>{multiplierText}×</SoftToneBadge>
              </div>
              <p className='text-sm leading-relaxed tabular-nums'>
                {t('userPortal.catalog.quotaFormula', {
                  multiplier: multiplierText,
                })}
              </p>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t('userPortal.catalog.quotaHint')}
              </p>
            </CardContent>
          </Card>
        </div>
        {query.data.items.length === 0 ? (
          <Notice tone='info'>{t('userPortal.noModels')}</Notice>
        ) : (
          <PortalModelList
            items={query.data.items}
            state={state}
            selectModel={selectModel}
          />
        )}
      </>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.modelsHint')}
        </p>
        <QuietRefreshStatus
          updatedAt={query.dataUpdatedAt}
          hasError={query.isError}
        />
      </div>
      {renderCatalogBody()}
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open)
            void navigate({
              href: modelContextHref(locationSearch, {}),
            })
        }}
      >
        <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle className='wrap-anywhere'>{selected?.id}</DialogTitle>
            <DialogDescription>
              {t('userPortal.catalog.detailHint')}
            </DialogDescription>
          </DialogHeader>
          {selected && (
            <ModelConnection
              key={selected.id + ':' + selected.protocols.join(',')}
              model={selected}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
