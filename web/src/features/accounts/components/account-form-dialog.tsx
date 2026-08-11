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
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  useForm,
  type SubmitErrorHandler,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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

import { useCreateAccount, useUpdateAccount } from '../api'
import { showAccountCreatedToast } from './account-created-toast'
import {
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import type { Account, CredentialMode, Site } from '../types'

interface AccountFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  account?: Account | null
  sites: Site[]
}

export function AccountFormDialog({
  open,
  onOpenChange,
  mode,
  account,
  sites,
}: AccountFormDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreateAccount()
  const updateMutation = useUpdateAccount()
  const isEdit = mode === 'edit' && !!account

  const schema = useMemo(() => getAccountFormSchema(), [])
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountFormDefaultValues(),
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
      account?.credentialMode ?? 'session',
    )
    if (isEdit && account) {
      form.reset({ ...baseDefaults, ...transformAccountToFormValues(account) })
    } else {
      form.reset(baseDefaults)
    }
  }, [open, isEdit, account, initializedFor, form])

  const onSubmit = async (values: AccountFormValues) => {
    const payload = transformFormToPayload(values)
    try {
      if (isEdit && account) {
        await updateMutation.mutateAsync({ id: account.id, payload })
      } else {
        const result = await createMutation.mutateAsync(payload)
        const newId =
          result?.data?.id ?? result?.data?.account?.id ?? undefined
        showAccountCreatedToast(newId, values.siteId)
      }
      onOpenChange(false)
    } catch {
      // http-client already toasted the business/network error.
    }
  }

  const onInvalid: SubmitErrorHandler<AccountFormValues> = () => {
    toast.error(t('accounts.form.invalid'))
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending
  const siteOptions = sites

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='flex w-full flex-col gap-0 sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>{isEdit ? t('accounts.form.editTitle') : t('accounts.form.addTitle')}</SheetTitle>
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
                        <SelectValue placeholder={t('accounts.form.sitePlaceholder')} />
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
                  <TabsTrigger value='session'>{t('accounts.form.modeSession')}</TabsTrigger>
                  <TabsTrigger value='apikey'>{t('accounts.form.modeApiKey')}</TabsTrigger>
                </TabsList>
              </Tabs>
            </FormItem>

            {/* Connection name (optional) */}
            <FormField
              control={form.control}
              name='username'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.form.connectionName')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('accounts.form.connectionNamePlaceholder')}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {credentialMode === 'session' ? (
              <SessionFields form={form} />
            ) : (
              <ApiKeyFields form={form} />
            )}

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
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='active'>{t('accounts.form.statusActive')}</SelectItem>
                      <SelectItem value='disabled'>{t('accounts.form.statusDisabled')}</SelectItem>
                      <SelectItem value='expired'>{t('accounts.form.statusExpired')}</SelectItem>
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
                    <FormDescription>{t('accounts.form.checkinEnabledHint')}</FormDescription>
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
                            : Number(event.target.value),
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
                  <FormDescription>{t('accounts.form.tagsHint')}</FormDescription>
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
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Session-mode fields
// ---------------------------------------------------------------------------

interface SessionFieldsProps {
  form: UseFormReturn<AccountFormValues>
}

function SessionFields({ form }: SessionFieldsProps) {
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
            <FormDescription>{t('accounts.formSession.accessTokenHint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
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
                placeholder={t('accounts.formSession.platformUserIdPlaceholder')}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value),
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
                placeholder={t('accounts.formSession.tokenExpiresAtPlaceholder')}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value),
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
// API-Key-mode fields
// ---------------------------------------------------------------------------

function ApiKeyFields({ form }: SessionFieldsProps) {
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

      <FormField
        control={form.control}
        name='skipModelFetch'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('accounts.formApiKey.skipModelFetch')}</FormLabel>
              <FormDescription>{t('accounts.formApiKey.skipModelFetchHint')}</FormDescription>
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
    </>
  )
}
