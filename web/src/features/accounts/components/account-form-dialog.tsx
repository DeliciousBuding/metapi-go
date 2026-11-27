import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { useSiteInitializationPresets } from '@/features/sites'
import { getPlatformDefinition } from '@/lib/platform-catalog'
import { toast } from '@/lib/toast'

import {
  resolveCreatedAccountId,
  useCreateAccount,
  useLoginAccount,
  useUpdateAccount,
} from '../api'
import {
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import { findAccountInitializationPreset } from '../lib/initialization-preset'
import type { Account, CredentialMode, Site } from '../types'
import {
  showAccountCreatedToast,
  showAccountLoginToast,
} from './account-created-toast'
import { AccountCredentialFields } from './account-credential-fields'
import { AccountSiteField } from './account-site-field'

interface AccountFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  account?: Account | null
  sites: Site[]
  initialSiteId?: number
  /** Credential-mode hint from the sites guided-flow deep link
   *  (`segment=apikey`) — applied as the create default only; the operator
   *  can switch tabs freely afterwards. Ignored in edit mode. */
  initialCredentialMode?: CredentialMode
}

export function AccountFormDialog({
  open,
  onOpenChange,
  mode,
  account,
  sites,
  initialSiteId,
  initialCredentialMode,
}: AccountFormDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreateAccount()
  const updateMutation = useUpdateAccount()
  const handleReauth = useReauthReplay()
  const loginMutation = useLoginAccount()
  const presets = useSiteInitializationPresets()
  const isEdit = mode === 'edit' && !!account

  const schema = useMemo(() => getAccountFormSchema(!isEdit), [isEdit])
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountFormDefaultValues(),
  })

  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  const credentialMode = form.watch('credentialMode') as CredentialMode
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const isInitialized = initializedFor !== null

  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && account ? `edit:${account.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    // An explicit deep link wins; direct API sites start in API-key mode.
    const initialSite = sites.find((site) => site.id === initialSiteId)
    const defaultMode =
      getPlatformDefinition(initialSite?.platform)?.group === 'api'
        ? 'apikey'
        : 'session'
    const baseDefaults = getAccountFormDefaultValues(
      isEdit
        ? (account?.credentialMode ?? 'session')
        : (initialCredentialMode ?? defaultMode)
    )
    if (isEdit && account) {
      form.reset({ ...baseDefaults, ...transformAccountToFormValues(account) })
    } else {
      form.reset({
        ...baseDefaults,
        siteId: initialSiteId ?? baseDefaults.siteId,
      })
    }
  }, [
    open,
    isEdit,
    account,
    initializedFor,
    initialSiteId,
    initialCredentialMode,
    sites,
    form,
  ])

  const watchedSiteId = form.watch('siteId')
  const watchedPassword = form.watch('password')
  const watchedUsername = form.watch('username')
  const selectedSite = sites.find((site) => site.id === watchedSiteId)
  const preset = findAccountInitializationPreset(selectedSite, presets.data)
  const apiSite = getPlatformDefinition(selectedSite?.platform)?.group === 'api'
  const passwordDraft =
    !!watchedPassword || (credentialMode === 'password' && !!watchedUsername)
  const showPasswordMode = isEdit || !apiSite || passwordDraft

  useEffect(() => {
    if (
      !open ||
      !isInitialized ||
      isEdit ||
      credentialMode !== 'apikey' ||
      !presets.isSuccess ||
      form.getFieldState('skipModelFetch').isTouched
    ) {
      return
    }
    form.setValue(
      'skipModelFetch',
      preset?.recommendedSkipModelFetch ?? false,
      { shouldDirty: false }
    )
  }, [
    open,
    isInitialized,
    isEdit,
    credentialMode,
    watchedSiteId,
    preset?.recommendedSkipModelFetch,
    presets.isSuccess,
    form,
  ])

  useEffect(() => {
    if (
      open &&
      isInitialized &&
      !showPasswordMode &&
      credentialMode === 'password'
    ) {
      form.setValue('credentialMode', 'apikey')
    }
  }, [open, isInitialized, showPasswordMode, credentialMode, form])

  const onSubmit = async (values: AccountFormValues) => {
    if (values.credentialMode === 'password') {
      try {
        const result = await loginMutation.mutateAsync({
          siteId: values.siteId,
          username: values.username?.trim() ?? '',
          password: values.password ?? '',
        })
        showAccountLoginToast(
          result?.account?.id,
          values.siteId,
          result?.reusedAccount,
          {
            tokenCount: result?.tokenCount,
            tokenSyncStatus: result?.tokenSyncStatus,
            tokenSyncMessage: result?.tokenSyncMessage,
          }
        )
        form.reset()
        onOpenChange(false)
      } catch {
        // http-client already toasted the business/network error.
      }
      return
    }

    const payload = transformFormToPayload(values, isEdit ? 'update' : 'create')
    if (!payload) {
      toast.error(t('accounts.toast.loginFailed'))
      return
    }
    const save = async (confirmToken?: string) => {
      if (isEdit && account) {
        await updateMutation.mutateAsync({
          id: account.id,
          payload,
          confirmToken,
        })
      } else {
        const result = await createMutation.mutateAsync({
          payload,
          confirmToken,
        })
        showAccountCreatedToast(
          resolveCreatedAccountId(result),
          values.siteId,
          {
            tokenCount: result?.tokenCount,
            tokenSyncStatus: result?.tokenSyncStatus,
            tokenSyncMessage: result?.tokenSyncMessage,
          }
        )
      }
      form.reset()
      onOpenChange(false)
    }
    try {
      await save()
    } catch (error) {
      // A per-account proxy redirects the stored upstream credential; the
      // backend answers reauthRequired until the master token is re-entered.
      handleReauth(error, t('reauth.operations.accountProxy'), (confirmToken) =>
        save(confirmToken)
      )
    }
  }

  const onInvalid: SubmitErrorHandler<AccountFormValues> = () => {
    toast.error(t('accounts.form.invalid'))
  }

  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    loginMutation.isPending

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 overflow-hidden sm:max-w-lg'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b'>
          <SheetTitle>
            {isEdit
              ? t('accounts.form.editTitle')
              : t('accounts.form.addTitle')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('accounts.form.editDescription')
              : t('accounts.form.addDescription')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='account-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            inert={!isInitialized ? true : undefined}
            aria-busy={!isInitialized}
            className='min-h-0 flex-1 space-y-6 overflow-y-auto p-4 text-sm leading-5 [&_[data-slot=form-description]]:leading-5 [&_[data-slot=form-label]]:font-semibold'
          >
            <AccountSiteField
              form={form}
              sites={sites}
              open={open}
              inferCredentialMode={!isEdit && !initialCredentialMode}
            />
            <AccountCredentialFields
              form={form}
              open={open}
              credentialMode={credentialMode}
              showPasswordMode={showPasswordMode}
            />
            {/* Account-parameter fields (status / check-in / unit cost / proxy
                / tags) only apply to session or api-key credentials: the
                password login-bind endpoint force-sets status + check-in and
                ignores the rest, so editing them in password mode would be a
                silent no-op. Hide them instead of showing
                controls that do nothing. */}
            {credentialMode !== 'password' && (
              <>
                {/* Status */}
                <FormField
                  control={form.control}
                  name='status'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('accounts.form.status')}</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue>
                              {(selected) => {
                                const labels: Record<string, string> = {
                                  active: t('accounts.form.statusActive'),
                                  disabled: t('accounts.form.statusDisabled'),
                                  expired: t('accounts.form.statusExpired'),
                                }
                                return selected
                                  ? (labels[String(selected)] ??
                                      String(selected))
                                  : ''
                              }}
                            </SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value='active'>
                            {t('accounts.form.statusActive')}
                          </SelectItem>
                          <SelectItem value='disabled'>
                            {t('accounts.form.statusDisabled')}
                          </SelectItem>
                          <SelectItem value='expired'>
                            {t('accounts.form.statusExpired')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Checkin toggle */}
                <FormField
                  control={form.control}
                  name='checkinEnabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                      <div className='space-y-0.5'>
                        <FormLabel>
                          {t('accounts.form.checkinEnabled')}
                        </FormLabel>
                        <FormDescription>
                          {t('accounts.form.checkinEnabledHint')}
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

                {/* Unit cost */}
                <FormField
                  control={form.control}
                  name='unitCost'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('accounts.form.unitCost')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          step='0.01'
                          placeholder='0.00'
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

                {/* Proxy URL */}
                <FormField
                  control={form.control}
                  name='proxyUrl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('accounts.form.proxyUrl')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder='https://proxy.example.com or socks5://proxy.example.com:1080'
                          {...field}
                          value={field.value ?? ''}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tags */}
                <FormField
                  control={form.control}
                  name='tags'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('accounts.form.tags')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder='prod, priority'
                          {...field}
                          value={field.value ?? ''}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('accounts.form.tagsHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </>
            )}
          </form>
        </Form>

        <SheetFooter>
          <Button
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='account-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Spinner />}
            {isEdit ? t('accounts.form.save') : t('accounts.form.create')}
          </Button>
        </SheetFooter>
      </SheetContent>
      {guard}
    </Sheet>
  )
}
