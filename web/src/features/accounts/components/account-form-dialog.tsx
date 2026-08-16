// metapi-go features/accounts/components — add/edit account form dialog.
//
// A Sheet side drawer with react-hook-form + zod, mirroring the keys
// feature's `api-keys-mutate-drawer.tsx` structure: schema factory from
// `lib/accounts-schema`, guarded one-time `form.reset` on open target,
// inert-until-initialized, and a footer submit button bound to the form id.
//
// On create success the form fires the guided "next step: configure routes"
// toast (account-created-toast.tsx) — step 2 of the site → account → route
// chain — rather than a plain confirmation.

import { zodResolver } from '@hookform/resolvers/zod'
import { CheckCircle2, Loader2, TriangleAlert } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  useForm,
  type SubmitErrorHandler,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'

import {
  useCreateAccount,
  useLoginAccount,
  useUpdateAccount,
  useVerifyAccountToken,
} from '../api'
import {
  buildAccountVerifyPayload,
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import type { Account, CredentialMode, Site } from '../types'
import { showAccountCreatedToast } from './account-created-toast'

interface AccountFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  account?: Account | null
  sites: Site[]
  initialSiteId?: number
}

// Inline verification state for the session/apikey credential fields. Resets
// to idle whenever the operator changes site, mode, or credential so a stale
// "verified" result can never be reattached to a different credential.
type VerificationState =
  | { status: 'idle' }
  | { status: 'pending' }
  | { status: 'verified'; tokenType: string; modelCount: number }
  | { status: 'failed'; message: string }

function resolveAxiosErrorMessage(error: unknown): string {
  if (error && typeof error === 'object') {
    const response = (error as { response?: { data?: unknown } }).response
    const data = response?.data
    if (data && typeof data === 'object') {
      const message = (data as { error?: unknown }).error
      if (typeof message === 'string' && message.trim()) return message
    }
  }
  return ''
}

export function AccountFormDialog({
  open,
  onOpenChange,
  mode,
  account,
  sites,
  initialSiteId,
}: AccountFormDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreateAccount()
  const updateMutation = useUpdateAccount()
  const loginMutation = useLoginAccount()
  const verifyMutation = useVerifyAccountToken()
  const isEdit = mode === 'edit' && !!account

  const schema = useMemo(() => getAccountFormSchema(), [])
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
    const baseDefaults = getAccountFormDefaultValues(
      account?.credentialMode ?? 'session'
    )
    if (isEdit && account) {
      form.reset({ ...baseDefaults, ...transformAccountToFormValues(account) })
    } else {
      form.reset({
        ...baseDefaults,
        siteId: initialSiteId ?? baseDefaults.siteId,
      })
    }
  }, [open, isEdit, account, initializedFor, initialSiteId, form])

  // Inline credential verification (session / apikey only). Password mode
  // binds through the real login submit path, so it never verifies inline.
  const [verification, setVerification] = useState<VerificationState>({
    status: 'idle',
  })
  const watchedSiteId = form.watch('siteId')
  const watchedAccessToken = form.watch('accessToken')
  const watchedApiToken = form.watch('apiToken')
  useEffect(() => {
    setVerification({ status: 'idle' })
  }, [watchedSiteId, credentialMode, watchedAccessToken, watchedApiToken])

  const handleVerify = async () => {
    const resolved = buildAccountVerifyPayload(form.getValues())
    if (!resolved.ok) {
      setVerification({
        status: 'failed',
        message: t(
          resolved.error === 'site'
            ? 'accounts.verify.siteRequired'
            : 'accounts.verify.tokenRequired'
        ),
      })
      return
    }
    setVerification({ status: 'pending' })
    try {
      const result = await verifyMutation.mutateAsync(resolved.payload)
      setVerification({
        status: 'verified',
        tokenType: result.tokenType ?? 'unknown',
        modelCount: result.modelCount ?? 0,
      })
    } catch (error) {
      const backendMessage = resolveAxiosErrorMessage(error)
      setVerification({
        status: 'failed',
        message: backendMessage || t('accounts.verify.failed'),
      })
    }
  }

  const onSubmit = async (values: AccountFormValues) => {
    try {
      if (values.credentialMode === 'password') {
        const result = await loginMutation.mutateAsync({
          siteId: values.siteId,
          username: values.username?.trim() ?? '',
          password: values.password ?? '',
        })
        toast.success(
          t(
            result?.reusedAccount
              ? 'accounts.toast.loginRelogged'
              : 'accounts.toast.loginSucceeded'
          )
        )
        form.reset()
        onOpenChange(false)
        return
      }

      const payload = transformFormToPayload(values)
      if (!payload) {
        toast.error(t('accounts.toast.loginFailed'))
        return
      }
      if (isEdit && account) {
        await updateMutation.mutateAsync({ id: account.id, payload })
      } else {
        const result = await createMutation.mutateAsync(payload)
        const newId = result?.data?.id ?? result?.data?.account?.id ?? undefined
        showAccountCreatedToast(newId, values.siteId)
      }
      form.reset()
      onOpenChange(false)
    } catch {
      // http-client already toasted the business/network error.
    }
  }

  const onInvalid: SubmitErrorHandler<AccountFormValues> = () => {
    toast.error(t('accounts.form.invalid'))
  }

  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    loginMutation.isPending
  const siteOptions = sites

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-lg'
      >
        <SheetHeader>
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
            className='flex-1 space-y-5 overflow-y-auto p-4'
          >
            {/* Site selection */}
            <FormField
              control={form.control}
              name='siteId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.form.site')}</FormLabel>
                  <Select
                    value={field.value ? String(field.value) : ''}
                    onValueChange={(value) => field.onChange(Number(value))}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue>
                          {(selected) => {
                            if (!selected) {
                              return t('accounts.form.sitePlaceholder')
                            }
                            const site = siteOptions.find(
                              (item) => String(item.id) === selected
                            )
                            if (!site) return String(selected)
                            const suffix = site.platform
                              ? ` · ${site.platform}`
                              : ''
                            return `${site.name || site.url || `#${site.id}`}${suffix}`
                          }}
                        </SelectValue>
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {siteOptions.length === 0 && (
                        <SelectItem value='__none' disabled>
                          {t('accounts.form.siteEmpty')}
                        </SelectItem>
                      )}
                      {siteOptions.map((site) => (
                        <SelectItem key={site.id} value={String(site.id)}>
                          {site.name || site.url || `#${site.id}`}
                          {site.platform ? ` · ${site.platform}` : ''}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Credential mode toggle */}
            <FormItem>
              <FormLabel>{t('accounts.form.credentialMode')}</FormLabel>
              <Tabs
                value={credentialMode}
                onValueChange={(value) =>
                  form.setValue('credentialMode', value as CredentialMode, {
                    shouldDirty: true,
                  })
                }
              >
                <TabsList>
                  <TabsTrigger value='session'>
                    {t('accounts.form.modeSession')}
                  </TabsTrigger>
                  <TabsTrigger value='apikey'>
                    {t('accounts.form.modeApiKey')}
                  </TabsTrigger>
                  <TabsTrigger value='password'>
                    {t('accounts.form.modePassword')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </FormItem>

            {/* Connection name (optional) — hidden in password mode, where
                username is the site login name collected by PasswordFields */}
            {credentialMode !== 'password' && (
              <FormField
                control={form.control}
                name='username'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('accounts.form.connectionName')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          'accounts.form.connectionNamePlaceholder'
                        )}
                        {...field}
                        value={field.value ?? ''}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {credentialMode === 'session' && (
              <SessionFields
                form={form}
                verification={verification}
                onVerify={handleVerify}
              />
            )}
            {credentialMode === 'apikey' && (
              <ApiKeyFields
                form={form}
                verification={verification}
                onVerify={handleVerify}
              />
            )}
            {credentialMode === 'password' && <PasswordFields form={form} />}

            {/* Status */}
            <FormField
              control={form.control}
              name='status'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.form.status')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
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
                              ? (labels[String(selected)] ?? String(selected))
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
                    <FormLabel>{t('accounts.form.checkinEnabled')}</FormLabel>
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
                      placeholder='https://proxy.example.com'
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
          </form>
        </Form>

        <SheetFooter>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='account-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Loader2 className='animate-spin' />}
            {isEdit ? t('accounts.form.save') : t('accounts.form.create')}
          </Button>
        </SheetFooter>
      </SheetContent>
      {guard}
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Session-mode fields
// ---------------------------------------------------------------------------

function VerifyCredentialFeedback({
  verification,
  onVerify,
}: {
  verification: VerificationState
  onVerify: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-1.5'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={onVerify}
        disabled={verification.status === 'pending'}
      >
        {verification.status === 'pending' ? (
          <Loader2 className='animate-spin' />
        ) : null}
        {t('accounts.verify.button')}
      </Button>
      {verification.status === 'verified' && (
        <p className='text-success flex items-center gap-1.5 text-xs'>
          <CheckCircle2 className='size-3.5' />
          {t('accounts.verify.verified', {
            tokenType: verification.tokenType,
            modelCount: verification.modelCount,
          })}
        </p>
      )}
      {verification.status === 'failed' && (
        <div className='text-destructive space-y-0.5 text-xs'>
          <p className='flex items-center gap-1.5'>
            <TriangleAlert className='size-3.5' />
            {t('accounts.verify.failed')}
          </p>
          {verification.message && (
            <p className='text-destructive/80'>{verification.message}</p>
          )}
        </div>
      )}
    </div>
  )
}

interface SessionFieldsProps {
  form: UseFormReturn<AccountFormValues>
  verification: VerificationState
  onVerify: () => void
}

function SessionFields({ form, verification, onVerify }: SessionFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='accessToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.accessToken')}</FormLabel>
            <FormControl>
              <Textarea
                rows={4}
                placeholder={t('accounts.formSession.accessTokenPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('accounts.formSession.accessTokenHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <VerifyCredentialFeedback
        verification={verification}
        onVerify={onVerify}
      />

      <FormField
        control={form.control}
        name='platformUserId'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.platformUserId')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.platformUserIdPlaceholder'
                )}
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

      <FormField
        control={form.control}
        name='refreshToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.refreshToken')}</FormLabel>
            <FormControl>
              <Input
                className='font-mono text-xs'
                placeholder={t('accounts.formSession.refreshTokenPlaceholder')}
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
        name='tokenExpiresAt'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.tokenExpiresAt')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.tokenExpiresAtPlaceholder'
                )}
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
    </>
  )
}

// ---------------------------------------------------------------------------
// Password-mode fields — username+password sign-in, bound via the separate
// POST /api/accounts/login endpoint (the backend fetches the session token
// and stores autoRelogin config on the account).
// ---------------------------------------------------------------------------

function PasswordFields({ form }: { form: UseFormReturn<AccountFormValues> }) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='username'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.username')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='username'
                placeholder={t('accounts.formPassword.usernamePlaceholder')}
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
        name='password'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.password')}</FormLabel>
            <FormControl>
              <Input
                type='password'
                autoComplete='new-password'
                placeholder={t('accounts.formPassword.passwordPlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>{t('accounts.formPassword.hint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// API-Key-mode fields
// ---------------------------------------------------------------------------

function ApiKeyFields({ form, verification, onVerify }: SessionFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='apiToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formApiKey.apiKey')}</FormLabel>
            <FormControl>
              <Textarea
                rows={3}
                placeholder={t('accounts.formApiKey.apiKeyPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <VerifyCredentialFeedback
        verification={verification}
        onVerify={onVerify}
      />

      <FormField
        control={form.control}
        name='skipModelFetch'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('accounts.formApiKey.skipModelFetch')}</FormLabel>
              <FormDescription>
                {t('accounts.formApiKey.skipModelFetchHint')}
              </FormDescription>
            </div>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </>
  )
}
