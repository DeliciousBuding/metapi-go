import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/lib/toast'

import type { AccountToken } from '../../types'
import { useCreateAccountToken, useUpdateAccountToken } from '../api'
import {
  getAccountTokenFormDefaultValues,
  getAccountTokenFormSchema,
  transformTokenFormToPayload,
  type AccountTokenFormValues,
} from '../lib/tokens-schema'

interface AccountTokenFormProps {
  accountId: number
  token: AccountToken | null
  onClose: () => void
  onDirtyChange?: (dirty: boolean) => void
}

export function AccountTokenForm({
  accountId,
  token,
  onClose,
  onDirtyChange,
}: AccountTokenFormProps) {
  const { t } = useTranslation()
  const isEdit = !!token
  const createMutation = useCreateAccountToken()
  const updateMutation = useUpdateAccountToken()
  const [confirmDiscardOpen, setConfirmDiscardOpen] = useState(false)

  const schema = useMemo(() => getAccountTokenFormSchema(), [])
  const form = useForm<AccountTokenFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountTokenFormDefaultValues(accountId),
  })

  const unlimited = form.watch('unlimited')
  const createsUpstream = !isEdit && form.watch('token').trim() === ''

  useEffect(() => {
    if (token) {
      form.reset({
        accountId,
        name: token.name || '',
        token: '',
        tokenGroup: token.tokenGroup ?? 'default',
        quota: undefined,
        unlimited: true,
        expiresAt: '',
        allowedIps: '',
      })
    } else {
      form.reset(getAccountTokenFormDefaultValues(accountId))
    }
  }, [token, accountId, form])

  // Keep the hosting sheet's dirty flag in sync so it can confirm before
  // closing over unsaved token edits; clear on unmount to avoid stale state.
  const isFormDirty = form.formState.isDirty
  useEffect(() => {
    onDirtyChange?.(isFormDirty)
    return () => onDirtyChange?.(false)
  }, [isFormDirty, onDirtyChange])

  // The inline form has no overlay of its own — its only close affordance is
  // Cancel — so the discard confirmation lives here; the post-save close
  // calls onClose directly and never trips the prompt.
  function requestClose() {
    if (form.formState.isDirty) {
      setConfirmDiscardOpen(true)
      return
    }
    onClose()
  }

  const onSubmit = async (values: AccountTokenFormValues) => {
    const payload = transformTokenFormToPayload(values)
    try {
      if (isEdit && token) {
        await updateMutation.mutateAsync({
          id: token.id,
          payload: {
            name: payload.name,
            group: payload.group,
            ...(values.token ? { token: values.token } : {}),
          },
        })
      } else {
        await createMutation.mutateAsync(payload)
      }
      onClose()
    } catch {
      // http-client toasted
    }
  }

  const onInvalid: SubmitErrorHandler<AccountTokenFormValues> = () => {
    toast.error(t('accounts.tokens.form.invalid'))
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending

  return (
    <>
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit, onInvalid)}
          className='bg-muted/30 flex flex-col gap-3 rounded-lg border p-3'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('accounts.tokens.form.name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('accounts.tokens.form.namePlaceholder')}
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='token'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('accounts.tokens.form.value')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    className='font-mono text-xs'
                    placeholder={
                      isEdit
                        ? t('accounts.tokens.form.valuePlaceholder')
                        : t('accounts.tokens.form.createValuePlaceholder')
                    }
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {isEdit
                    ? t('accounts.tokens.form.valueHint')
                    : t('accounts.tokens.form.createValueHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid grid-cols-2 gap-3'>
            <FormField
              control={form.control}
              name='tokenGroup'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.tokens.form.group')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('accounts.tokens.form.groupPlaceholder')}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {createsUpstream && (
              <FormField
                control={form.control}
                name='quota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('accounts.tokens.form.quota')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        placeholder={t('accounts.tokens.form.quotaPlaceholder')}
                        disabled={unlimited}
                        value={field.value ?? ''}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? undefined
                              : Number(event.target.value)
                          )
                        }
                        onBlur={field.onBlur}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
          </div>

          {createsUpstream && (
            <>
              <FormField
                control={form.control}
                name='unlimited'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-2.5'>
                    <div className='space-y-0.5'>
                      <FormLabel>
                        {t('accounts.tokens.form.unlimited')}
                      </FormLabel>
                      <FormDescription>
                        {t('accounts.tokens.form.unlimitedHint')}
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

              <div className='grid grid-cols-2 gap-3'>
                <FormField
                  control={form.control}
                  name='expiresAt'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('accounts.tokens.form.expiresAt')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type='datetime-local'
                          {...field}
                          value={field.value ?? ''}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='allowedIps'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('accounts.tokens.form.allowedIps')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t(
                            'accounts.tokens.form.allowedIpsPlaceholder'
                          )}
                          {...field}
                          value={field.value ?? ''}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </>
          )}

          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={requestClose}
              disabled={isSubmitting}
            >
              {t('common.cancel')}
            </Button>
            <Button type='submit' size='sm' disabled={isSubmitting}>
              {isSubmitting && <Spinner className='size-3.5' />}
              {isEdit ? t('common.save') : t('accounts.tokens.form.create')}
            </Button>
          </div>
        </form>
      </Form>

      <ConfirmDialog
        open={confirmDiscardOpen}
        title={t('settings.common.unsavedTitle')}
        description={t('settings.common.unsavedDescription')}
        confirmLabel={t('settings.common.discardChanges')}
        cancelLabel={t('settings.common.keepEditing')}
        destructive
        onConfirm={() => {
          setConfirmDiscardOpen(false)
          form.reset()
          onClose()
        }}
        onCancel={() => setConfirmDiscardOpen(false)}
      />
    </>
  )
}
