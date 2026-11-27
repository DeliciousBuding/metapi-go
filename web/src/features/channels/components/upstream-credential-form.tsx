import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import type { ImportedCredential } from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'

import { credentialKindLabel, upstreamKeys } from '../lib/upstream-config'
import {
  credentialSchema,
  credentialToPatch,
  emptyCredentialInputs,
  type CredentialValues,
} from './upstream-credential-values'

const credentialInputTypes = {
  refreshToken: 'password',
  clientId: 'text',
  idToken: 'password',
  accountId: 'text',
  projectId: 'text',
  expiresAt: 'datetime-local',
} as const

export function CredentialForm(props: {
  credential?: ImportedCredential
  channelId?: number
  allowAnonymous?: boolean
  onCreated?: () => void
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const credential = props.credential
  const defaultKind = props.allowAnonymous ? 'none' : 'api_key'
  const form = useForm<CredentialValues>({
    resolver: zodResolver(credentialSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      name: credential?.name ?? '',
      kind: credential?.kind ?? defaultKind,
      enabled: credential?.enabled ?? false,
      replace: !credential,
      ...emptyCredentialInputs,
    },
  })
  const replace = form.watch('replace')
  const kind = form.watch('kind')
  const [saving, setSaving] = useState(false)
  const submitting = useRef(false)
  const pending = saving || form.formState.isSubmitting
  const dirty = form.formState.isDirty
  const hasUpdate =
    replace ||
    form.formState.dirtyFields.name ||
    form.formState.dirtyFields.enabled
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    const key = `credential-${credential?.id ?? 'new'}`
    onDirtyChange(key, dirty)
    return () => onDirtyChange(key, false)
  }, [credential?.id, dirty, onDirtyChange])

  async function save(values: CredentialValues) {
    if (submitting.current) return
    if (!credential && values.kind === 'none' && !props.allowAnonymous) {
      form.setError('kind', {
        message: 'channels.upstream.anonymousUnavailable',
      })
      return
    }
    submitting.current = true
    setSaving(true)
    try {
      const patch = credentialToPatch(
        values,
        credential ? form.formState.dirtyFields : undefined
      )
      // Secret-bearing requests stay out of Query/Mutation caches.
      if (credential) {
        const result = await api.updateImportedCredential(credential.id, patch)
        if (!result.success) return
      } else if (props.channelId !== undefined) {
        await api.createUpstreamCredential(props.channelId, {
          ...patch,
          name: values.name,
        })
      }
      form.reset(
        {
          kind: values.kind,
          name: credential ? values.name : '',
          enabled: credential ? values.enabled : false,
          replace: !credential,
          ...emptyCredentialInputs,
        },
        { keepDirtyValues: false }
      )
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
      toast.success(t('channels.upstream.saved'))
      props.onCreated?.()
    } finally {
      submitting.current = false
      setSaving(false)
    }
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
        {!credential && (
          <FormField
            control={form.control}
            name='kind'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.catalog.credentialType')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={pending}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue>
                        {(value) =>
                          credentialKindLabel(
                            value as ImportedCredential['kind']
                          )
                        }
                      </SelectValue>
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='api_key'>API Key</SelectItem>
                    <SelectItem value='oauth'>OAuth</SelectItem>
                    <SelectItem value='gcp_service_account'>
                      {t('channels.upstream.gcpServiceAccount')}
                    </SelectItem>
                    {props.allowAnonymous && (
                      <SelectItem value='none'>
                        {t('channels.upstream.noAuthentication')}
                      </SelectItem>
                    )}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        {credential && credential.kind !== 'none' && (
          <FormField
            control={form.control}
            name='replace'
            render={({ field }) => (
              <FormItem className='bg-muted/40 flex items-center justify-between gap-3 rounded-lg p-3'>
                <FormLabel>
                  {t('channels.upstream.replaceCredential')}
                </FormLabel>
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
        )}
        {replace && kind === 'gcp_service_account' && (
          <FormField
            control={form.control}
            name='gcpServiceAccount'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('channels.upstream.gcpServiceAccount')}
                </FormLabel>
                <FormControl>
                  <textarea
                    {...field}
                    rows={8}
                    spellCheck={false}
                    autoComplete='off'
                    placeholder='{"type":"service_account",...}'
                    disabled={pending}
                    className='border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring min-h-32 w-full rounded-md border px-3 py-2 font-mono text-xs focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50'
                  />
                </FormControl>
                <p className='text-muted-foreground text-xs'>
                  {t('channels.upstream.gcpServiceAccountHint')}
                </p>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        {replace && kind !== 'none' && kind !== 'gcp_service_account' && (
          <div className='space-y-4'>
            <FormField
              control={form.control}
              name='accessToken'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {kind === 'oauth' ? 'Access token' : 'API Key'}
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
            {kind === 'oauth' && (
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
                      'projectId',
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
                              type={credentialInputTypes[name]}
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
            {t(
              credential
                ? 'channels.upstream.saveCredential'
                : 'channels.catalog.addCredential'
            )}
          </Button>
        </div>
      </form>
    </Form>
  )
}
