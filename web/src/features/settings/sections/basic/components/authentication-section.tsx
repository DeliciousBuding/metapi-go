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

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
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

const ALLOWLIST_FORM_ID = 'settings-basic-auth-allowlist-form'
const TOKEN_FORM_ID = 'settings-basic-auth-token-form'

const tokenSchema = z.object({
  oldToken: z
    .string()
    .min(1, 'settings.basic.authentication.schema.oldRequired'),
})

type TokenFormValues = z.infer<typeof tokenSchema>

const allowlistSchema = z.object({
  adminIpAllowlist: z.string().optional(),
})

type AllowlistFormValues = z.infer<typeof allowlistSchema>

const DEFAULT_ALLOWLIST_VALUES: AllowlistFormValues = { adminIpAllowlist: '' }

type AuthInfo = {
  customized?: boolean
  configured?: boolean
  length?: number
  currentAdminIp?: string
}

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

  const [generatedToken, setGeneratedToken] = useState<string | null>(null)

  const tokenMutation = useMutation({
    mutationFn: async (values: TokenFormValues) =>
      api.changeAuthToken(values.oldToken),
    onSuccess: (result) => {
      const newToken = result?.newToken
      if (newToken) {
        // The server chose the secret. Show it exactly once and only then
        // invalidate the local session; redirecting immediately would hide
        // the only copy of the new token.
        setGeneratedToken(newToken)
        toast.success(t('settings.basic.authentication.toast.tokenGenerated'))
        return
      }
      finishRotation()
    },
    onError: () =>
      toast.error(t('settings.basic.authentication.toast.tokenChangeFailed')),
  })

  function finishRotation() {
    // The old token stops working immediately, so the current session is
    // dead the moment rotation succeeds. Clear it and send the operator to
    // sign in with the new token — staying on this page would just loop
    // on invalid-token 403s.
    clearAuthentication()
    window.location.replace('/sign-in?reason=tokenChanged')
  }

  const tokenForm = useForm<TokenFormValues>({
    resolver: zodResolver(tokenSchema) as never,
    defaultValues: { oldToken: '' },
  })

  const serverValues = deriveAllowlistServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<AllowlistFormValues>({
      schema: allowlistSchema,
      defaultValues: DEFAULT_ALLOWLIST_VALUES,
      serverValues,
    })

  function onAllowlistSubmit(values: AllowlistFormValues) {
    const changed = collectChangedFields(values, baseline)
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
      <SectionCard
        title={t('settings.basic.authentication.title')}
        description={t('settings.basic.authentication.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SectionError
        title={t('settings.basic.authentication.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SectionCard
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
              {authInfoQuery.data
                ? authInfoQuery.data.customized
                  ? t('settings.basic.authentication.customizedToken', {
                      length: authInfoQuery.data.length ?? 0,
                    })
                  : authInfoQuery.data.configured
                    ? t('settings.basic.authentication.defaultToken')
                    : t('settings.basic.authentication.notSet')
                : t('settings.basic.authentication.notSet')}
            </code>
            {generatedToken ? null : (
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
            )}
          </div>
          {showTokenFields && !generatedToken ? (
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
                      <FormDescription>
                        {t('settings.basic.authentication.fields.generateHint')}
                      </FormDescription>
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
          {generatedToken ? (
            <div className='border-warning/40 bg-warning/10 space-y-2 rounded-md border p-3'>
              <p className='text-sm font-medium'>
                {t('settings.basic.authentication.generatedTitle')}
              </p>
              <code className='bg-background block rounded px-2 py-1 text-xs break-all'>
                {generatedToken}
              </code>
              <p className='text-muted-foreground text-xs'>
                {t('settings.basic.authentication.generatedHint')}
              </p>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => {
                    void navigator.clipboard
                      .writeText(generatedToken)
                      .then(() =>
                        toast.success(
                          t('settings.basic.authentication.generatedCopied')
                        )
                      )
                  }}
                >
                  {t('settings.basic.authentication.generatedCopy')}
                </Button>
                <Button type='button' size='sm' onClick={finishRotation}>
                  {t('settings.basic.authentication.generatedSaved')}
                </Button>
              </div>
            </div>
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
    </SectionCard>
  )
}
