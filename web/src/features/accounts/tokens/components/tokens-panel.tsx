/* eslint-disable no-nested-ternary -- badge variant selection uses chained ternary */
// metapi-go features/accounts/tokens/components — tokens sub-panel embedded
// inside the account detail sheet (not a standalone page, matching the
// legacy metapi design). Shows the account's tokens + an inline add/edit
// token form (RHF + Zod) + sync-from-site action.

import { zodResolver } from '@hookform/resolvers/zod'
import { CheckCircle2, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Badge } from '@/components/ui/badge'
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
import { SecretField } from '@/components/ui/secret-field'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'
import { cn } from '@/lib/utils'

import { accountQueryKeys } from '../../api'
import type { AccountToken } from '../../types'
import {
  accountTokenQueryKeys,
  useAccountTokens,
  useCreateAccountToken,
  useSetDefaultAccountToken,
  useSyncAccountTokens,
  useToggleAccountTokenEnabled,
  useUpdateAccountToken,
} from '../api'
import {
  getAccountTokenFormDefaultValues,
  getAccountTokenFormSchema,
  transformTokenFormToPayload,
  type AccountTokenFormValues,
} from '../lib/tokens-schema'

interface TokensPanelProps {
  accountId: number
  /**
   * Reports the inline token form's dirty state so the hosting account
   * detail sheet can confirm before it closes over unsaved edits.
   */
  onFormDirtyChange?: (dirty: boolean) => void
}

export function TokensPanel({
  accountId,
  onFormDirtyChange,
}: TokensPanelProps) {
  const { t } = useTranslation()
  const { data: tokens = [], isLoading } = useAccountTokens(accountId)
  const syncMutation = useSyncAccountTokens()
  const setDefaultMutation = useSetDefaultAccountToken()
  const toggleEnabledMutation = useToggleAccountTokenEnabled()

  const [formOpen, setFormOpen] = useState(false)
  const [editingToken, setEditingToken] = useState<AccountToken | null>(null)

  // S7 删除+undo 档: token delete is leaf — no confirm dialog; the row
  // leaves immediately and a 6s undo toast gates the real DELETE.
  const undoableDelete = useUndoableDelete()
  const deleteToken = (token: AccountToken) =>
    undoableDelete<AccountToken[], AccountToken>({
      item: token,
      queryKey: accountTokenQueryKeys.list(accountId),
      removeFromCache: (data, target) =>
        data.filter((entry) => entry.id !== target.id),
      deleteFn: async (target) => {
        const result = await api.deleteAccountToken(target.id)
        assertBusinessOk(result, 'accounts.tokens.toast.deleteFailed')
      },
      title: t('accounts.tokens.toast.deleted'),
      undoLabel: t('common.undo'),
      errorTitle: t('accounts.tokens.toast.deleteFailed'),
      alsoInvalidate: [accountTokenQueryKeys.all, accountQueryKeys.all],
    })

  const openCreateForm = () => {
    setEditingToken(null)
    setFormOpen(true)
  }

  const openEditForm = (token: AccountToken) => {
    setEditingToken(token)
    setFormOpen(true)
  }

  const closeForm = () => {
    setFormOpen(false)
    setEditingToken(null)
  }

  const handleSync = async () => {
    try {
      await syncMutation.mutateAsync(accountId)
    } catch {
      // http-client toasted
    }
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center justify-between'>
        <h3 className='text-sm font-medium'>{t('accounts.tokens.title')}</h3>
        <div className='flex items-center gap-1'>
          <Button
            variant='outline'
            size='xs'
            onClick={handleSync}
            disabled={syncMutation.isPending}
          >
            {syncMutation.isPending ? (
              <Spinner className='size-3' />
            ) : (
              <RefreshCw />
            )}
            {t('accounts.tokens.syncFromSite')}
          </Button>
          <Button size='xs' onClick={openCreateForm}>
            <Plus />
            {t('accounts.tokens.add')}
          </Button>
        </div>
      </div>

      {formOpen && (
        <AccountTokenForm
          accountId={accountId}
          token={editingToken}
          onClose={closeForm}
          onDirtyChange={onFormDirtyChange}
        />
      )}

      <Separator />

      {isLoading ? (
        <div className='text-muted-foreground flex items-center justify-center py-6 text-sm'>
          <Spinner />
          {t('accounts.tokens.loading')}
        </div>
      ) : tokens.length === 0 ? (
        <p className='text-muted-foreground py-6 text-center text-sm'>
          {t('accounts.tokens.empty')}
        </p>
      ) : (
        <ul className='flex flex-col divide-y rounded-lg border'>
          {tokens.map((token) => (
            <TokenRow
              key={token.id}
              token={token}
              onEdit={() => openEditForm(token)}
              onDelete={() => deleteToken(token)}
              onSetDefault={() => setDefaultMutation.mutate(token.id)}
              onToggleEnabled={(enabled) =>
                toggleEnabledMutation.mutate({ id: token.id, enabled })
              }
              isToggling={toggleEnabledMutation.isPending}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Token row
// ---------------------------------------------------------------------------

interface TokenRowProps {
  token: AccountToken
  onEdit: () => void
  onDelete: () => void
  onSetDefault: () => void
  onToggleEnabled: (enabled: boolean) => void
  isToggling: boolean
}

function TokenRow({
  token,
  onEdit,
  onDelete,
  onSetDefault,
  onToggleEnabled,
  isToggling,
}: TokenRowProps) {
  const { t } = useTranslation()
  const isMaskedPending = token.valueStatus === 'masked_pending'
  return (
    <li className='flex items-center gap-3 px-3 py-2'>
      <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
        <div className='flex items-center gap-1.5'>
          <span className='truncate text-sm font-medium'>
            {token.name || t('accounts.tokens.unnamed')}
          </span>
          {token.isDefault && (
            <Badge variant='default' className='text-[10px]'>
              {t('accounts.tokens.default')}
            </Badge>
          )}
          {isMaskedPending && (
            // "Pending" alone reads as "wait", which is what an operator did
            // while the token stayed unusable: Metapi only ever received the
            // upstream's masked display value, and the fix is the sync action
            // that already sits on this account, not time.
            <Badge
              variant='warning'
              className='text-[10px]'
              title={t('accounts.tokens.pendingCompleteHint')}
            >
              {t('accounts.tokens.pendingComplete')}
            </Badge>
          )}
        </div>
        <SecretField value={token.token} masked={token.tokenMasked} />
      </div>

      <div className='flex items-center gap-1'>
        {!token.isDefault && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onSetDefault}
            title={t('accounts.tokens.setAsDefault')}
            aria-label={t('accounts.tokens.setAsDefault')}
          >
            <CheckCircle2 />
          </Button>
        )}
        <Switch
          checked={token.enabled ?? false}
          onCheckedChange={onToggleEnabled}
          disabled={isToggling}
          aria-label={t('accounts.tokens.toggleEnabled')}
        />
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onEdit}
          title={t('accounts.tokens.edit')}
          aria-label={t('accounts.tokens.edit')}
        >
          <Pencil />
        </Button>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onDelete}
          className={cn('text-muted-foreground hover:text-destructive')}
          title={t('accounts.tokens.delete')}
          aria-label={t('accounts.tokens.delete')}
        >
          <Trash2 />
        </Button>
      </div>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Inline add/edit token form
// ---------------------------------------------------------------------------

interface AccountTokenFormProps {
  accountId: number
  token: AccountToken | null
  onClose: () => void
  onDirtyChange?: (dirty: boolean) => void
}

function AccountTokenForm({
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
            tokenGroup: payload.tokenGroup,
            quota: payload.quota,
            remainQuota: payload.remainQuota,
            unlimitedQuota: payload.unlimitedQuota,
            expiredTime: payload.expiredTime,
            allowedIps: payload.allowedIps,
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
                        : 'sk-...'
                    }
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {isEdit ? t('accounts.tokens.form.valueHint') : undefined}
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
          </div>

          <FormField
            control={form.control}
            name='unlimited'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-2.5'>
                <div className='space-y-0.5'>
                  <FormLabel>{t('accounts.tokens.form.unlimited')}</FormLabel>
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
                  <FormLabel>{t('accounts.tokens.form.expiresAt')}</FormLabel>
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
                  <FormLabel>{t('accounts.tokens.form.allowedIps')}</FormLabel>
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
