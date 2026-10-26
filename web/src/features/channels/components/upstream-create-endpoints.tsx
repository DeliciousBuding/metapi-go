import { useEffect, useRef, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { connectionSchema, upstreamProtocols } from '../lib/upstream-config'
import type { UpstreamCreateValues } from '../lib/upstream-create'
import { resolvePresetEndpoints } from '../lib/upstream-preset'
import { UpstreamCapabilityPicker } from './upstream-capability-picker'
import { UpstreamEndpointsEditor } from './upstream-endpoints-editor'

export function UpstreamCreateEndpoints(props: {
  preset: UpstreamPreset
  disabled: boolean
  onResolvingChange: (resolving: boolean) => void
}) {
  const { t } = useTranslation()
  const form = useFormContext<UpstreamCreateValues>()
  const custom = form.watch('customEndpoints')
  const baseUrl = form.watch('baseUrl')
  const projectId = form.watch('projectId')
  const [presetState, setPresetState] = useState<
    'uninitialized' | 'draft' | 'prepared'
  >('uninitialized')
  const [resolving, setResolving] = useState(false)
  const request = useRef<AbortController | null>(null)
  const onResolvingChange = props.onResolvingChange
  useEffect(() => {
    onResolvingChange(resolving)
    return () => onResolvingChange(false)
  }, [resolving, onResolvingChange])
  useEffect(() => {
    request.current?.abort()
    setResolving(false)
    return () => request.current?.abort()
  }, [baseUrl, projectId])

  async function prepareEndpoints() {
    if (props.disabled || resolving) return
    form.clearErrors(['endpointConfig', 'baseUrl', 'projectId'])
    const originalBase = form.getValues('baseUrl')
    const originalProject = form.getValues('projectId')
    if (
      !connectionSchema.shape.baseUrl.safeParse(
        originalBase || props.preset.defaultUrl
      ).success
    ) {
      form.setError('baseUrl', { message: 'channels.upstream.invalidUrl' })
      return
    }
    if (props.preset.requiresProject && !originalProject.trim()) {
      form.setError('projectId', {
        message: 'channels.create.projectForPresetRequired',
      })
      return
    }
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setResolving(true)
    try {
      const endpoints = await resolvePresetEndpoints(
        props.preset,
        {
          baseUrl: originalBase,
          ...(props.preset.requiresProject
            ? { projectId: originalProject.trim() }
            : {}),
        },
        controller.signal
      )
      if (
        controller.signal.aborted ||
        originalBase !== form.getValues('baseUrl') ||
        originalProject !== form.getValues('projectId')
      ) {
        return
      }
      const selected = form.getValues('protocols')
      const defaults = Object.fromEntries(
        Object.entries(endpoints).filter(([key]) =>
          selected.includes(key as UpstreamPreset['protocols'][number])
        )
      )
      form.setValue(
        'endpointConfig',
        { ...defaults, ...form.getValues('endpointConfig') },
        {
          shouldDirty: true,
          shouldValidate: true,
        }
      )
      setPresetState('prepared')
    } catch {
      if (!controller.signal.aborted) {
        form.setError('endpointConfig', {
          message: 'channels.upstream.presetResolveFailed',
        })
      }
    } finally {
      if (!controller.signal.aborted) setResolving(false)
    }
  }

  return (
    <div className='space-y-3'>
      {custom ? (
        <FormField
          control={form.control}
          name='endpointConfig'
          render={({ field }) => (
            <FormItem>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <FormLabel>{t('channels.create.customEndpoints')}</FormLabel>
                <Button
                  type='button'
                  size='sm'
                  variant='ghost'
                  disabled={props.disabled}
                  onClick={() => {
                    request.current?.abort()
                    setResolving(false)
                    form.clearErrors('endpointConfig')
                    form.setValue('customEndpoints', false, {
                      shouldDirty: true,
                    })
                  }}
                >
                  {t('channels.create.useDefaultEndpoints')}
                </Button>
              </div>
              <FormDescription>
                {t('channels.create.customEndpointsHint')}
              </FormDescription>
              {resolving && (
                <p role='status' className='flex items-center gap-2 text-sm'>
                  <Spinner aria-hidden />
                  {t('channels.upstream.resolvingPreset')}
                </p>
              )}
              <FormControl>
                <div
                  ref={field.ref}
                  role='group'
                  aria-label={t('channels.create.customEndpoints')}
                >
                  <UpstreamEndpointsEditor
                    value={field.value}
                    onChange={field.onChange}
                    provider={props.preset.provider}
                    disabled={props.disabled || resolving}
                  />
                </div>
              </FormControl>
              <FormMessage />
              {presetState !== 'prepared' && (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  disabled={props.disabled || resolving}
                  onClick={() => void prepareEndpoints()}
                >
                  {t('channels.create.loadDefaultEndpoints')}
                </Button>
              )}
            </FormItem>
          )}
        />
      ) : (
        <FormField
          control={form.control}
          name='protocols'
          render={({ field }) => (
            <FormItem>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <FormLabel>{t('channels.create.protocols')}</FormLabel>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  disabled={props.disabled}
                  onClick={() => {
                    form.setValue('customEndpoints', true, {
                      shouldDirty: true,
                    })
                    if (presetState === 'uninitialized') {
                      setPresetState('draft')
                      void prepareEndpoints()
                    }
                  }}
                >
                  {t('channels.create.customizeEndpoints')}
                </Button>
              </div>
              <FormDescription>
                {t('channels.create.protocolsHint')}
              </FormDescription>
              <FormControl>
                <UpstreamCapabilityPicker
                  ref={field.ref}
                  onBlur={field.onBlur}
                  value={upstreamProtocols
                    .filter((protocol) => field.value.includes(protocol.key))
                    .map((protocol) => protocol.bit)}
                  available={upstreamProtocols
                    .filter((protocol) =>
                      props.preset.protocols.includes(protocol.key)
                    )
                    .map((protocol) => protocol.bit)}
                  onChange={(bits) =>
                    field.onChange(
                      upstreamProtocols
                        .filter((protocol) => bits.includes(protocol.bit))
                        .map((protocol) => protocol.key)
                    )
                  }
                  minSelected={1}
                  disabled={props.disabled}
                />
              </FormControl>
              {!props.preset.protocols.length && (
                <p className='text-muted-foreground text-sm'>
                  {t('channels.create.noProtocols')}
                </p>
              )}
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </div>
  )
}
