import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { SectionCard } from '@/components/common/section-card'
import { SectionSkeleton } from '@/components/common/section-skeleton'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EmptyText } from '@/components/ui/empty'
import { Spinner } from '@/components/ui/spinner'
import { api, type Announcement, type AnnouncementsResponse } from '@/lib/api'
import { productAnnouncementKeys } from '@/lib/product-announcements'
import { toast } from '@/lib/toast'

import { AnnouncementEditDialog } from './announcement-edit-dialog'
import type { EditMode } from './announcement-form'
import { AnnouncementList } from './announcement-list'

export function AnnouncementsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editMode, setEditMode] = useState<EditMode>(null)
  const [deleteTarget, setDeleteTarget] = useState<Announcement | null>(null)

  const announcementsQuery = useQuery<AnnouncementsResponse>({
    queryKey: productAnnouncementKeys.list(),
    queryFn: async () => api.getAnnouncements(),
    staleTime: 30 * 1000,
  })

  const invalidateAnnouncementQueries = () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: productAnnouncementKeys.list(),
      }),
      queryClient.invalidateQueries({
        queryKey: productAnnouncementKeys.active(),
      }),
    ])

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteAnnouncement(id),
    onSuccess: () => {
      void invalidateAnnouncementQueries()
      toast.success(t('settings.content.announcements.toast.deleted'))
      setDeleteTarget(null)
    },
    onError: () =>
      toast.error(t('settings.content.announcements.toast.deleteFailed')),
  })

  const items = announcementsQuery.data?.items ?? []
  const isLoading = announcementsQuery.isLoading
  const loadError = announcementsQuery.error as Error | null

  return (
    <SectionCard
      title={t('settings.content.announcements.title')}
      description={t('settings.content.announcements.description')}
      hideHeaderCopy
      actions={
        <Button size='sm' onClick={() => setEditMode({ kind: 'create' })}>
          {t('settings.content.announcements.create')}
        </Button>
      }
    >
      {isLoading ? <SectionSkeleton /> : null}
      <QueryErrorBanner
        error={loadError}
        messageKey='settings.content.announcements.loadError'
        onRetry={() => void announcementsQuery.refetch()}
        isRetrying={announcementsQuery.isFetching}
      />
      {!isLoading && !loadError && items.length === 0 ? (
        <EmptyText>{t('settings.content.announcements.empty')}</EmptyText>
      ) : null}
      {!isLoading && items.length > 0 ? (
        <AnnouncementList
          items={items}
          onEdit={(announcement) => setEditMode({ kind: 'edit', announcement })}
          onDelete={setDeleteTarget}
        />
      ) : null}
      <AnnouncementEditDialog
        editMode={editMode}
        onClose={() => setEditMode(null)}
        invalidateAnnouncementQueries={invalidateAnnouncementQueries}
      />

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t('settings.content.announcements.deleteTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.content.announcements.deleteDescription', {
                title: deleteTarget?.title ?? '',
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeleteTarget(null)}
              disabled={deleteMutation.isPending}
            >
              {t('settings.common.cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.id)
                }
              }}
            >
              {deleteMutation.isPending && <Spinner />}
              {t('settings.common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
