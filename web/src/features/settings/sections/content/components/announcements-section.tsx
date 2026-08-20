// metapi-go/features/settings/sections/content/components — risk-banner
// announcements section (H1). Inline CRUD list with create/edit/delete +
// severity select + enabled toggle. Content edits reset dismissals (the
// backend bumps the announcement revision on PUT).

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Badge } from '@/components/ui/badge'
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
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { api, type Announcement, type AnnouncementsResponse } from '@/lib/api'
import { toast } from '@/lib/toast'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

const announcementsQueryKeys = {
  all: ['announcements'] as const,
}

const announcementSchema = z.object({
  title: z
    .string()
    .min(1, 'settings.content.announcements.schema.titleRequired'),
  message: z
    .string()
    .min(1, 'settings.content.announcements.schema.messageRequired'),
  severity: z.enum(['info', 'warning', 'critical']),
  link: z.string().optional(),
  enabled: z.boolean().optional(),
})

type AnnouncementFormValues = z.infer<typeof announcementSchema>

type EditMode =
  | { kind: 'create' }
  | { kind: 'edit'; announcement: Announcement }
  | null

const EDIT_FORM_ID = 'settings-content-announcements-edit-form'

export function AnnouncementsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editMode, setEditMode] = useState<EditMode>(null)
  const [deleteTarget, setDeleteTarget] = useState<Announcement | null>(null)

  const announcementsQuery = useQuery<AnnouncementsResponse>({
    queryKey: announcementsQueryKeys.all,
    queryFn: async () => api.getAnnouncements(),
    staleTime: 30 * 1000,
  })

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
      if (mode === 'create') {
        return api.createAnnouncement({
          title: values.title,
          message: values.message,
          severity: values.severity,
          link: values.link || undefined,
          enabled: Boolean(values.enabled),
        })
      }
      const targetId = (editMode as { announcement: Announcement } | null)
        ?.announcement?.id
      if (!targetId) {
        throw new Error('No announcement selected for edit')
      }
      return api.updateAnnouncement(targetId, {
        title: values.title,
        message: values.message,
        severity: values.severity,
        link: values.link || undefined,
        enabled: Boolean(values.enabled),
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: announcementsQueryKeys.all,
      })
      toast.success(t('settings.content.announcements.toast.saved'))
      setEditMode(null)
    },
    onError: () =>
      toast.error(t('settings.content.announcements.toast.saveFailed')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteAnnouncement(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: announcementsQueryKeys.all,
      })
      toast.success(t('settings.content.announcements.toast.deleted'))
      setDeleteTarget(null)
    },
    onError: () =>
      toast.error(t('settings.content.announcements.toast.deleteFailed')),
  })

  function onSubmit(values: AnnouncementFormValues) {
    upsertMutation.mutate({
      mode: editMode?.kind === 'edit' ? 'edit' : 'create',
      values,
    })
  }

  // User-initiated closes (X / Escape / overlay / Cancel) are intercepted
  // while the form is dirty. The post-save path closes via setEditMode(null)
  // on purpose so a successful save never trips the discard prompt.
  const { handleOpenChange: guardedEditOpenChange, guard: editDirtyGuard } =
    useDirtyDialogClose({
      enabled: form.formState.isDirty,
      onDiscard: () => form.reset(),
      onOpenChange: (open) => {
        if (!open) setEditMode(null)
      },
    })

  const items = announcementsQuery.data?.items ?? []
  const isLoading = announcementsQuery.isLoading

  return (
    <SettingsSectionCard
      title={t('settings.content.announcements.title')}
      description={t('settings.content.announcements.description')}
      actions={
        <Button size='sm' onClick={() => setEditMode({ kind: 'create' })}>
          {t('settings.content.announcements.create')}
        </Button>
      }
    >
      {isLoading ? <SettingsSectionSkeleton /> : null}
      {!isLoading && items.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.content.announcements.empty')}
        </p>
      ) : null}
      {!isLoading && items.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.content.announcements.columns.severity')}
              </TableHead>
              <TableHead>
                {t('settings.content.announcements.columns.title')}
              </TableHead>
              <TableHead>
                {t('settings.content.announcements.columns.enabled')}
              </TableHead>
              <TableHead className='text-right'>
                {t('settings.content.announcements.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((announcement) => (
              <TableRow key={announcement.id}>
                <TableCell>
                  <Badge variant={severityVariant(announcement.severity)}>
                    {t(
                      `settings.content.announcements.severity.${announcement.severity}`
                    )}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div className='flex flex-col'>
                    <span className='font-medium'>{announcement.title}</span>
                    <span className='text-muted-foreground line-clamp-2 text-xs'>
                      {announcement.message}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    variant={announcement.enabled ? 'default' : 'secondary'}
                  >
                    {announcement.enabled
                      ? t('settings.common.enabled')
                      : t('settings.common.disabled')}
                  </Badge>
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() =>
                        setEditMode({ kind: 'edit', announcement })
                      }
                    >
                      {t('settings.common.edit')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => setDeleteTarget(announcement)}
                    >
                      {t('settings.common.delete')}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}

      <Dialog
        open={editMode !== null}
        onOpenChange={guardedEditOpenChange}
      >
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
              <FormField
                control={form.control}
                name='title'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.content.announcements.fields.title')}
                    </FormLabel>
                    <FormControl>
                      <Input {...field} value={field.value ?? ''} />
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
                    <FormLabel>
                      {t('settings.content.announcements.fields.message')}
                    </FormLabel>
                    <FormControl>
                      <Textarea {...field} value={field.value ?? ''} rows={4} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='severity'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.content.announcements.fields.severity')}
                    </FormLabel>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue>
                            {(selected) =>
                              selected
                                ? t(
                                    `settings.content.announcements.severity.${String(selected)}`
                                  )
                                : ''
                            }
                          </SelectValue>
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='info'>
                          {t('settings.content.announcements.severity.info')}
                        </SelectItem>
                        <SelectItem value='warning'>
                          {t('settings.content.announcements.severity.warning')}
                        </SelectItem>
                        <SelectItem value='critical'>
                          {t(
                            'settings.content.announcements.severity.critical'
                          )}
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
                    <FormLabel>
                      {t('settings.content.announcements.fields.link')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        placeholder='https://…'
                      />
                    </FormControl>
                    <FormDescription>
                      {t('settings.content.announcements.fields.linkHint')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center gap-3'>
                    <FormControl>
                      <Switch
                        checked={Boolean(field.value)}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel className='cursor-pointer'>
                      {t('settings.content.announcements.fields.enabled')}
                    </FormLabel>
                  </FormItem>
                )}
              />
            </form>
          </Form>
          <DialogFooter>
            <Button variant='outline' onClick={() => guardedEditOpenChange(false)}>
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
            <Button variant='outline' onClick={() => setDeleteTarget(null)}>
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
              {t('settings.common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSectionCard>
  )
}

function severityVariant(
  severity: Announcement['severity']
): 'default' | 'secondary' | 'destructive' {
  if (severity === 'critical') {
    return 'destructive'
  }
  if (severity === 'warning') {
    return 'default'
  }
  return 'secondary'
}
