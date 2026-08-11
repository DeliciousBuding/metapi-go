// metapi-go/features/settings/sections/downstream/components — downstream
// keys section. A lean list + create sheet + enable/disable/delete actions.
// The legacy DownstreamKeys page (1500+ lines, rich editor, batch ops, trend
// charts) is intentionally reduced to its core here; richer surfaces can be
// layered back on as separate sub-features once the rewrite matures.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

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

export function KeysSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<DownstreamApiKeyItem | null>(
    null
  )

  const keysQuery = useQuery<DownstreamKeysResponse>({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })

  const createForm = useForm<CreateKeyFormValues>({
    resolver: zodResolver(createKeySchema) as never,
    defaultValues: {
      name: '',
      key: '',
      groupName: '',
      maxRequests: undefined,
      maxCost: undefined,
      enabled: true,
      expiresAt: '',
      description: '',
    },
  })

  function resetCreateForm() {
    createForm.reset({
      name: '',
      key: '',
      groupName: '',
      maxRequests: undefined,
      maxCost: undefined,
      enabled: true,
      expiresAt: '',
      description: '',
    })
  }

  function onCreateOpenChange(open: boolean) {
    setCreateOpen(open)
    if (open) {
      resetCreateForm()
    }
  }

  function generateKey() {
    createForm.setValue('key', `sk-${generateDownstreamSkSuffix()}`, {
      shouldDirty: true,
    })
  }

  const createMutation = useMutation({
    mutationFn: async (values: CreateKeyFormValues) =>
      api.createDownstreamApiKey(values),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.created'))
      onCreateOpenChange(false)
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.createFailed')),
  })

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

  function onCreateSubmit(values: CreateKeyFormValues) {
    createMutation.mutate(values)
  }

  const items = keysQuery.data?.items ?? []
  const isLoading = keysQuery.isLoading

  return (
    <SettingsSectionCard
      title={t('settings.downstream.keys.title')}
      description={t('settings.downstream.keys.description')}
      actions={
        <Button size='sm' onClick={() => onCreateOpenChange(true)}>
          {t('settings.downstream.keys.create')}
        </Button>
      }
    >
      {isLoading ? <SettingsSectionSkeleton /> : null}
      {!isLoading && items.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.downstream.keys.empty')}
        </p>
      ) : null}
      {!isLoading && items.length > 0 ? (
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
                    onCheckedChange={(checked) =>
                      toggleMutation.mutate({ id: item.id, enabled: checked })
                    }
                    aria-label={t('settings.downstream.keys.columns.enabled')}
                  />
                </TableCell>
                <TableCell className='text-muted-foreground text-xs'>
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
                </TableCell>
                <TableCell className='text-right'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={() => setDeleteTarget(item)}
                  >
                    {t('settings.common.delete')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}

      <Sheet open={createOpen} onOpenChange={onCreateOpenChange}>
        <SheetContent className='flex flex-col gap-4 overflow-y-auto'>
          <SheetHeader>
            <SheetTitle>{t('settings.downstream.keys.createTitle')}</SheetTitle>
            <SheetDescription>
              {t('settings.downstream.keys.createDescription')}
            </SheetDescription>
          </SheetHeader>
          <Form {...createForm}>
            <form
              id={CREATE_FORM_ID}
              onSubmit={createForm.handleSubmit(onCreateSubmit)}
              className='space-y-4'
            >
              <FormField
                control={createForm.control}
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
              <FormField
                control={createForm.control}
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
              <FormField
                control={createForm.control}
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
                  control={createForm.control}
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
                  control={createForm.control}
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
                control={createForm.control}
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
                control={createForm.control}
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
              <FormField
                control={createForm.control}
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
            </form>
          </Form>
          <SheetFooter>
            <Button
              type='submit'
              form={CREATE_FORM_ID}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending
                ? t('settings.common.saving')
                : t('settings.common.create')}
            </Button>
          </SheetFooter>
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
    </SettingsSectionCard>
  )
}
