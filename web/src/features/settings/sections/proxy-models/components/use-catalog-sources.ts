import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api, type CatalogSource, type CatalogSyncStatus } from '@/lib/api'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

const catalogSyncKeys = {
  all: ['catalog-sync'] as const,
  status: () => [...catalogSyncKeys.all, 'status'] as const,
}

export function useCatalogSources() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const undoableDelete = useUndoableDelete()
  const [syncingId, setSyncingId] = useState<number | 'all' | null>(null)

  const statusQuery = useQuery<CatalogSyncStatus>({
    queryKey: catalogSyncKeys.status(),
    queryFn: async () => api.getCatalogSync(),
    staleTime: 10 * 1000,
  })

  const refreshStatus = (status: CatalogSyncStatus) => {
    queryClient.setQueryData(catalogSyncKeys.status(), status)
  }

  const syncMutation = useMutation({
    mutationFn: (target: number | 'all') =>
      target === 'all' ? api.syncCatalog() : api.syncCatalog(target),
    onMutate: (target) => setSyncingId(target),
    onSettled: () => setSyncingId(null),
    onSuccess: (status) => {
      refreshStatus(status)
      // Syncing catalog sources adds/updates marketplace models + pricing;
      // refresh the models marketplace so the Models page is not stale.
      void queryClient.invalidateQueries({ queryKey: ['models'] })
      toast.success(
        t('settings.proxyModels.catalogSources.toast.syncSucceeded', {
          count: status.snapshot.models,
        })
      )
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.syncFailed', {
          message: error.message,
        })
      ),
  })

  const autoSyncMutation = useMutation({
    mutationFn: async (enabled: boolean) =>
      api.updateCatalogSyncConfig(enabled),
    onSuccess: (status) => {
      refreshStatus(status)
      toast.success(
        status.autoSync
          ? t('settings.proxyModels.catalogSources.toast.autoSyncOn')
          : t('settings.proxyModels.catalogSources.toast.autoSyncOff')
      )
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  const toggleEnabledMutation = useMutation({
    mutationFn: async (source: CatalogSource) =>
      api.updateCatalogSource(source.id, { enabled: !source.enabled }),
    onSuccess: (_result, source) => {
      const status = statusQuery.data
      if (status) {
        refreshStatus({
          ...status,
          sources: status.sources.map((row) =>
            row.id === source.id ? { ...row, enabled: !source.enabled } : row
          ),
        })
      }
      toast.success(t('settings.proxyModels.catalogSources.toast.updated'))
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  const reorderMutation = useMutation({
    mutationFn: async ({
      sourceId,
      targetIndex,
    }: {
      sourceId: number
      targetIndex: number
    }) => api.updateCatalogSource(sourceId, { sortOrder: targetIndex }),
    // Optimistic move: splice the row to its drop position so the drag feels
    // immediate; the backend repositions + renumbers sort_order server-side.
    onMutate: async ({ sourceId, targetIndex }) => {
      const status = statusQuery.data
      if (!status) return undefined
      const rows = [...status.sources]
      const from = rows.findIndex((row) => row.id === sourceId)
      if (from < 0) return undefined
      const [moved] = rows.splice(from, 1)
      rows.splice(Math.min(targetIndex, rows.length), 0, moved)
      refreshStatus({ ...status, sources: rows })
      return status
    },
    onError: (error: Error, _vars, context) => {
      if (context) refreshStatus(context)
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      )
    },
  })

  // Delete-with-undo tier: leaf single-row delete — no confirm dialog; the row
  // leaves immediately and a 6s undo toast gates the real DELETE.
  const deleteSource = (source: CatalogSource) =>
    undoableDelete<CatalogSyncStatus, CatalogSource>({
      item: source,
      queryKey: catalogSyncKeys.status(),
      removeFromCache: (data, item) => ({
        ...data,
        sources: data.sources.filter((row) => row.id !== item.id),
      }),
      deleteFn: (item) => api.deleteCatalogSource(item.id),
      title: t('settings.proxyModels.catalogSources.toast.deleted'),
      undoLabel: t('common.undo'),
      errorTitle: (error) =>
        t('settings.proxyModels.catalogSources.toast.deleteFailed', {
          message: error instanceof Error ? error.message : String(error),
        }),
    })

  return {
    statusQuery,
    syncingId,
    sync: syncMutation.mutate,
    autoSyncMutation,
    reloadSources: () =>
      queryClient.invalidateQueries({ queryKey: catalogSyncKeys.all }),
    sourceActions: {
      toggleEnabled: toggleEnabledMutation.mutate,
      toggling: toggleEnabledMutation.isPending,
      reorder: reorderMutation.mutate,
      reordering: reorderMutation.isPending,
      deleteSource,
    },
  }
}
