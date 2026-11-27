import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
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
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import { userPortalApi } from '../api'
import type { ModelConnectionContext } from '../connection-context'
import { CreatedKey } from './portal-created-key'
import { PortalField } from './portal-field'

type CreatedPersonalKey = Awaited<ReturnType<typeof userPortalApi.createKey>>

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

function NewKeyForm({
  onCreated,
  onPending,
}: {
  onCreated: (created: CreatedPersonalKey) => void
  onPending: (pending: boolean) => void
}) {
  const { t } = useTranslation()
  const [failed, setFailed] = useState(false)
  const form = useForm<KeyFormValues>({
    resolver: zodResolver(keyFormSchema),
    defaultValues: { name: '', models: '', expiresAt: '', maxRequests: '' },
  })
  async function submit(values: KeyFormValues) {
    setFailed(false)
    onPending(true)
    try {
      const created = await userPortalApi.createKey({
        name: values.name,
        ...(values.models.trim()
          ? {
              supportedModels: values.models
                .split(/[\n,]+/)
                .map((v) => v.trim())
                .filter(Boolean),
            }
          : {}),
        ...(values.expiresAt
          ? { expiresAt: new Date(values.expiresAt).toISOString() }
          : {}),
        ...(values.maxRequests
          ? { maxRequests: Number(values.maxRequests) }
          : {}),
      })
      form.reset()
      onCreated(created)
    } catch {
      setFailed(true)
    } finally {
      onPending(false)
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

export function CreateKeyDialog({
  userId,
  connectionContext,
}: {
  userId: number
  connectionContext?: ModelConnectionContext
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState<CreatedPersonalKey | null>(null)
  const [createPending, setCreatePending] = useState(false)
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return (
    <>
      <Button disabled={createPending} onClick={() => setCreating(true)}>
        <Plus aria-hidden='true' />
        {t('userPortal.createKey')}
      </Button>
      <Dialog
        open={creating || created !== null}
        onOpenChange={(open) => {
          if (!open && !createPending) {
            setCreating(false)
            setCreated(null)
          }
        }}
      >
        <DialogContent showCloseButton={!createPending}>
          <DialogHeader>
            <DialogTitle>
              {t(created ? 'userPortal.keyReady' : 'userPortal.createKey')}
            </DialogTitle>
            <DialogDescription>
              {t(created ? 'userPortal.oneTimeKey' : 'userPortal.keyInherits')}
            </DialogDescription>
          </DialogHeader>
          {created ? (
            <CreatedKey
              secret={created.key}
              onDismiss={() => setCreated(null)}
              connection={
                connectionContext?.model || connectionContext?.invalid
                  ? {
                      userId,
                      context: connectionContext,
                      item: created.item,
                    }
                  : undefined
              }
            />
          ) : (
            <NewKeyForm
              onPending={(pending) => {
                if (mounted.current) setCreatePending(pending)
              }}
              onCreated={(value) => {
                if (!mounted.current) return
                setCreating(false)
                setCreated(value)
                void client.invalidateQueries({
                  queryKey: ['userPortal', userId, 'keys'],
                })
                void client.invalidateQueries({
                  queryKey: ['userPortal', userId, 'usage'],
                })
              }}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
