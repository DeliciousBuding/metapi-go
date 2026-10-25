import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
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
import { Spinner } from '@/components/ui/spinner'
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import { upstreamPresetsApi } from '@/lib/api/upstream-presets'

import { connectionSchema } from '../lib/upstream-config'
import { resolvePresetEndpoints } from '../lib/upstream-preset'
import { UpstreamPresetPicker } from './upstream-preset-picker'

export function UpstreamPresetEndpoints(props: {
  baseUrl: string
  disabled: boolean
  onApply: (endpoints: ImportedEndpointConfig) => number
  onResolvingChange: (resolving: boolean) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [added, setAdded] = useState<number | null>(null)
  const request = useRef<AbortController | null>(null)
  const presets = useQuery({
    queryKey: ['upstream-presets'],
    queryFn: upstreamPresetsApi.getUpstreamPresets,
    enabled: open,
  })
  const schema = z.object({
    presetId: z.string().min(1, 'channels.create.selectProvider'),
    projectId: z.string().trim(),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { presetId: '', projectId: '' },
  })
  const presetId = form.watch('presetId')
  const projectId = form.watch('projectId')
  const selected = presets.data?.items.find((preset) => preset.id === presetId)
  const inputs = useRef({ baseUrl: props.baseUrl, presetId, projectId })
  inputs.current = { baseUrl: props.baseUrl, presetId, projectId }
  const clearErrors = form.clearErrors
  const onResolvingChange = props.onResolvingChange
  useEffect(() => {
    onResolvingChange(resolving)
    return () => onResolvingChange(false)
  }, [resolving, onResolvingChange])
  useEffect(() => {
    request.current?.abort()
    setResolving(false)
    setAdded(null)
    clearErrors('root')
    return () => request.current?.abort()
  }, [props.baseUrl, presetId, projectId, open, clearErrors])

  async function resolve(values: z.infer<typeof schema>) {
    if (!selected || props.disabled || resolving) return
    form.clearErrors()
    if (!connectionSchema.shape.baseUrl.safeParse(props.baseUrl).success) {
      form.setError('root', { message: 'channels.upstream.invalidUrl' })
      return
    }
    if (selected.requiresProject && !values.projectId) {
      form.setError('projectId', { message: 'channels.upstream.required' })
      return
    }
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    const original = inputs.current
    setResolving(true)
    setAdded(null)
    try {
      const endpoints = await resolvePresetEndpoints(
        selected,
        {
          baseUrl: props.baseUrl,
          ...(selected.requiresProject ? { projectId: values.projectId } : {}),
        },
        controller.signal
      )
      if (
        controller.signal.aborted ||
        original.baseUrl !== inputs.current.baseUrl ||
        original.presetId !== inputs.current.presetId ||
        original.projectId !== inputs.current.projectId
      ) {
        return
      }
      setAdded(props.onApply(endpoints))
    } catch {
      if (!controller.signal.aborted) {
        form.setError('root', {
          message: 'channels.upstream.presetResolveFailed',
        })
      }
    } finally {
      if (!controller.signal.aborted) setResolving(false)
    }
  }

  return (
    <div className='space-y-3'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled}
        aria-expanded={open}
        onClick={() => {
          request.current?.abort()
          setOpen((value) => !value)
        }}
      >
        {t('channels.upstream.fillPresetEndpoints')}
      </Button>
      {open && (
        <Form {...form}>
          <section
            className='space-y-4 rounded-xl border p-4'
            aria-label={t('channels.upstream.fillPresetEndpoints')}
            onKeyDown={(event) => {
              if (
                event.key === 'Enter' &&
                event.target instanceof HTMLInputElement
              ) {
                event.preventDefault()
                if (event.target.name === 'projectId')
                  void form.handleSubmit(resolve)()
              }
            }}
          >
            <p className='text-muted-foreground text-sm'>
              {t('channels.upstream.fillPresetHint')}
            </p>
            <QueryErrorBanner
              error={presets.error}
              messageKey='channels.create.presetsFailed'
              onRetry={() => void presets.refetch()}
              isRetrying={presets.isFetching}
            />
            {presets.isPending && (
              <p role='status' className='flex items-center gap-2 text-sm'>
                <Spinner aria-hidden />
                {t('channels.create.loading')}
              </p>
            )}
            {!presets.error && (
              <UpstreamPresetPicker
                presets={presets.data?.items ?? []}
                selectedId={presetId || null}
                disabled={props.disabled}
                onSelect={(preset) => {
                  if (preset.id === presetId) return
                  request.current?.abort()
                  form.reset({ presetId: preset.id, projectId: '' })
                }}
              />
            )}
            {presets.data?.items.length === 0 && (
              <p className='text-muted-foreground text-sm'>
                {t('channels.create.noPresets')}
              </p>
            )}
            {selected?.requiresProject && (
              <FormField
                control={form.control}
                name='projectId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('channels.create.projectId')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={props.disabled}
                        autoComplete='off'
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <Button
              type='button'
              size='sm'
              disabled={
                props.disabled || resolving || !selected || !!presets.error
              }
              onClick={() => void form.handleSubmit(resolve)()}
            >
              {resolving && <Spinner aria-hidden />}
              {t(
                resolving
                  ? 'channels.upstream.resolvingPreset'
                  : 'channels.upstream.addMissingEndpoints'
              )}
            </Button>
            {form.formState.errors.root && (
              <p role='alert' className='text-destructive text-sm'>
                {t(
                  form.formState.errors.root.message ??
                    'channels.upstream.presetResolveFailed'
                )}
              </p>
            )}
            {added !== null && (
              <p role='status' className='text-muted-foreground text-sm'>
                {t(
                  added
                    ? 'channels.upstream.presetEndpointsAdded'
                    : 'channels.upstream.noMissingEndpoints',
                  { count: added }
                )}
              </p>
            )}
          </section>
        </Form>
      )}
    </div>
  )
}
