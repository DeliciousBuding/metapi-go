import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { isAxiosError } from 'axios'
import { ChevronDown, KeyRound } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import {
  upstreamCreateDefaults,
  upstreamCreatePayload,
  upstreamCreateSchema,
  type UpstreamCreateValues,
} from '../lib/upstream-create'
import { UpstreamPresetPicker } from './upstream-preset-picker'

export function UpstreamCreateSheet(props: {
  onClose: () => void
  onCreated: (id: number) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [selected, setSelected] = useState<UpstreamPreset | null>(null)
  const [advanced, setAdvanced] = useState(false)
  const [saving, setSaving] = useState(false)
  const submitting = useRef(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const form = useForm<UpstreamCreateValues>({
    resolver: zodResolver(upstreamCreateSchema(selected)),
    defaultValues: upstreamCreateDefaults,
  })
  const pending = saving || form.formState.isSubmitting
  const presets = useQuery({
    queryKey: ['upstream-presets'],
    queryFn: upstreamPresetsApi.getUpstreamPresets,
  })
  const credentialMode = selected?.credentialMode ?? 'apiKey'
  const requiresBaseUrl = selected?.requiresBaseUrl ?? !selected?.defaultUrl
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange: (open) => {
      if (!open) props.onClose()
    },
  })
  function selectPreset(preset: UpstreamPreset) {
    if (selected?.id === preset.id) return
    if (contentRef.current) contentRef.current.scrollTop = 0
    setSelected(preset)
    setAdvanced(false)
    form.reset({
      ...upstreamCreateDefaults,
      presetId: preset.id,
      baseUrl: preset.defaultUrl,
    })
  }
  async function submit(values: UpstreamCreateValues) {
    if (!selected || credentialMode === 'oauth' || submitting.current) return
    submitting.current = true
    setSaving(true)
    form.clearErrors('root')
    try {
      // Credential-bearing input stays out of Query and Mutation caches.
      const result = await api.connectUpstream(
        upstreamCreatePayload(values, selected)
      )
      form.reset({ ...values, apiKey: '', channelProxy: '' })
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
      const message = t(
        `channels.create.connected.${result.discovery.status}`,
        { count: result.modelCount }
      )
      if (result.discovery.status === 'empty') {
        toast.info(message, { description: result.discovery.message })
      } else toast.success(message, { description: result.discovery.message })
      props.onCreated(result.id)
    } catch (error) {
      const message = isAxiosError(error)
        ? error.response?.data?.error
        : undefined
      form.setError('root', {
        message:
          typeof message === 'string' ? message : t('channels.create.failed'),
      })
    } finally {
      submitting.current = false
      setSaving(false)
    }
  }
  return (
    <>
      <Sheet
        open
        onOpenChange={(open) => {
          if (!pending) handleOpenChange(open)
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
              onSubmit={form.handleSubmit(submit, (errors) => {
                if (
                  (!requiresBaseUrl && errors.baseUrl) ||
                  errors.name ||
                  errors.channelProxy
                ) {
                  setAdvanced(true)
                }
              })}
            >
              <div
                ref={contentRef}
                className='min-h-0 flex-1 space-y-5 overflow-y-auto p-5'
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
                  disabled={pending}
                  onSelect={selectPreset}
                />
                {selected && credentialMode !== 'oauth' && (
                  <div className='space-y-5 border-t pt-5'>
                    {requiresBaseUrl && (
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
                                placeholder='https://…'
                                disabled={pending}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}
                    <FormField
                      control={form.control}
                      name='apiKey'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t(
                              credentialMode === 'optional'
                                ? 'channels.create.optionalKey'
                                : 'channels.create.apiKey'
                            )}
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
                    <Collapsible
                      open={advanced}
                      onOpenChange={setAdvanced}
                      className='rounded-xl border'
                    >
                      <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center justify-between gap-3 rounded-xl px-4 py-3 text-sm font-medium focus-visible:outline-2'>
                        {t('channels.upstream.advanced')}
                        <ChevronDown
                          aria-hidden
                          className='size-4 transition-transform group-data-[panel-open]:rotate-180'
                        />
                      </CollapsibleTrigger>
                      <CollapsibleContent
                        keepMounted
                        className='space-y-4 border-t p-4'
                      >
                        <FormField
                          control={form.control}
                          name='name'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t('channels.create.optionalName')}
                              </FormLabel>
                              <FormControl>
                                <Input
                                  {...field}
                                  placeholder={selected.name}
                                  disabled={pending}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        {!requiresBaseUrl && (
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
                                    placeholder={selected.defaultUrl}
                                    disabled={pending}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        )}
                        <FormField
                          control={form.control}
                          name='useSystemProxy'
                          render={({ field }) => (
                            <FormItem className='flex items-center justify-between gap-3'>
                              <FormLabel>
                                {t('channels.upstream.systemProxy')}
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
                        <FormField
                          control={form.control}
                          name='channelProxy'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t('channels.upstream.channelProxy')}
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
                      </CollapsibleContent>
                    </Collapsible>
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
                  disabled={pending}
                  onClick={() => handleOpenChange(false)}
                >
                  {t('common.cancel')}
                </Button>
                {selected && credentialMode === 'oauth' ? (
                  <Button nativeButton={false} render={<Link to='/oauth' />}>
                    <KeyRound aria-hidden />
                    {t('channels.create.authorize')}
                  </Button>
                ) : (
                  <Button type='submit' disabled={!selected || pending}>
                    {pending && <Spinner aria-hidden />}
                    {t(
                      pending
                        ? 'channels.create.connecting'
                        : 'channels.create.submit'
                    )}
                  </Button>
                )}
              </SheetFooter>
            </form>
          </Form>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
