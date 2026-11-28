import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Box, Users as UsersIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { DataTablePage } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { useModelDetail } from '../api'
import { modelsKeys, type ModelRow } from '../types'
import { ModelDetailSheet } from './model-detail-sheet'
import { ModelVerifyDialog } from './model-verify-dialog'
import { useModelsTable } from './use-models-table'
import { useModelsToolbar } from './use-models-toolbar'

export function ModelsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const [viewingModel, setViewingModel] = useState<ModelRow | null>(null)
  const [verifyOpen, setVerifyOpen] = useState(false)

  // Consume the one-shot `?model=<name>` deep link exactly once (the search
  // palette passes it when a model hit is selected). The name is resolved by
  // the exact-name detail endpoint — not by scanning the loaded page — so a
  // model on any page (or under any active filter) still opens. A settled
  // failure is explicit: the notice carries the server's answer, then the
  // transient param is stripped so a refetch or remount never reopens it.
  const [deepLinkName] = useState<string | null>(() => {
    if (typeof window === 'undefined') return null
    return new URLSearchParams(window.location.search).get('model')
  })
  const modelDeepLinkConsumed = useRef(false)
  const deepLinkQuery = useModelDetail(
    deepLinkName && !modelDeepLinkConsumed.current ? deepLinkName : null
  )
  useEffect(() => {
    if (!deepLinkName || modelDeepLinkConsumed.current) return
    if (deepLinkQuery.isPending) return
    modelDeepLinkConsumed.current = true

    const model = deepLinkQuery.data?.model ?? null
    if (model) {
      setViewingModel(model)
    } else {
      toast.error(
        t('models.deepLink.unavailable', {
          name: deepLinkName,
          message: deepLinkQuery.error?.message ?? '',
        })
      )
    }
    const params = new URLSearchParams(window.location.search)
    params.delete('model')
    const queryString = params.toString()
    navigate({
      href: queryString ? `/models?${queryString}` : '/models',
      replace: true,
    })
  }, [
    deepLinkName,
    deepLinkQuery.isPending,
    deepLinkQuery.data,
    deepLinkQuery.error,
    navigate,
    t,
  ])

  const { table, modelsPageQuery } = useModelsTable(setViewingModel)
  const refreshMutation = useMutation({
    mutationFn: async () => {
      await api.getModelsMarketplace({ refresh: true, includePricing: true })
    },
    onSuccess: () => {
      // The refresh call re-aggregates the marketplace server-side. The list
      // query has a 10s staleTime and would keep serving the pre-refresh
      // cache during that window, so invalidate the models prefix explicitly
      // to refetch immediately (list + capability selectors share it).
      void queryClient.invalidateQueries({ queryKey: modelsKeys.all })
      toast.success(t('models.toast.refreshSucceeded'))
    },
  })

  const toolbar = useModelsToolbar(
    modelsPageQuery.data?.facets ?? null,
    () => setVerifyOpen(true),
    () => refreshMutation.mutate(),
    refreshMutation.isPending
  )

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t('models.page.title')}
        description={t('models.page.description')}
      />

      {/* Unified list-page error contract: the failed load
          replaces the table instead of stacking over it, so a stale cache can
          never read as current data. Query object is the server-side
          paged one (modelsPageQuery). */}
      <DataTablePage
        table={table}
        isLoading={modelsPageQuery.isLoading}
        isFetching={modelsPageQuery.isFetching}
        error={modelsPageQuery.error as Error | null}
        errorMessageKey='models.page.loadError'
        onErrorRetry={() => modelsPageQuery.refetch()}
        isErrorRetrying={modelsPageQuery.isFetching}
        emptyIcon={<Box className='size-6' />}
        emptyTitle={t('models.empty.title')}
        emptyDescription={t('models.empty.description')}
        emptyAction={
          <Button
            variant='outline'
            onClick={() => void navigate({ to: '/accounts' })}
          >
            <UsersIcon className='size-4' />
            {t('models.empty.manageAccounts')}
          </Button>
        }
        skeletonKeyPrefix='model-skeleton'
        toolbarProps={toolbar}
      />

      <ModelDetailSheet
        model={viewingModel}
        open={viewingModel !== null}
        onOpenChange={(open) => {
          if (!open) setViewingModel(null)
        }}
      />

      <ModelVerifyDialog open={verifyOpen} onOpenChange={setVerifyOpen} />
    </div>
  )
}
