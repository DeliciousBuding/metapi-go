// metapi-go/features/settings/sections/general/components — authentication
// section. Two surfaces: admin token rotation (POST /api/settings/auth/change)
// and the admin IP allowlist (PUT /api/settings/runtime, adminIpAllowlist).
// The current masked token is shown via GET /api/settings/auth/info.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { api } from '@/lib/api'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asString,
  joinListField,
  splitListField,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const ALLOWLIST_FORM_ID = 'settings-general-auth-allowlist-form'
const TOKEN_FORM_ID = 'settings-general-auth-token-form'

const tokenSchema = z
  .object({
    oldToken: z
      .string()
      .min(1, 'settings.general.authentication.schema.oldRequired'),
    newToken: z
      .string()
      .min(6, 'settings.general.authentication.schema.newMinLength'),
    confirmToken: z
      .string()
      .min(1, 'settings.general.authentication.schema.confirmRequired'),
  })
  .refine((values) => values.newToken === values.confirmToken, {
    path: ['confirmToken'],
    message: 'settings.general.authentication.schema.confirmMismatch',
  })

type TokenFormValues = z.infer<typeof tokenSchema>

const allowlistSchema = z.object({
  adminIpAllowlist: z.string().optional(),
})

type AllowlistFormValues = z.infer<typeof allowlistSchema>

type AuthInfo = { masked?: string; currentAdminIp?: string }

export function AuthenticationSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
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
      void authInfoQuery.refetch()
      toast.success(t('settings.general.authentication.toast.tokenChanged'))
      setShowTokenFields(false)
    },
    onError: () =>
      toast.error(t('settings.general.authentication.toast.tokenChangeFailed')),
  })

  const tokenForm = useForm<TokenFormValues>({
    resolver: zodResolver(tokenSchema) as never,
    defaultValues: { oldToken: '', newToken: '', confirmToken: '' },
  })

  const allowlistForm = useForm<AllowlistFormValues>({
    resolver: zodResolver(allowlistSchema) as never,
    defaultValues: { adminIpAllowlist: '' },
  })

  useEffect(() => {
    if (!data) {
      return
    }
    allowlistForm.reset(
      {
        adminIpAllowlist: joinListField(splitListField(data.adminIpAllowlist)),
      },
      { keepDirtyValues: true }
    )
  }, [data, allowlistForm])

  function onAllowlistSubmit(values: AllowlistFormValues) {
    updateMutation.mutate(
      { adminIpAllowlist: splitListField(values.adminIpAllowlist) },
      {
        onSuccess: () =>
          toast.success(
            t('settings.general.authentication.toast.allowlistSaved')
          ),
        onError: () =>
          toast.error(
            t('settings.general.authentication.toast.allowlistSaveFailed')
          ),
      }
    )
  }

  function onTokenSubmit(values: TokenFormValues) {
    tokenMutation.mutate(values)
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  return (
    <SettingsSectionCard
      title={t('settings.general.authentication.title')}
      description={t('settings.general.authentication.description')}
    >
      <div className='space-y-6'>
        <div className='space-y-3'>
          <div className='flex items-center gap-3'>
            <span className='text-muted-foreground text-sm'>
              {t('settings.general.authentication.currentToken')}
            </span>
            <code className='bg-muted rounded px-2 py-0.5 text-xs'>
              {asString(authInfoQuery.data?.masked) ||
                t('settings.general.authentication.notSet')}
            </code>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setShowTokenFields((prev) => !prev)}
            >
              {showTokenFields
                ? t('settings.general.authentication.cancelChange')
                : t('settings.general.authentication.changeToken')}
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
                        {t('settings.general.authentication.fields.oldToken')}
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
                        {t('settings.general.authentication.fields.newToken')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='new-password'
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'settings.general.authentication.fields.newTokenHint'
                        )}
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
                        {t(
                          'settings.general.authentication.fields.confirmToken'
                        )}
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
                    : t('settings.general.authentication.submitToken')}
                </Button>
              </form>
            </Form>
          ) : null}
        </div>

        <div className='space-y-3'>
          {authInfoQuery.data?.currentAdminIp ? (
            <div className='text-muted-foreground text-sm'>
              <span>{t('settings.general.authentication.detectedIp')}</span>
              <code className='bg-muted ml-2 rounded px-2 py-0.5 text-xs'>
                {asString(authInfoQuery.data.currentAdminIp)}
              </code>
            </div>
          ) : null}
          <Form {...allowlistForm}>
            <form
              id={ALLOWLIST_FORM_ID}
              onSubmit={allowlistForm.handleSubmit(onAllowlistSubmit)}
              className='space-y-3'
            >
              <FormField
                control={allowlistForm.control}
                name='adminIpAllowlist'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t(
                        'settings.general.authentication.fields.adminIpAllowlist'
                      )}
                    </FormLabel>
                    <FormControl>
                      <textarea
                        {...field}
                        value={field.value ?? ''}
                        rows={4}
                        placeholder='127.0.0.1&#10;192.168.1.0/24'
                        className='border-input placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 flex field-sizing-content w-full rounded-md border bg-transparent px-3 py-2 text-sm shadow-xs transition-[color,box-shadow] outline-none focus-visible:ring-[3px]'
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'settings.general.authentication.fields.adminIpAllowlistHint'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button
                type='submit'
                form={ALLOWLIST_FORM_ID}
                disabled={updateMutation.isPending}
              >
                {updateMutation.isPending
                  ? t('settings.common.saving')
                  : t('settings.common.save')}
              </Button>
            </form>
          </Form>
        </div>
      </div>
    </SettingsSectionCard>
  )
}
