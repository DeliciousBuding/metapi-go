// metapi-go/features/settings/sections/downstream/components — downstream
// keys section. A lean list + create sheet + enable/disable/delete actions.
// The legacy DownstreamKeys page (1500+ lines, rich editor, batch ops, trend
// charts) is intentionally reduced to its core here; richer surfaces can be
// layered back on as separate sub-features once the rewrite matures.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  CredentialExportDialog,
  type CredentialExportTarget,
} from '@/components/common/credential-export-dialog'
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'

type DownstreamKeyUsage24h = {
  requests?: number
  tokens?: number
  cost?: number
}

type DownstreamApiKeyItem = {
  id: number
  name: string
  keyMasked?: string
  groupName?: string
  enabled: boolean
  expiresAt?: string | null
  maxCost?: number | null
  usedCost?: number | null
  maxRequests?: number | null
  usedRequests?: number | null
  usage24h?: DownstreamKeyUsage24h
}

type DownstreamKeysResponse = { items: DownstreamApiKeyItem[] }

const downstreamKeysQueryKeys = {
  all: ['downstream-keys'] as const,
  list: () => [...downstreamKeysQueryKeys.all, 'list'] as const,
}

const CREATE_FORM_ID = 'settings-downstream-keys-create-form'

function generateDownstreamSkSuffix(): string {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    ''
  )
}

const createKeySchema = z.object({
  name: z.string().min(1, 'settings.downstream.keys.schema.nameRequired'),
  key: z.string().min(8, 'settings.downstream.keys.schema.keyMinLength'),
  groupName: z.string().optional(),
  maxRequests: z.coerce.number().int().min(0).optional(),
  maxCost: z.coerce.number().min(0).optional(),
  enabled: z.boolean().optional(),
  expiresAt: z.string().optional(),
  description: z.string().optional(),
})

type CreateKeyFormValues = z.infer<typeof createKeySchema>

// Edit mode reuses the create schema but drops the secret `key` field — the
// key value is never editable here (rotation is a separate action). The
// backend applies a PATCH-style partial update (only fields present in the
// request body are changed; the toggle path already relies on this), so the
// edit payload omits `key` and `description` to preserve both as-is.
const editKeySchema = createKeySchema.omit({ key: true })

// datetime-local input format: "YYYY-MM-DDTHH:MM" in the browser's local
// timezone. The backend stores the value as-is when it cannot parse it as
// RFC3339, so create/edit round-trip the same local string.
function isoToLocalDatetimeInput(iso?: string | null): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function blankKeyFormValues(): CreateKeyFormValues {
  return {
    name: '',
    key: '',
    groupName: '',
    maxRequests: undefined,
    maxCost: undefined,
    enabled: true,
    expiresAt: '',
    description: '',
  }
}

function keyFormValuesFromItem(
  item: DownstreamApiKeyItem
): CreateKeyFormValues {
  return {
    ...blankKeyFormValues(),
    name: item.name,
    groupName: item.groupName ?? '',
    maxRequests: item.maxRequests ?? undefined,
    maxCost: item.maxCost ?? undefined,
    enabled: item.enabled,
    expiresAt: isoToLocalDatetimeInput(item.expiresAt),
  }
}

// Exported so the edit-mode behavior test can render it in isolation,
// mirroring the AccountsRowActions export pattern. The whole KeysSection
// remains the only public entry in the settings surface.
export function KeySheetForm({
  editingKey,
  onDone,
}: {
  editingKey: DownstreamApiKeyItem | null
  onDone: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = editingKey !== null

  const form = useForm<CreateKeyFormValues>({
    resolver: zodResolver(
      editingKey ? editKeySchema : createKeySchema
    ) as never,
    defaultValues: editingKey
      ? keyFormValuesFromItem(editingKey)
      : blankKeyFormValues(),
  })

  function generateKey() {
    form.setValue('key', `sk-${generateDownstreamSkSuffix()}`, {
      shouldDirty: true,
    })
  }

  const submitMutation = useMutation({
    mutationFn: async (values: CreateKeyFormValues) => {
      if (!editingKey) {
        return api.createDownstreamApiKey(values)
      }
      return api.updateDownstreamApiKey(editingKey.id, {
        name: values.name,
        groupName: values.groupName,
        maxRequests: values.maxRequests,
        maxCost: values.maxCost,
        enabled: values.enabled,
        expiresAt: values.expiresAt,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(
        isEdit
          ? t('settings.downstream.keys.toast.updated')
          : t('settings.downstream.keys.toast.created')
      )
      onDone()
    },
    onError: () =>
      toast.error(
        isEdit
          ? t('settings.downstream.keys.toast.updateFailed')
          : t('settings.downstream.keys.toast.createFailed')
      ),
  })

  function onSubmit(values: CreateKeyFormValues) {
    submitMutation.mutate(values)
  }

  function resolveSubmitLabel(): string {
    if (submitMutation.isPending) return t('settings.common.saving')
    if (isEdit) return t('settings.common.save')
    return t('settings.common.create')
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>
          {isEdit
            ? t('settings.downstream.keys.editTitle')
            : t('settings.downstream.keys.createTitle')}
        </SheetTitle>
        <SheetDescription>
          {isEdit
            ? t('settings.downstream.keys.editDescription')
            : t('settings.downstream.keys.createDescription')}
        </SheetDescription>
      </SheetHeader>
      <Form {...form}>
        <form
          id={CREATE_FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.name')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {isEdit ? null : (
            <FormField
              control={form.control}
              name='key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.key')}
                  </FormLabel>
                  <div className='flex gap-2'>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        className='font-mono'
                        placeholder='sk-…'
                      />
                    </FormControl>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={generateKey}
                    >
                      {t('settings.downstream.keys.generate')}
                    </Button>
                  </div>
                  <FormDescription>
                    {t('settings.downstream.keys.fields.keyHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <FormField
            control={form.control}
            name='groupName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.groupName')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='grid grid-cols-2 gap-4'>
            <FormField
              control={form.control}
              name='maxRequests'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.maxRequests')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      placeholder={t('settings.common.unlimited')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='maxCost'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.maxCost')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      step={0.01}
                      placeholder={t('settings.common.unlimited')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormField
            control={form.control}
            name='expiresAt'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.expiresAt')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    type='datetime-local'
                  />
                </FormControl>
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
                  {t('settings.downstream.keys.fields.enabled')}
                </FormLabel>
              </FormItem>
            )}
          />
          {isEdit ? null : (
            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.description')}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
        </form>
      </Form>
      <SheetFooter>
        <Button
          type='submit'
          form={CREATE_FORM_ID}
          disabled={submitMutation.isPending}
        >
          {resolveSubmitLabel()}
        </Button>
      </SheetFooter>
    </>
  )
}

// Exported so the usage-cell test can render it in isolation (same pattern as
// KeySheetForm). Renders quota usage plus the per-key 24h proxy_logs summary.
export function KeyUsageCell({ item }: { item: DownstreamApiKeyItem }) {
  const { t } = useTranslation()
  return (
    <div>
      <div>
        {t('settings.downstream.keys.requests', {
          used: item.usedRequests ?? 0,
          max: item.maxRequests ?? t('settings.common.unlimited'),
        })}
      </div>
      <div>
        {t('settings.downstream.keys.cost', {
          used: item.usedCost ?? 0,
          max: item.maxCost ?? t('settings.common.unlimited'),
        })}
      </div>
      <div className='mt-1 border-t pt-1'>
        {t('settings.downstream.keys.usage24h', {
          requests: item.usage24h?.requests ?? 0,
          tokens: item.usage24h?.tokens ?? 0,
          cost: item.usage24h?.cost ?? 0,
        })}
      </div>
    </div>
  )
}

export function KeysSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<DownstreamApiKeyItem | null>(
    null
  )
  const [exportTarget, setExportTarget] =
    useState<CredentialExportTarget | null>(null)

  const keysQuery = useQuery<DownstreamKeysResponse>({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })

  const [editingKey, setEditingKey] = useState<DownstreamApiKeyItem | null>(
    null
  )

  function openCreate() {
    setEditingKey(null)
    setCreateOpen(true)
  }

  function openEdit(item: DownstreamApiKeyItem) {
    setEditingKey(item)
    setCreateOpen(true)
  }

  function onSheetOpenChange(open: boolean) {
    setCreateOpen(open)
    if (!open) {
      setEditingKey(null)
    }
  }

  const toggleMutation = useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.updateDownstreamApiKey(id, { enabled }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.updated'))
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.updateFailed')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteDownstreamApiKey(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.deleted'))
      setDeleteTarget(null)
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.deleteFailed')),
  })

  const items = keysQuery.data?.items ?? []
  const isLoading = keysQuery.isLoading

  return (
    <SettingsSectionCard
      title={t('settings.downstream.keys.title')}
      description={t('settings.downstream.keys.description')}
      actions={
        <Button size='sm' onClick={() => openCreate()}>
          {t('settings.downstream.keys.create')}
        </Button>
      }
    >
      {isLoading ? <SettingsSectionSkeleton /> : null}
      {keysQuery.isError ? (
        <SettingsSectionError
          title={t('settings.downstream.keys.title')}
          onRetry={() => void keysQuery.refetch()}
        />
      ) : null}
      {!isLoading && !keysQuery.isError && items.length === 0 ? (
        <div className='flex flex-col items-center gap-3 py-8'>
          <p className='text-muted-foreground text-sm'>
            {t('settings.downstream.keys.empty')}
          </p>
          <Button size='sm' onClick={() => openCreate()}>
            {t('settings.downstream.keys.create')}
          </Button>
        </div>
      ) : null}
      {!isLoading && !keysQuery.isError && items.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.downstream.keys.columns.name')}
              </TableHead>
              <TableHead>
                {t('settings.downstream.keys.columns.group')}
              </TableHead>
              <TableHead>
                {t('settings.downstream.keys.columns.enabled')}
              </TableHead>
              <TableHead>
                {t('settings.downstream.keys.columns.usage')}
              </TableHead>
              <TableHead className='text-right'>
                {t('settings.downstream.keys.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => (
              <TableRow key={item.id}>
                <TableCell>
                  <div className='flex flex-col'>
                    <span className='font-medium'>{item.name}</span>
                    {item.keyMasked ? (
                      <code className='text-muted-foreground text-xs'>
                        {item.keyMasked}
                      </code>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell>
                  {item.groupName ? (
                    <Badge variant='secondary'>{item.groupName}</Badge>
                  ) : (
                    <span className='text-muted-foreground'>—</span>
                  )}
                </TableCell>
                <TableCell>
                  <Switch
                    checked={item.enabled}
                    disabled={toggleMutation.isPending}
                    onCheckedChange={(checked) =>
                      toggleMutation.mutate({ id: item.id, enabled: checked })
                    }
                    aria-label={t(
                      'settings.downstream.keys.columns.enabledAria',
                      { name: item.name }
                    )}
                  />
                </TableCell>
                <TableCell className='text-muted-foreground text-xs'>
                  <KeyUsageCell item={item} />
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => setExportTarget(item)}
                    >
                      {t('settings.downstream.keys.connect')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      aria-label={t(
                        'settings.downstream.keys.columns.editAria',
                        { name: item.name }
                      )}
                      onClick={() => openEdit(item)}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => setDeleteTarget(item)}
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

      <Sheet open={createOpen} onOpenChange={onSheetOpenChange}>
        <SheetContent className='flex flex-col gap-4 overflow-y-auto'>
          <KeySheetForm
            key={editingKey?.id ?? 'create'}
            editingKey={editingKey}
            onDone={() => onSheetOpenChange(false)}
          />
        </SheetContent>
      </Sheet>

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
              {t('settings.downstream.keys.deleteTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.downstream.keys.deleteDescription', {
                name: deleteTarget?.name ?? '',
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

      <CredentialExportDialog
        target={exportTarget}
        onOpenChange={(open) => {
          if (!open) setExportTarget(null)
        }}
      />
    </SettingsSectionCard>
  )
}
