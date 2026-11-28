import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'
import { getSafeProductAnnouncementUrl } from '@/lib/product-announcements'
import { toast } from '@/lib/toast'

import {
  announcementSchema,
  type AnnouncementFormValues,
  type EditMode,
} from './announcement-form'
import { AnnouncementFormFields } from './announcement-form-fields'

const EDIT_FORM_ID = 'settings-content-announcements-edit-form'

export function AnnouncementEditDialog({
  editMode,
  onClose,
  invalidateAnnouncementQueries,
}: {
  editMode: EditMode
  onClose: () => void
  invalidateAnnouncementQueries: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const form = useForm<AnnouncementFormValues>({
    resolver: zodResolver(announcementSchema) as never,
    defaultValues: {
      title: '',
      message: '',
      severity: 'info',
      link: '',
      enabled: true,
    },
  })

  useEffect(() => {
    if (editMode?.kind === 'edit') {
      const announcement = editMode.announcement
      form.reset({
        title: announcement.title,
        message: announcement.message,
        severity: announcement.severity,
        link: announcement.link ?? '',
        enabled: announcement.enabled,
      })
    } else if (editMode?.kind === 'create') {
      form.reset({
        title: '',
        message: '',
        severity: 'info',
        link: '',
        enabled: true,
      })
    }
  }, [editMode, form])

  const upsertMutation = useMutation({
    mutationFn: async ({
      mode,
      values,
    }: {
      mode: 'create' | 'edit'
      values: AnnouncementFormValues
    }) => {
      const link = getSafeProductAnnouncementUrl(values.link) ?? undefined
      if (mode === 'create') {
        return api.createAnnouncement({
          title: values.title,
          message: values.message,
          severity: values.severity,
          link,
          enabled: Boolean(values.enabled),
        })
      }
      const targetId =
        editMode?.kind === 'edit' ? editMode.announcement.id : undefined
      if (!targetId) {
        throw new Error('No announcement selected for edit')
      }
      return api.updateAnnouncement(targetId, {
        title: values.title,
        message: values.message,
        severity: values.severity,
        link,
        enabled: Boolean(values.enabled),
      })
    },
    onSuccess: () => {
      void invalidateAnnouncementQueries()
      toast.success(t('settings.content.announcements.toast.saved'))
      onClose()
    },
    onError: () =>
      toast.error(t('settings.content.announcements.toast.saveFailed')),
  })

  function onSubmit(values: AnnouncementFormValues) {
    upsertMutation.mutate({
      mode: editMode?.kind === 'edit' ? 'edit' : 'create',
      values,
    })
  }

  // User-initiated closes (X / Escape / overlay / Cancel) are intercepted
  // while the form is dirty. The post-save path closes via onClose()
  // on purpose so a successful save never trips the discard prompt.
  const { handleOpenChange: guardedEditOpenChange, guard: editDirtyGuard } =
    useDirtyDialogClose({
      enabled: form.formState.isDirty,
      onDiscard: () => form.reset(),
      onOpenChange: (open) => {
        if (!open) onClose()
      },
    })

  return (
    <Dialog open={editMode !== null} onOpenChange={guardedEditOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {editMode?.kind === 'edit'
              ? t('settings.content.announcements.editTitle')
              : t('settings.content.announcements.createTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('settings.content.announcements.editDescription')}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id={EDIT_FORM_ID}
            onSubmit={form.handleSubmit(onSubmit)}
            className='space-y-4'
          >
            <AnnouncementFormFields control={form.control} />
          </form>
        </Form>
        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => guardedEditOpenChange(false)}
            disabled={upsertMutation.isPending}
          >
            {t('settings.common.cancel')}
          </Button>
          <Button
            type='submit'
            form={EDIT_FORM_ID}
            disabled={upsertMutation.isPending}
          >
            {upsertMutation.isPending
              ? t('settings.common.saving')
              : t('settings.common.save')}
          </Button>
        </DialogFooter>
        {editDirtyGuard}
      </DialogContent>
    </Dialog>
  )
}
