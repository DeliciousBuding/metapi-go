// metapi-go/features/settings/sections/basic/components — authentication
// section. Two surfaces: admin token rotation (POST /api/settings/auth/change)
// and the admin IP allowlist (PUT /api/settings/runtime, adminIpAllowlist).
// The current masked token is shown via GET /api/settings/auth/info.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

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
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { clearAuthentication } from '@/lib/auth-session'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  asString,
  joinListField,
  splitListField,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

const ALLOWLIST_FORM_ID = 'settings-general-auth-allowlist-form'
const TOKEN_FORM_ID = 'settings-general-auth-token-form'

const tokenSchema = z
  .object({
    oldToken: z
      .string()
      .min(1, 'settings.basic.authentication.schema.oldRequired'),
    newToken: z
      .string()
      .min(6, 'settings.basic.authentication.schema.newMinLength'),
    confirmToken: z
      .string()
      .min(1, 'settings.basic.authentication.schema.confirmRequired'),
  })
  .refine((values) => values.newToken === values.confirmToken, {
    path: ['confirmToken'],
    message: 'settings.basic.authentication.schema.confirmMismatch',
  })

type TokenFormValues = z.infer<typeof tokenSchema>

const allowlistSchema = z.object({
  adminIpAllowlist: z.string().optional(),
})

type AllowlistFormValues = z.infer<typeof allowlistSchema>

const DEFAULT_ALLOWLIST_VALUES: AllowlistFormValues = { adminIpAllowlist: '' }

type AuthInfo = { masked?: string; currentAdminIp?: string }

function deriveAllowlistServerValues(
  data: RuntimeSettings | undefined
): AllowlistFormValues | null {
  if (!data) {
    return null
  }
  return {
    adminIpAllowlist: joinListField(splitListField(data.adminIpAllowlist)),
  }
}

export function AuthenticationSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()
  const [showTokenFields, setShowTokenFields] = useState(false)

  const authInfoQuery = useQuery<AuthInfo>({
    queryKey: ['settings-auth-info'] as const,
    queryFn: async () => {
      const info = (await api.getAuthInfo()) as AuthInfo
      return info ?? {}
    },
    staleTime: 30 * 1000,
  })

  const tokenMutation = useMutation({
    mutationFn: async (values: TokenFormValues) =>
      api.changeAuthToken(values.oldToken, values.newToken),
    onSuccess: () => {
      toast.success(t('settings.basic.authentication.toast.tokenChanged'))
      // The old token stops working immediately, so the current session is
      // dead the moment rotation succeeds. Clear it and send the operator to
      // sign in with the new token — staying on this page would just loop
      // on invalid-token 403s.
      clearAuthentication()
      window.location.replace('/sign-in?reason=tokenChanged')
    },
    onError: () =>
      toast.error(t('settings.basic.authentication.toast.tokenChangeFailed')),
  })

  const tokenForm = useForm<TokenFormValues>({
    resolver: zodResolver(tokenSchema) as never,
    defaultValues: { oldToken: '', newToken: '', confirmToken: '' },
  })

  const serverValues = deriveAllowlistServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<AllowlistFormValues>({
      schema: allowlistSchema,
      defaultValues: DEFAULT_ALLOWLIST_VALUES,
      serverValues,
    })

  function onAllowlistSubmit(values: AllowlistFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<AllowlistFormValues>
    if (!hasChanges(changed) || changed.adminIpAllowlist === undefined) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(
      { adminIpAllowlist: splitListField(changed.adminIpAllowlist) },
      {
        onSuccess: () =>
          toast.success(
            t('settings.basic.authentication.toast.allowlistSaved')
          ),
        onError: () =>
          toast.error(
            t('settings.basic.authentication.toast.allowlistSaveFailed')
          ),
      }
    )
  }

  function onTokenSubmit(values: TokenFormValues) {
    tokenMutation.mutate(values)
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.basic.authentication.title')}
        description={t('settings.basic.authentication.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SettingsSectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SettingsSectionError
        title={t('settings.basic.authentication.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.basic.authentication.title')}
      description={t('settings.basic.authentication.description')}
    >
      <div className='space-y-4'>
        <div className='space-y-3'>
          <div className='flex items-center gap-3'>
            <span className='text-muted-foreground text-sm'>
              {t('settings.basic.authentication.currentToken')}
            </span>
            <code className='bg-muted rounded px-2 py-0.5 text-xs'>
              {asString(authInfoQuery.data?.masked) ||
                t('settings.basic.authentication.notSet')}
            </code>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setShowTokenFields((prev) => !prev)}
            >
              {showTokenFields
                ? t('settings.basic.authentication.cancelChange')
                : t('settings.basic.authentication.changeToken')}
            </Button>
          </div>
          {showTokenFields ? (
            <Form {...tokenForm}>
              <form
                id={TOKEN_FORM_ID}
                onSubmit={tokenForm.handleSubmit(onTokenSubmit)}
                className='space-y-4'
              >
                <FormField
                  control={tokenForm.control}
                  name='oldToken'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('settings.basic.authentication.fields.oldToken')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='current-password'
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={tokenForm.control}
                  name='newToken'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('settings.basic.authentication.fields.newToken')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='new-password'
                        />
                      </FormControl>
                      <FormDescription>
                        {t('settings.basic.authentication.fields.newTokenHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={tokenForm.control}
                  name='confirmToken'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('settings.basic.authentication.fields.confirmToken')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='new-password'
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button
                  type='submit'
                  form={TOKEN_FORM_ID}
                  disabled={tokenMutation.isPending}
                >
                  {tokenMutation.isPending
                    ? t('settings.common.saving')
                    : t('settings.basic.authentication.submitToken')}
                </Button>
              </form>
            </Form>
          ) : null}
        </div>

        <div className='space-y-3'>
          {authInfoQuery.data?.currentAdminIp ? (
            <div className='text-muted-foreground text-sm'>
              <span>{t('settings.basic.authentication.detectedIp')}</span>
              <code className='bg-muted ml-2 rounded px-2 py-0.5 text-xs'>
                {asString(authInfoQuery.data.currentAdminIp)}
              </code>
            </div>
          ) : null}
          <Form {...form}>
            <form
              id={ALLOWLIST_FORM_ID}
              onSubmit={form.handleSubmit(onAllowlistSubmit)}
              className='space-y-3'
            >
              <FormField
                control={form.control}
                name='adminIpAllowlist'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t(
                        'settings.basic.authentication.fields.adminIpAllowlist'
                      )}
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        value={field.value ?? ''}
                        rows={4}
                        placeholder='127.0.0.1&#10;192.168.1.0/24'
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'settings.basic.authentication.fields.adminIpAllowlistHint'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <SettingsFormActions
                formId={ALLOWLIST_FORM_ID}
                isDirty={isDirty}
                isPending={updateMutation.isPending}
                onReset={() =>
                  syncFromServer(
                    deriveAllowlistServerValues(data) ??
                      DEFAULT_ALLOWLIST_VALUES
                  )
                }
              />
            </form>
          </Form>
        </div>
      </div>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
