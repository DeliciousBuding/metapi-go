import { LockKeyhole, SlidersHorizontal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { StringMapEditor } from '@/components/common/string-map-editor'
import { Button } from '@/components/ui/button'
import {
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
  pluginBindingSchema,
  providerBindingSchema,
} from '@/lib/api/channel-plugins'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'

import { headersForEditor, type ConnectionValues } from '../lib/upstream-config'
import { ChannelPluginBindings } from './channel-plugin-bindings'
import { ChannelProviderBinding } from './channel-provider-binding'

export function UpstreamRequestConfig({
  detail,
}: {
  detail: ImportedUpstreamDetail
}) {
  const { t } = useTranslation()
  const form = useFormContext<ConnectionValues>()
  const pending = form.formState.isSubmitting
  const [configState, setConfigState] = useState<
    'locked' | 'loading' | 'ready'
  >('locked')
  const request = useRef<AbortController | null>(null)
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
        detail.id,
        controller.signal
      )
      if (controller.signal.aborted) return
      const providerBinding = providerBindingSchema
        .nullable()
        .parse(config.providerBinding ?? null)
      const pluginBindings = pluginBindingSchema
        .array()
        .parse(config.pluginBindings ?? [])
      form.resetField('channelProxy', { defaultValue: config.channelProxy })
      form.resetField('customHeaders', {
        defaultValue: headersForEditor(config.customHeaders),
      })
      form.resetField('paramOverride', { defaultValue: config.paramOverride })
      form.resetField('pluginBindings', {
        defaultValue: pluginBindings,
      })
      form.resetField('providerBinding', { defaultValue: providerBinding })
      setConfigState('ready')
    } catch {
      if (!controller.signal.aborted) setConfigState('locked')
    }
  }
  return (
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
              detail.hasChannelProxy
                ? 'channels.upstream.configured'
                : 'channels.upstream.unset'
            )}
          </span>
          <span>
            {t('channels.upstream.customHeaders')} ·{' '}
            {t(
              detail.hasCustomHeaders
                ? 'channels.upstream.configured'
                : 'channels.upstream.unset'
            )}
          </span>
          <span>
            {t('channels.upstream.paramOverride')} ·{' '}
            {t(
              detail.hasParamOverride
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
        <FormField
          control={form.control}
          name='providerBinding'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.plugins.providerBinding')}</FormLabel>
              <FormDescription>
                {t('channels.plugins.providerDescription')}
              </FormDescription>
              <FormControl>
                <ChannelProviderBinding
                  {...field}
                  value={field.value ?? null}
                  protocols={Object.entries(
                    form.watch('endpointConfig')
                  ).flatMap(([key, endpoint]) =>
                    endpoint
                      ? [
                          key as keyof NonNullable<
                            ImportedUpstreamDetail['endpointConfig']
                          >,
                        ]
                      : []
                  )}
                  disabled={pending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='pluginBindings'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.plugins.bindings')}</FormLabel>
              <FormDescription>
                {t('channels.plugins.bindingDescription')}
              </FormDescription>
              <FormControl>
                <ChannelPluginBindings
                  {...field}
                  value={field.value ?? []}
                  endpoints={form.watch('endpointConfig')}
                  disabled={pending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </section>
  )
}
