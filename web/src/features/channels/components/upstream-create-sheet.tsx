import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import {
  upstreamPresetsApi,
  type UpstreamPreset,
} from '@/lib/api/upstream-presets'

import { connectionSchema, upstreamKeys } from '../lib/upstream-config'
import {
  upstreamCreateDefaults,
  upstreamCreatePayload,
  upstreamCreateSchema,
  type UpstreamCreateValues,
} from '../lib/upstream-create'
import { UpstreamEndpointsEditor } from './upstream-endpoints-editor'
import { UpstreamPresetPicker } from './upstream-preset-picker'

export function UpstreamCreateSheet(props: {
  onClose: () => void
  onCreated: (id: number) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<UpstreamCreateValues>({
    resolver: zodResolver(upstreamCreateSchema),
    defaultValues: upstreamCreateDefaults,
  })
  const presets = useQuery({
    queryKey: ['upstream-presets'],
    queryFn: upstreamPresetsApi.getUpstreamPresets,
  })
  const create = useMutation({ mutationFn: api.createUpstreamChannel })
  const [selected, setSelected] = useState<UpstreamPreset | null>(null)
  const [manual, setManual] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [resolvedFor, setResolvedFor] = useState('')
  const requestVersion = useRef(0)
  const selectedName = useRef('')
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(
    () => () => {
      requestVersion.current += 1
    },
    []
  )
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange: (open) => {
      if (!open) props.onClose()
    },
  })

  async function resolve(
    preset: UpstreamPreset,
    rawBaseUrl: string,
    force = false
  ) {
    const baseUrl = rawBaseUrl.trim()
    const key = `${preset.id}\n${baseUrl}`
    if ((!force && resolvedFor === key) || !baseUrl) return
    if (!connectionSchema.shape.baseUrl.safeParse(baseUrl).success) {
      form.setError('baseUrl', { message: 'channels.upstream.invalidUrl' })
      return
    }
    const version = ++requestVersion.current
    setResolving(true)
    setResolvedFor('')
    form.setValue('endpointConfig', {}, { shouldDirty: true })
    form.clearErrors(['baseUrl', 'endpointConfig'])
    try {
      const result = await upstreamPresetsApi.resolveUpstreamPreset({
        presetId: preset.id,
        baseUrl,
      })
      if (version !== requestVersion.current) return
      form.setValue('provider', result.provider, { shouldDirty: true })
      form.setValue('endpointConfig', result.endpointConfig, {
        shouldDirty: true,
        shouldValidate: true,
      })
      setResolvedFor(key)
    } catch {
      if (version === requestVersion.current) {
        form.setError('endpointConfig', {
          message: 'channels.create.resolveFailed',
        })
      }
    } finally {
      if (version === requestVersion.current) setResolving(false)
    }
  }

  function selectPreset(preset: UpstreamPreset, name: string) {
    if (contentRef.current) contentRef.current.scrollTop = 0
    if (selected?.id === preset.id) return
    requestVersion.current += 1
    setSelected(preset)
    setManual(false)
    setResolvedFor('')
    setResolving(false)
    if (
      !form.getValues('name') ||
      form.getValues('name') === selectedName.current
    ) {
      form.setValue('name', name, { shouldDirty: true })
    }
    selectedName.current = name
    form.setValue('provider', preset.provider, { shouldDirty: true })
    form.setValue('baseUrl', preset.defaultUrl, { shouldDirty: true })
    form.setValue('endpointConfig', {}, { shouldDirty: true })
    form.clearErrors()
    if (preset.defaultUrl) void resolve(preset, preset.defaultUrl, true)
  }

  function selectCustom() {
    if (contentRef.current) contentRef.current.scrollTop = 0
    if (manual) return
    requestVersion.current += 1
    setSelected(null)
    setManual(true)
    setResolvedFor('')
    setResolving(false)
    form.setValue('provider', 'openai_compatible', { shouldDirty: true })
    form.setValue('endpointConfig', {}, { shouldDirty: true })
    form.clearErrors()
  }

  async function submit(values: UpstreamCreateValues) {
    if (
      selected &&
      resolvedFor !== `${selected.id}\n${values.baseUrl.trim()}`
    ) {
      form.setError('endpointConfig', {
        message: 'channels.create.resolveRequired',
      })
      return
    }
    try {
      const result = await create.mutateAsync(upstreamCreatePayload(values))
      form.reset(values)
      void client.invalidateQueries({ queryKey: upstreamKeys.all })
      props.onCreated(result.id)
    } catch {
      form.setError('root', { message: t('channels.create.failed') })
    }
  }

  const configured = selected !== null || manual
  return (
    <>
      <Sheet
        open
        onOpenChange={(open) => {
          if (!create.isPending) handleOpenChange(open)
        }}
      >
        <SheetContent
          className='flex w-full flex-col gap-0 p-0 sm:max-w-2xl'
          showMobileCloseBar={false}
        >
          <SheetHeader className='shrink-0 border-b p-5 pr-12'>
            <SheetTitle>{t('channels.create.title')}</SheetTitle>
            <SheetDescription className='sr-only'>
              {t('channels.create.description')}
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              className='flex min-h-0 flex-1 flex-col'
              onSubmit={form.handleSubmit(submit)}
            >
              <div
                ref={contentRef}
                className='min-h-0 flex-1 space-y-6 overflow-y-auto p-5'
              >
                <QueryErrorBanner
                  error={presets.error}
                  messageKey='channels.create.presetsFailed'
                  onRetry={() => void presets.refetch()}
                  isRetrying={presets.isFetching}
                />
                {presets.isPending && (
                  <div
                    role='status'
                    className='flex items-center gap-2 text-sm'
                  >
                    <Spinner aria-hidden />
                    {t('channels.create.loading')}
                  </div>
                )}
                <UpstreamPresetPicker
                  presets={presets.data?.items ?? []}
                  selectedId={selected?.id ?? null}
                  manual={manual}
                  disabled={create.isPending}
                  onSelect={selectPreset}
                  onCustom={selectCustom}
                />
                {configured && (
                  <div className='space-y-5 border-t pt-5'>
                    <FormField
                      control={form.control}
                      name='name'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('channels.upstream.name')}</FormLabel>
                          <FormControl>
                            <Input {...field} disabled={create.isPending} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    {manual && (
                      <FormField
                        control={form.control}
                        name='provider'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('channels.create.provider')}
                            </FormLabel>
                            <FormControl>
                              <Input {...field} disabled={create.isPending} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}
                    <FormField
                      control={form.control}
                      name='baseUrl'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('channels.upstream.baseUrl')}
                          </FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              disabled={create.isPending}
                              placeholder='https://…'
                              onChange={(event) => {
                                field.onChange(event)
                                requestVersion.current += 1
                                setResolvedFor('')
                                setResolving(false)
                                form.setValue(
                                  'endpointConfig',
                                  {},
                                  { shouldDirty: true }
                                )
                                form.clearErrors(['baseUrl', 'endpointConfig'])
                              }}
                              onBlur={() => {
                                field.onBlur()
                                if (selected) {
                                  void resolve(
                                    selected,
                                    form.getValues('baseUrl')
                                  )
                                }
                              }}
                            />
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
                          <div className='flex items-center justify-between gap-3'>
                            <FormLabel>
                              {t('channels.imported.endpoints')}
                            </FormLabel>
                            {selected && (
                              <Button
                                type='button'
                                size='xs'
                                variant='ghost'
                                disabled={
                                  create.isPending ||
                                  resolving ||
                                  !form.getValues('baseUrl')
                                }
                                onClick={() =>
                                  void resolve(
                                    selected,
                                    form.getValues('baseUrl'),
                                    true
                                  )
                                }
                              >
                                {resolving && <Spinner aria-hidden />}
                                {t('channels.create.resolve')}
                              </Button>
                            )}
                          </div>
                          <FormControl>
                            <div role='group' ref={field.ref} tabIndex={-1}>
                              <UpstreamEndpointsEditor
                                value={field.value}
                                onChange={field.onChange}
                                provider={form.watch('provider')}
                                disabled={create.isPending || resolving}
                              />
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <div className='flex flex-wrap gap-x-8 gap-y-4'>
                      {(['enabled', 'useSystemProxy'] as const).map((name) => (
                        <FormField
                          key={name}
                          control={form.control}
                          name={name}
                          render={({ field }) => (
                            <FormItem className='flex items-center gap-3'>
                              <FormLabel>
                                {t(
                                  name === 'enabled'
                                    ? 'channels.create.enabled'
                                    : 'channels.upstream.systemProxy'
                                )}
                              </FormLabel>
                              <FormControl>
                                <Switch
                                  checked={field.value}
                                  onCheckedChange={field.onChange}
                                  disabled={create.isPending}
                                />
                              </FormControl>
                            </FormItem>
                          )}
                        />
                      ))}
                    </div>
                  </div>
                )}
                {form.formState.errors.root && (
                  <p role='alert' className='text-destructive text-sm'>
                    {form.formState.errors.root.message}
                  </p>
                )}
              </div>
              <SheetFooter className='shrink-0 flex-row justify-end gap-2 border-t p-5'>
                <Button
                  type='button'
                  variant='outline'
                  disabled={create.isPending}
                  onClick={() => handleOpenChange(false)}
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  type='submit'
                  disabled={!configured || create.isPending || resolving}
                >
                  {create.isPending && <Spinner aria-hidden />}
                  {t('channels.create.submit')}
                </Button>
              </SheetFooter>
            </form>
          </Form>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
