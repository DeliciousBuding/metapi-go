// metapi-go/features/settings/sections/proxy-models/components — model-name
// redirects section (K1a). Table of canonical → actual mappings with
// generate / preview / apply / promote-to-manual / delete actions.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  api,
  type ModelRedirect,
  type ModelRedirectsResponse,
  type RedirectApplyResponse,
} from '@/lib/api'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'

const modelRedirectsQueryKeys = {
  all: ['model-redirects'] as const,
  list: () => [...modelRedirectsQueryKeys.all, 'list'] as const,
}

type RedirectPreview = RedirectApplyResponse

export function RedirectsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const undoableDelete = useUndoableDelete()
  const [applyConfirmOpen, setApplyConfirmOpen] = useState(false)

  const redirectsQuery = useQuery<ModelRedirectsResponse>({
    queryKey: modelRedirectsQueryKeys.list(),
    queryFn: async () => api.getModelRedirects(),
    staleTime: 30 * 1000,
  })

  const generateMutation = useMutation({
    mutationFn: async () => api.generateModelRedirects(0),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({
        queryKey: modelRedirectsQueryKeys.all,
      })
      toast.success(
        t('settings.proxyModels.redirects.toast.generated', {
          created: result.created,
          accounts: result.accounts ?? 0,
        })
      )
    },
    onError: () =>
      toast.error(t('settings.proxyModels.redirects.toast.generateFailed')),
  })

  const applyMutation = useMutation({
    mutationFn: async (dryRun: boolean) => api.applyModelRedirects(dryRun),
    onSuccess: (result) => {
      if (result.dryRun) {
        const preview = result as RedirectPreview
        const candidateCount = preview.candidates?.length ?? 0
        if (candidateCount === 0) {
          toast.info(t('settings.proxyModels.redirects.toast.previewEmpty'))
        } else {
          toast.info(
            t('settings.proxyModels.redirects.toast.preview', {
              count: candidateCount,
            })
          )
        }
      } else {
        void queryClient.invalidateQueries({
          queryKey: modelRedirectsQueryKeys.all,
        })
        toast.success(
          t('settings.proxyModels.redirects.toast.applied', {
            removed: result.removed ?? 0,
          })
        )
      }
    },
    onError: () =>
      toast.error(t('settings.proxyModels.redirects.toast.applyFailed')),
  })

  const promoteMutation = useMutation({
    mutationFn: async (id: number) =>
      api.updateModelRedirect(id, { source: 'manual' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: modelRedirectsQueryKeys.all,
      })
      toast.success(t('settings.proxyModels.redirects.toast.promoted'))
    },
    onError: () =>
      toast.error(t('settings.proxyModels.redirects.toast.promoteFailed')),
  })

  // S7 删除+undo 档: leaf single-row delete — no confirm dialog; the row
  // leaves immediately and a 6s undo toast gates the real DELETE.
  const deleteRedirect = (redirect: ModelRedirect) =>
    undoableDelete<ModelRedirectsResponse, ModelRedirect>({
      item: redirect,
      queryKey: modelRedirectsQueryKeys.list(),
      removeFromCache: (data, item) => ({
        ...data,
        items: data.items.filter((entry) => entry.id !== item.id),
      }),
      deleteFn: (item) => api.deleteModelRedirect(item.id),
      title: t('settings.proxyModels.redirects.toast.deleted'),
      undoLabel: t('common.undo'),
      errorTitle: t('settings.proxyModels.redirects.toast.deleteFailed'),
    })

  const items = redirectsQuery.data?.items ?? []
  const isLoading = redirectsQuery.isLoading

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  if (redirectsQuery.isError || !redirectsQuery.data) {
    return (
      <SettingsSectionError
        title={t('settings.proxyModels.redirects.title')}
        onRetry={() => void redirectsQuery.refetch()}
      />
    )
  }

  return (
    <SettingsSectionCard
      title={t('settings.proxyModels.redirects.title')}
      description={t('settings.proxyModels.redirects.description')}
      actions={
        <div className='flex gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={generateMutation.isPending}
            onClick={() => generateMutation.mutate()}
          >
            {t('settings.proxyModels.redirects.generate')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={applyMutation.isPending}
            onClick={() => applyMutation.mutate(true)}
          >
            {t('settings.proxyModels.redirects.preview')}
          </Button>
          <Button
            size='sm'
            disabled={applyMutation.isPending}
            onClick={() => setApplyConfirmOpen(true)}
          >
            {t('settings.proxyModels.redirects.apply')}
          </Button>
        </div>
      }
    >
      {items.length === 0 ? (
        <div className='flex flex-col items-center gap-3 py-8 text-center'>
          <p className='text-muted-foreground text-sm'>
            {t('settings.proxyModels.redirects.empty')}
          </p>
          <Button
            size='sm'
            variant='outline'
            disabled={generateMutation.isPending}
            onClick={() => generateMutation.mutate()}
          >
            {t('settings.proxyModels.redirects.generate')}
          </Button>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.proxyModels.redirects.columns.canonical')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.redirects.columns.actual')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.redirects.columns.account')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.redirects.columns.source')}
              </TableHead>
              <TableHead className='text-right'>
                {t('settings.proxyModels.redirects.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((redirect: ModelRedirectsResponse['items'][number]) => (
              <TableRow key={redirect.id}>
                <TableCell className='font-mono text-xs'>
                  {redirect.canonical}
                </TableCell>
                <TableCell className='font-mono text-xs'>
                  {redirect.actual}
                </TableCell>
                <TableCell className='text-xs'>
                  <div>
                    {redirect.siteName ??
                      redirect.username ??
                      `#${redirect.accountId}`}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    variant={
                      redirect.source === 'manual' ? 'default' : 'secondary'
                    }
                  >
                    {t(
                      `settings.proxyModels.redirects.source.${redirect.source}`
                    )}
                  </Badge>
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-1'>
                    {redirect.source === 'sync' ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        disabled={promoteMutation.isPending}
                        onClick={() => promoteMutation.mutate(redirect.id)}
                      >
                        {t('settings.proxyModels.redirects.promote')}
                      </Button>
                    ) : null}
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => deleteRedirect(redirect)}
                    >
                      {t('settings.common.delete')}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <ConfirmDialog
        open={applyConfirmOpen}
        title={t('settings.proxyModels.redirects.applyTitle')}
        description={t('settings.proxyModels.redirects.applyDescription')}
        confirmLabel={t('settings.proxyModels.redirects.apply')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setApplyConfirmOpen(false)
          applyMutation.mutate(false)
        }}
        onCancel={() => setApplyConfirmOpen(false)}
      />
    </SettingsSectionCard>
  )
}
