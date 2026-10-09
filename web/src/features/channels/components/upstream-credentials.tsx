import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Pencil } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import type {
  ImportedCredential,
  ImportedCredentialUpdate,
} from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'

const credentialSchema = z
  .object({
    name: z.string().trim().min(1, 'channels.upstream.required'),
    enabled: z.boolean(),
    replace: z.boolean(),
    accessToken: z.string(),
    refreshToken: z.string(),
    clientId: z.string(),
    idToken: z.string(),
    accountId: z.string(),
    expiresAt: z
      .string()
      .refine(
        (value) =>
          !value ||
          (/^\d+$/.test(value) &&
            Number.isSafeInteger(Number(value)) &&
            Number(value) > 0),
        'channels.upstream.invalidExpiry'
      ),
  })
  .refine((value) => !value.replace || !!value.accessToken.trim(), {
    path: ['accessToken'],
    message: 'channels.upstream.required',
  })
type CredentialValues = z.infer<typeof credentialSchema>

function CredentialForm(props: {
  credential: ImportedCredential
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const credential = props.credential
  const form = useForm<CredentialValues>({
    resolver: zodResolver(credentialSchema),
    defaultValues: {
      name: credential.name,
      enabled: credential.enabled,
      replace: false,
      accessToken: '',
      refreshToken: '',
      clientId: '',
      idToken: '',
      accountId: '',
      expiresAt: '',
    },
  })
  const replace = form.watch('replace')
  const pending = form.formState.isSubmitting
  const dirty = form.formState.isDirty
  const hasUpdate =
    replace ||
    form.formState.dirtyFields.name ||
    form.formState.dirtyFields.enabled
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange(`credential-${credential.id}`, dirty)
  }, [credential.id, dirty, onDirtyChange])

  async function save(values: CredentialValues) {
    const patch: ImportedCredentialUpdate = {}
    if (form.formState.dirtyFields.name) patch.name = values.name
    if (form.formState.dirtyFields.enabled) patch.enabled = values.enabled
    if (values.replace) {
      if (credential.kind === 'api_key') patch.apiKey = values.accessToken
      else {
        patch.oauth = {
          accessToken: values.accessToken,
          refreshToken: values.refreshToken || undefined,
          clientId: values.clientId || undefined,
          idToken: values.idToken || undefined,
          accountId: values.accountId || undefined,
          expiresAt: values.expiresAt ? Number(values.expiresAt) : undefined,
        }
      }
    }
    // Secret-bearing requests stay out of Query/Mutation caches.
    const result = await api.updateImportedCredential(credential.id, patch)
    if (!result.success) return
    form.reset({
      ...values,
      replace: false,
      accessToken: '',
      refreshToken: '',
      clientId: '',
      idToken: '',
      accountId: '',
      expiresAt: '',
    })
    await client.invalidateQueries({ queryKey: upstreamKeys.all })
    toast.success(t('channels.upstream.saved'))
  }

  return (
    <Form {...form}>
      <form
        className='space-y-4 border-t p-4'
        onSubmit={form.handleSubmit((values) => save(values).catch(() => {}))}
      >
        <div className='grid gap-4 sm:grid-cols-[1fr_auto]'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.upstream.name')}</FormLabel>
                <FormControl>
                  <Input {...field} disabled={pending} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center gap-3 sm:pt-6'>
                <FormLabel>{t('channels.imported.enabled')}</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={pending}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </div>
        <FormField
          control={form.control}
          name='replace'
          render={({ field }) => (
            <FormItem className='bg-muted/40 flex items-center justify-between gap-3 rounded-lg p-3'>
              <FormLabel>{t('channels.upstream.replaceCredential')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={pending}
                />
              </FormControl>
            </FormItem>
          )}
        />
        {replace && (
          <div className='space-y-4'>
            <FormField
              control={form.control}
              name='accessToken'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {credential.kind === 'oauth' ? 'Access token' : 'API Key'}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='password'
                      autoComplete='new-password'
                      disabled={pending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {credential.kind === 'oauth' && (
              <>
                <p className='text-muted-foreground text-xs'>
                  {t('channels.upstream.oauthReplacement')}
                </p>
                <div className='grid gap-4 sm:grid-cols-2'>
                  {(
                    [
                      'refreshToken',
                      'clientId',
                      'idToken',
                      'accountId',
                      'expiresAt',
                    ] as const
                  ).map((name) => (
                    <FormField
                      key={name}
                      control={form.control}
                      name={name}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t(`channels.upstream.${name}`)}
                          </FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type={
                                name === 'refreshToken' || name === 'idToken'
                                  ? 'password'
                                  : 'text'
                              }
                              autoComplete='off'
                              disabled={pending}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ))}
                </div>
              </>
            )}
          </div>
        )}
        <div className='flex justify-end'>
          <Button type='submit' size='sm' disabled={!hasUpdate || pending}>
            {t('channels.upstream.saveCredential')}
          </Button>
        </div>
      </form>
    </Form>
  )
}

export function UpstreamCredentials(props: {
  id: number
  active: boolean
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const query = useQuery({
    queryKey: upstreamKeys.credentials(props.id),
    queryFn: () => api.getImportedCredentials(props.id),
    enabled: props.active,
  })
  const [expanded, setExpanded] = useState<number[]>([])
  return (
    <div className='space-y-3'>
      {query.error && (
        <QueryErrorBanner
          error={query.error}
          messageKey='channels.imported.loadError'
          onRetry={() => query.refetch()}
          isRetrying={query.isFetching}
        />
      )}
      {query.isLoading && (
        <p className='text-muted-foreground py-6 text-center'>
          {t('channels.upstream.loading')}
        </p>
      )}
      {query.data?.items.map((credential) => (
        <article
          key={credential.id}
          className='overflow-hidden rounded-xl border'
        >
          <div className='flex items-center gap-3 p-4'>
            <span className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-lg'>
              <KeyRound className='text-muted-foreground size-4' />
            </span>
            <div className='min-w-0 flex-1'>
              <h3 className='truncate text-sm font-medium'>
                {credential.name}
              </h3>
              <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs'>
                <span>{credential.kind === 'oauth' ? 'OAuth' : 'API Key'}</span>
                {credential.expiresAt ? (
                  <span>
                    {t('channels.upstream.expires', {
                      value: formatDateTime(
                        credential.expiresAt * 1000,
                        toBcp47(i18n.language)
                      ),
                    })}
                  </span>
                ) : null}
                {credential.canRefresh && (
                  <span>{t('channels.upstream.refreshable')}</span>
                )}
              </div>
            </div>
            <Badge variant={credential.enabled ? 'success' : 'secondary'}>
              {t(
                credential.enabled
                  ? 'channels.imported.enabled'
                  : 'channels.imported.disabled'
              )}
            </Badge>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              aria-label={t('channels.upstream.editCredential', {
                name: credential.name,
              })}
              aria-expanded={expanded.includes(credential.id)}
              onClick={() =>
                setExpanded((ids) =>
                  ids.includes(credential.id)
                    ? ids.filter((id) => id !== credential.id)
                    : [...ids, credential.id]
                )
              }
            >
              <Pencil className='size-4' />
            </Button>
          </div>
          {/* Collapse only hides the form; unsaved replacement values survive until
          the drawer closes or the server confirms the replacement. */}
          <div hidden={!expanded.includes(credential.id)}>
            <CredentialForm
              credential={credential}
              onDirtyChange={props.onDirtyChange}
            />
          </div>
        </article>
      ))}
      {query.data?.items.length === 0 && (
        <p className='text-muted-foreground py-8 text-center'>
          {t('channels.upstream.noCredentials')}
        </p>
      )}
    </div>
  )
}
