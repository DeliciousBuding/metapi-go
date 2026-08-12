// metapi-go/features/site-announcements — add/edit form dialog
// (RHF + Zod + shadcn).
//
// One dialog serves both create and edit. The `editingAnnouncement` prop
// selects the mode; when null the dialog is in "add" mode. On edit, the
// form preserves the `dismissed` / `dismissedAt` flags by not touching them
// — the update payload only carries the editable fields (title / message /
// severity / link / enabled).

import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { useCreateAnnouncement, useUpdateAnnouncement } from '../api'
import {
  ANNOUNCEMENT_FORM_DEFAULT_VALUES,
  announcementFormSchema,
  type AnnouncementFormValues,
} from '../lib/announcements-schema'
import type {
  AnnouncementFormPayload,
  AnnouncementSeverity,
  SiteAnnouncement,
} from '../types'

type AnnouncementFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingAnnouncement: SiteAnnouncement | null
}

function announcementToFormValues(
  item: SiteAnnouncement,
): AnnouncementFormValues {
  return {
    title: item.title ?? '',
    message: item.message ?? '',
    severity: (item.severity as AnnouncementSeverity) ?? 'info',
    link: item.link ?? '',
    enabled: item.enabled ?? false,
  }
}

function buildPayload(values: AnnouncementFormValues): AnnouncementFormPayload {
  return {
    title: values.title,
    message: values.message,
    severity: values.severity,
    link: values.link.trim() || null,
    enabled: values.enabled,
  }
}

export function AnnouncementFormDialog({
  open,
  onOpenChange,
  editingAnnouncement,
}: AnnouncementFormDialogProps) {
  const { t } = useTranslation()
  const isEditing = editingAnnouncement !== null

  const form = useForm<AnnouncementFormValues>({
    resolver: zodResolver(announcementFormSchema),
    defaultValues: ANNOUNCEMENT_FORM_DEFAULT_VALUES,
  })

  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  const createAnnouncement = useCreateAnnouncement()
  const updateAnnouncement = useUpdateAnnouncement()

  useEffect(() => {
    if (!open) return
    if (editingAnnouncement) {
      form.reset(announcementToFormValues(editingAnnouncement))
    } else {
      form.reset(ANNOUNCEMENT_FORM_DEFAULT_VALUES)
    }
  }, [open, editingAnnouncement, form])

  const isSubmitting = createAnnouncement.isPending || updateAnnouncement.isPending

  async function onSubmit(values: AnnouncementFormValues) {
    const payload = buildPayload(values)
    try {
      if (isEditing && editingAnnouncement) {
        await updateAnnouncement.mutateAsync({
          id: editingAnnouncement.id,
          payload,
        })
        toast.success(t('siteAnnouncements.form.updateSucceeded', { title: values.title }))
        form.reset()
        onOpenChange(false)
      } else {
        await createAnnouncement.mutateAsync(payload)
        toast.success(t('siteAnnouncements.form.createSucceeded', { title: values.title }))
        form.reset()
        onOpenChange(false)
      }
    } catch {
      // http-client toasted
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {isEditing
              ? t('siteAnnouncements.form.editTitle')
              : t('siteAnnouncements.form.addTitle')}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? t('siteAnnouncements.form.editDescription')
              : t('siteAnnouncements.form.addDescription')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className='grid gap-4'>
            <FormField
              control={form.control}
              name='title'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('siteAnnouncements.form.title')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('siteAnnouncements.form.titlePlaceholder')}
                      autoFocus
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='message'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('siteAnnouncements.form.message')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('siteAnnouncements.form.messagePlaceholder')}
                      rows={4}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('siteAnnouncements.form.messageDescription')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='severity'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('siteAnnouncements.form.severity')}</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('siteAnnouncements.form.severityPlaceholder')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='info'>
                          {t('siteAnnouncements.severity.info')}
                        </SelectItem>
                        <SelectItem value='warning'>
                          {t('siteAnnouncements.severity.warning')}
                        </SelectItem>
                        <SelectItem value='critical'>
                          {t('siteAnnouncements.severity.critical')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='link'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('siteAnnouncements.form.link')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('siteAnnouncements.form.linkPlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('siteAnnouncements.form.linkDescription')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border border-border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('siteAnnouncements.form.enabled')}</FormLabel>
                    <FormDescription>
                      {t('siteAnnouncements.form.enabledDescription')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
                disabled={isSubmitting}
              >
                {t('siteAnnouncements.form.cancel')}
              </Button>
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting && <Spinner className='mr-1' />}
                {isEditing
                  ? t('siteAnnouncements.form.save')
                  : t('siteAnnouncements.form.create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
      {guard}
    </Dialog>
  )
}
