// metapi-go/features/settings/sections/models/components — model-name
// redirects section (K1a). Table of canonical → actual mappings with
// generate / preview / apply / promote-to-manual / delete actions.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
  type ModelRedirectsResponse,
  type RedirectApplyResponse,
} from '@/lib/api'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

const modelRedirectsQueryKeys = {
  all: ['model-redirects'] as const,
  list: () => [...modelRedirectsQueryKeys.all, 'list'] as const,
}

type RedirectPreview = RedirectApplyResponse

export function RedirectsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

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
        t('settings.models.redirects.toast.generated', {
          created: result.created,
          accounts: result.accounts ?? 0,
        })
      )
    },
    onError: () =>
      toast.error(t('settings.models.redirects.toast.generateFailed')),
  })

  const applyMutation = useMutation({
    mutationFn: async (dryRun: boolean) => api.applyModelRedirects(dryRun),
    onSuccess: (result) => {
      if (result.dryRun) {
        const preview = result as RedirectPreview
        const candidateCount = preview.candidates?.length ?? 0
        if (candidateCount === 0) {
          toast.info(t('settings.models.redirects.toast.previewEmpty'))
        } else {
          toast.info(
            t('settings.models.redirects.toast.preview', {
              count: candidateCount,
            })
          )
        }
      } else {
        void queryClient.invalidateQueries({
          queryKey: modelRedirectsQueryKeys.all,
        })
        toast.success(
          t('settings.models.redirects.toast.applied', {
            removed: result.removed ?? 0,
          })
        )
      }
    },
    onError: () =>
      toast.error(t('settings.models.redirects.toast.applyFailed')),
  })

  const promoteMutation = useMutation({
    mutationFn: async (id: number) =>
      api.updateModelRedirect(id, { source: 'manual' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: modelRedirectsQueryKeys.all,
      })
      toast.success(t('settings.models.redirects.toast.promoted'))
    },
    onError: () =>
      toast.error(t('settings.models.redirects.toast.promoteFailed')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteModelRedirect(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: modelRedirectsQueryKeys.all,
      })
      toast.success(t('settings.models.redirects.toast.deleted'))
    },
    onError: () =>
      toast.error(t('settings.models.redirects.toast.deleteFailed')),
  })

  const items = redirectsQuery.data?.items ?? []
  const isLoading = redirectsQuery.isLoading

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  return (
    <SettingsSectionCard
      title={t('settings.models.redirects.title')}
      description={t('settings.models.redirects.description')}
      actions={
        <div className='flex gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={generateMutation.isPending}
            onClick={() => generateMutation.mutate()}
          >
            {t('settings.models.redirects.generate')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={applyMutation.isPending}
            onClick={() => applyMutation.mutate(true)}
          >
            {t('settings.models.redirects.preview')}
          </Button>
          <Button
            size='sm'
            disabled={applyMutation.isPending}
            onClick={() => applyMutation.mutate(false)}
          >
            {t('settings.models.redirects.apply')}
          </Button>
        </div>
      }
    >
      {items.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.models.redirects.empty')}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.models.redirects.columns.canonical')}
              </TableHead>
              <TableHead>
                {t('settings.models.redirects.columns.actual')}
              </TableHead>
              <TableHead>
                {t('settings.models.redirects.columns.account')}
              </TableHead>
              <TableHead>
                {t('settings.models.redirects.columns.source')}
              </TableHead>
              <TableHead className='text-right'>
                {t('settings.models.redirects.columns.actions')}
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
                    {t(`settings.models.redirects.source.${redirect.source}`)}
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
                        {t('settings.models.redirects.promote')}
                      </Button>
                    ) : null}
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      disabled={deleteMutation.isPending}
                      onClick={() => deleteMutation.mutate(redirect.id)}
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
    </SettingsSectionCard>
  )
}
