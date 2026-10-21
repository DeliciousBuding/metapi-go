import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Eye, EyeOff, KeyRound, Plus } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { type PersonalKey, userPortalApi } from '../api'
import { PortalField } from './portal-field'

const keyFormSchema = z.object({
  name: z.string().trim().min(1, 'userPortal.required').max(100),
  models: z.string().max(8000),
  expiresAt: z
    .string()
    .refine(
      (v) =>
        !v ||
        (Number.isFinite(new Date(v).getTime()) &&
          new Date(v).getTime() > Date.now()),
      'userPortal.expiryInvalid'
    ),
  maxRequests: z
    .string()
    .refine(
      (v) =>
        !v ||
        (/^\d+$/.test(v) && Number.isSafeInteger(Number(v)) && Number(v) > 0),
      'userPortal.requestLimitInvalid'
    ),
})
type KeyFormValues = z.infer<typeof keyFormSchema>

function NewKeyForm({ onCreated }: { onCreated: (key: string) => void }) {
  const { t } = useTranslation()
  const [failed, setFailed] = useState(false)
  const form = useForm<KeyFormValues>({
    resolver: zodResolver(keyFormSchema),
    defaultValues: { name: '', models: '', expiresAt: '', maxRequests: '' },
  })
  async function submit(values: KeyFormValues) {
    setFailed(false)
    try {
      const created = await userPortalApi.createKey({
        name: values.name,
        supportedModels: values.models
          .split(/[\n,]+/)
          .map((v) => v.trim())
          .filter(Boolean),
        ...(values.expiresAt
          ? { expiresAt: new Date(values.expiresAt).toISOString() }
          : {}),
        ...(values.maxRequests
          ? { maxRequests: Number(values.maxRequests) }
          : {}),
      })
      form.reset()
      onCreated(created.key)
    } catch {
      setFailed(true)
    }
  }
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} className='grid gap-4'>
        {failed && (
          <Notice tone='destructive' role='alert'>
            {t('userPortal.actionFailed')}
          </Notice>
        )}
        <PortalField
          control={form.control}
          name='name'
          label='userPortal.keyName'
          autoComplete='off'
        />
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.keyInherits')}
        </p>
        <details className='space-y-4'>
          <summary className='cursor-pointer text-sm font-medium'>
            {t('userPortal.keyRestrictions')}
          </summary>
          <div className='grid gap-4 pt-2'>
            <FormField
              control={form.control}
              name='models'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('userPortal.modelLimits')}</FormLabel>
                  <FormControl>
                    <Textarea {...field} rows={3} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <PortalField
              control={form.control}
              name='expiresAt'
              label='userPortal.expiresAt'
              type='datetime-local'
            />
            <PortalField
              control={form.control}
              name='maxRequests'
              label='userPortal.maxRequests'
              type='number'
            />
          </div>
        </details>
        <Button type='submit' disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting && <Spinner />}
          {t('userPortal.createKey')}
        </Button>
      </form>
    </Form>
  )
}

export function CopyValue({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      variant='outline'
      size='sm'
      onClick={async () => {
        if (await copyText(value)) toast.success(t('userPortal.copied'))
        else toast.error(t('userPortal.copyFailed'))
      }}
    >
      <Copy aria-hidden='true' />
      {label}
    </Button>
  )
}

function CreatedKey({
  secret,
  onDismiss,
}: {
  secret: string
  onDismiss: () => void
}) {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  const endpoint = window.location.origin + '/v1'
  return (
    <div className='grid gap-4'>
      <Notice tone='warning'>{t('userPortal.oneTimeKey')}</Notice>
      <div className='space-y-2'>
        <Label htmlFor='created-personal-key'>
          {t('userPortal.secretKey')}
        </Label>
        <div className='flex gap-2'>
          <Input
            id='created-personal-key'
            type={visible ? 'text' : 'password'}
            readOnly
            value={secret}
            className='font-mono'
          />
          <Button
            type='button'
            variant='outline'
            size='icon'
            aria-label={t(
              visible ? 'userPortal.hideKey' : 'userPortal.showKey'
            )}
            onClick={() => setVisible(!visible)}
          >
            {visible ? <EyeOff /> : <Eye />}
          </Button>
        </div>
        <CopyValue value={secret} label={t('userPortal.copyKey')} />
      </div>
      <div className='space-y-2'>
        <Label htmlFor='personal-endpoint'>{t('userPortal.endpoint')}</Label>
        <Input id='personal-endpoint' readOnly value={endpoint} />
        <CopyValue value={endpoint} label={t('userPortal.copyEndpoint')} />
      </div>
      <p className='text-muted-foreground text-sm'>
        {t('userPortal.connectHint')}
      </p>
      <Button onClick={onDismiss}>{t('userPortal.savedKey')}</Button>
    </div>
  )
}

export function PortalKeys({ userId }: { userId: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const queryKey = ['userPortal', userId, 'keys'] as const
  const query = useQuery({ queryKey, queryFn: userPortalApi.keys })
  const remove = useUndoableDelete()
  const [creating, setCreating] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.keysHint')}
        </p>
        <Button onClick={() => setCreating(true)}>
          <Plus aria-hidden='true' />
          {t('userPortal.createKey')}
        </Button>
      </div>
      {query.isPending ? (
        <Spinner aria-label={t('common.loading')} />
      ) : query.isError ? (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      ) : query.data.items.length === 0 ? (
        <Card>
          <CardContent className='text-muted-foreground flex items-center gap-3 py-4'>
            <KeyRound aria-hidden='true' />
            {t('userPortal.noKeys')}
          </CardContent>
        </Card>
      ) : (
        <div className='divide-border divide-y rounded-xl border'>
          {query.data.items.map((key) => (
            <div
              key={key.id}
              className='flex flex-wrap items-center justify-between gap-3 p-4'
            >
              <div className='min-w-0 space-y-1'>
                <p className='font-medium wrap-anywhere'>{key.name}</p>
                <p className='text-muted-foreground font-mono text-sm'>
                  {key.keyMasked}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    key.enabled ? 'userPortal.enabled' : 'userPortal.disabled'
                  )}
                  {' · '}
                  {t('userPortal.requestCount', { count: key.usedRequests })}
                  {' · '}
                  {key.expiresAt
                    ? new Date(key.expiresAt).toLocaleString()
                    : t('userPortal.noExpiry')}
                </p>
                {key.supportedModels.length > 0 && (
                  <p className='text-muted-foreground text-xs wrap-anywhere'>
                    {key.supportedModels.join(', ')}
                  </p>
                )}
              </div>
              <Button
                variant='destructive'
                size='sm'
                onClick={() =>
                  remove<{ items: PersonalKey[] }, PersonalKey>({
                    item: key,
                    queryKey,
                    removeFromCache: (data, item) => ({
                      items: data.items.filter((v) => v.id !== item.id),
                    }),
                    deleteFn: (item) => userPortalApi.deleteKey(item.id),
                    title: t('userPortal.keyRemoved'),
                    undoLabel: t('common.undo'),
                    errorTitle: t('userPortal.actionFailed'),
                    alsoInvalidate: [['userPortal', userId, 'usage']],
                  })
                }
              >
                {t('userPortal.revokeKey')}
              </Button>
            </div>
          ))}
        </div>
      )}
      <Dialog
        open={creating || secret !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false)
            setSecret(null)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t(secret ? 'userPortal.keyReady' : 'userPortal.createKey')}
            </DialogTitle>
            <DialogDescription>
              {t(secret ? 'userPortal.oneTimeKey' : 'userPortal.keyInherits')}
            </DialogDescription>
          </DialogHeader>
          {secret ? (
            <CreatedKey secret={secret} onDismiss={() => setSecret(null)} />
          ) : (
            <NewKeyForm
              onCreated={(key) => {
                setCreating(false)
                setSecret(key)
                void client.invalidateQueries({ queryKey })
                void client.invalidateQueries({
                  queryKey: ['userPortal', userId, 'usage'],
                })
              }}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
