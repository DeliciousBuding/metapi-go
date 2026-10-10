import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Pencil, Plus, Trash2, Unplug } from 'lucide-react'
import { useEffect, useState, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import type {
  ImportedCredential,
  ImportedCredentialUpdate,
} from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { credentialKindLabel, upstreamKeys } from '../lib/upstream-config'

const credentialSchema = z
  .object({
    name: z.string().trim().min(1, 'channels.upstream.required'),
    kind: z.enum(['api_key', 'oauth', 'none']),
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
          (Number.isFinite(new Date(value).getTime()) &&
            new Date(value).getTime() > 0),
        'channels.upstream.invalidExpiry'
      ),
  })
  .refine(
    (value) =>
      value.kind === 'none' || !value.replace || !!value.accessToken.trim(),
    {
      path: ['accessToken'],
      message: 'channels.upstream.required',
    }
  )
type CredentialValues = z.infer<typeof credentialSchema>

const credentialInputTypes = {
  refreshToken: 'password',
  clientId: 'text',
  idToken: 'password',
  accountId: 'text',
  expiresAt: 'datetime-local',
} as const

function CredentialForm(props: {
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
      accessToken: '',
      refreshToken: '',
      clientId: '',
      idToken: '',
      accountId: '',
      expiresAt: '',
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
      const patch: ImportedCredentialUpdate = {}
      if (!credential || form.formState.dirtyFields.name) {
        patch.name = values.name
      }
      if (!credential || form.formState.dirtyFields.enabled) {
        patch.enabled = values.enabled
      }
      if (values.replace) {
        if (values.kind === 'api_key') patch.apiKey = values.accessToken
        else if (values.kind === 'none') patch.kind = 'none'
        else {
          patch.oauth = {
            accessToken: values.accessToken,
            refreshToken: values.refreshToken || undefined,
            clientId: values.clientId || undefined,
            idToken: values.idToken || undefined,
            accountId: values.accountId || undefined,
            expiresAt: values.expiresAt
              ? new Date(values.expiresAt).getTime()
              : undefined,
          }
        }
      }
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
          ...values,
          name: credential ? values.name : '',
          enabled: credential ? values.enabled : false,
          replace: !credential,
          accessToken: '',
          refreshToken: '',
          clientId: '',
          idToken: '',
          accountId: '',
          expiresAt: '',
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
        {replace && kind !== 'none' && (
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

export function UpstreamCredentials(props: {
  id: number
  active: boolean
  allowAnonymous?: boolean
  onDirtyChange: (key: string, dirty: boolean) => void
  beforeDelete?: (action: () => void) => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const deletion = useUpstreamDeletion()
  const undoDelete = useUndoableDelete()
  const query = useQuery({
    queryKey: upstreamKeys.credentials(props.id),
    queryFn: () => api.getImportedCredentials(props.id),
    enabled: props.active,
  })
  const [expanded, setExpanded] = useState<number[]>([])
  const [creating, setCreating] = useState(false)
  const [creationStarted, setCreationStarted] = useState(false)
  async function remove(credential: ImportedCredential) {
    await deletion.requestDeletion(
      { kind: 'credential', id: credential.id, name: credential.name },
      (_, commit) => {
        undoDelete<{ items: ImportedCredential[] }, ImportedCredential>({
          item: credential,
          queryKey: upstreamKeys.credentials(props.id),
          removeFromCache: (data, item) => ({
            items: data.items.filter((row) => row.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.catalog.deleted', { name: credential.name }),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.catalog.deleteError'),
          alsoInvalidate: [upstreamKeys.all, ['routes']],
        })
      },
      () => {
        void client.invalidateQueries({ queryKey: upstreamKeys.all })
      }
    )
  }
  return (
    <div className='space-y-3'>
      <div className='flex justify-end'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => {
            setCreationStarted(true)
            setCreating((value) => !value)
          }}
          aria-expanded={creating}
        >
          <Plus className='size-4' />
          {t('channels.catalog.addCredential')}
        </Button>
      </div>
      <div hidden={!creating} className='overflow-hidden rounded-xl border'>
        {creationStarted && (
          <CredentialForm
            key={props.id}
            channelId={props.id}
            allowAnonymous={props.allowAnonymous}
            onDirtyChange={props.onDirtyChange}
            onCreated={() => {
              setCreating(false)
              setCreationStarted(false)
            }}
          />
        )}
      </div>
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
              {credential.kind === 'none' ? (
                <Unplug className='text-muted-foreground size-4' />
              ) : (
                <KeyRound className='text-muted-foreground size-4' />
              )}
            </span>
            <div className='min-w-0 flex-1'>
              <h3 className='truncate text-sm font-medium'>
                {credential.name}
              </h3>
              <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs'>
                <span>{credentialKindLabel(credential.kind)}</span>
                {credential.expiresAt ? (
                  <span>
                    {t('channels.upstream.expires', {
                      value: formatDateTime(
                        credential.expiresAt,
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
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('channels.catalog.deleteCredential', {
                name: credential.name,
              })}
              disabled={deletion.isPending}
              onClick={() => {
                const action = () => {
                  void remove(credential)
                }
                if (props.beforeDelete) props.beforeDelete(action)
                else action()
              }}
            >
              <Trash2 className='size-4' />
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
      {deletion.dialog}
    </div>
  )
}
