import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { LockKeyhole, SlidersHorizontal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { StringMapEditor } from '@/components/common/string-map-editor'
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
import { api } from '@/lib/api'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'

import {
  connectionPatch,
  connectionSchema,
  connectionValues,
  headersForEditor,
  upstreamKeys,
  type ConnectionValues,
} from '../lib/upstream-config'
import { UpstreamEndpointsEditor } from './upstream-endpoints-editor'

export function UpstreamConnectionForm(props: {
  detail: ImportedUpstreamDetail
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<ConnectionValues>({
    resolver: zodResolver(connectionSchema),
    defaultValues: connectionValues(props.detail),
  })
  const [configState, setConfigState] = useState<
    'locked' | 'loading' | 'ready'
  >('locked')
  const request = useRef<AbortController | null>(null)
  const dirty = form.formState.isDirty
  const pending = form.formState.isSubmitting
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange('connection', dirty)
    return () => onDirtyChange('connection', false)
  }, [dirty, onDirtyChange])
  useEffect(
    () => () => {
      request.current?.abort()
    },
    []
  )

  async function loadRequestConfig() {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setConfigState('loading')
    try {
      const config = await api.getImportedRequestConfig(
        props.detail.id,
        controller.signal
      )
      if (controller.signal.aborted) return
      form.resetField('channelProxy', { defaultValue: config.channelProxy })
      form.resetField('customHeaders', {
        defaultValue: headersForEditor(config.customHeaders),
      })
      form.resetField('paramOverride', { defaultValue: config.paramOverride })
      setConfigState('ready')
    } catch {
      if (!controller.signal.aborted) setConfigState('locked')
    }
  }

  async function save(values: ConnectionValues) {
    const result = await api.updateImportedUpstream(
      props.detail.id,
      connectionPatch(values, form.formState.dirtyFields)
    )
    if (!result.success) return
    form.reset(values)
    await client.invalidateQueries({ queryKey: upstreamKeys.all })
    toast.success(t('channels.upstream.saved'))
  }

  return (
    <Form {...form}>
      <form
        className='space-y-6'
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
            name='useSystemProxy'
            render={({ field }) => (
              <FormItem className='flex items-center gap-3 sm:pt-6'>
                <FormLabel>{t('channels.upstream.systemProxy')}</FormLabel>
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
          name='baseUrl'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.baseUrl')}</FormLabel>
              <FormControl>
                <Input {...field} disabled={pending} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='endpointConfig'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.imported.endpoints')}</FormLabel>
              <FormControl>
                <div className='min-w-0' role='group' ref={field.ref}>
                  <UpstreamEndpointsEditor
                    value={field.value}
                    onChange={field.onChange}
                    provider={props.detail.provider}
                    disabled={pending}
                  />
                </div>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {!Object.keys(props.detail.endpointConfig ?? {}).length && (
          <div className='grid gap-3 sm:grid-cols-2'>
            {(
              [
                'openaiChatCompletionPath',
                'openaiResponsePath',
                'anthropicMessagePath',
              ] as const
            ).map((name) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t(`channels.upstream.${name}`)}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={pending} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
        )}
        <section className='space-y-4 rounded-xl border p-4'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <h3 className='flex items-center gap-2 text-sm font-semibold'>
              <SlidersHorizontal className='text-muted-foreground size-4' />
              {t('channels.upstream.requestConfig')}
            </h3>
            {configState !== 'ready' && (
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={configState === 'loading'}
                onClick={() => void loadRequestConfig()}
              >
                <LockKeyhole className='size-4' />
                {t('channels.upstream.editRequestConfig')}
              </Button>
            )}
          </div>
          {configState !== 'ready' && (
            <div className='text-muted-foreground flex flex-wrap gap-x-5 gap-y-2 text-xs'>
              <span>
                {t('channels.upstream.channelProxy')} ·{' '}
                {t(
                  props.detail.hasChannelProxy
                    ? 'channels.upstream.configured'
                    : 'channels.upstream.unset'
                )}
              </span>
              <span>
                {t('channels.upstream.customHeaders')} ·{' '}
                {t(
                  props.detail.hasCustomHeaders
                    ? 'channels.upstream.configured'
                    : 'channels.upstream.unset'
                )}
              </span>
              <span>
                {t('channels.upstream.paramOverride')} ·{' '}
                {t(
                  props.detail.hasParamOverride
                    ? 'channels.upstream.configured'
                    : 'channels.upstream.unset'
                )}
              </span>
            </div>
          )}
          {/* Registered while locked so explicit reads can replace defaults without
          marking untouched secrets dirty or wiping other field drafts. */}
          <div hidden={configState !== 'ready'} className='space-y-4'>
            <FormField
              control={form.control}
              name='channelProxy'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('channels.upstream.channelProxy')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='password'
                      autoComplete='off'
                      disabled={pending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='customHeaders'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('channels.upstream.customHeaders')}</FormLabel>
                  <FormControl>
                    <StringMapEditor
                      {...field}
                      ordered
                      structuredOnly
                      disabled={pending}
                      keyLabel={t('stringMapEditor.headerName')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='paramOverride'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('channels.upstream.paramOverride')}</FormLabel>
                  <FormControl>
                    <StringMapEditor {...field} disabled={pending} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </section>
        <div className='bg-background/95 sticky bottom-0 -mx-1 flex justify-end border-t px-1 py-3'>
          <Button type='submit' disabled={!dirty || pending}>
            {t('channels.upstream.saveConnection')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
