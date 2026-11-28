import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useDeferredValue, useEffect, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  channelPluginQueryKey,
  channelPluginsApi,
} from '@/lib/api/channel-plugins'
import { toast } from '@/lib/toast'

import { ChannelPluginMetaPreview } from './channel-plugin-meta-preview'

const sourceSchema = z.object({
  key: z
    .string()
    .regex(/^[a-z][a-z0-9-]{0,63}$/, 'channels.plugins.invalidKey'),
  source: z
    .string()
    .min(1, 'channels.plugins.requiredSource')
    .max(256 * 1024),
  enabled: z.boolean(),
  expectedRevision: z.number().int().nonnegative(),
})
type SourceValues = z.infer<typeof sourceSchema>
const exampleSource = `export const meta = {
  apiVersion: 1, key: "vendor-options", name: "Vendor options", version: "1.0.0",
  protocols: ["chat"], hooks: ["request"],
  options: [{key: "mode", label: "Mode", type: "select", default: "safe", choices: ["safe", "fast"]}],
};
export function request(input) {
  return {body: [{op: "set", path: "vendor.mode", value: input.options.mode}]};
}
`

export function ChannelPluginEditor(props: {
  pluginKey: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<SourceValues>({
    resolver: zodResolver(sourceSchema),
    defaultValues: {
      key: props.pluginKey || 'vendor-options',
      source: props.pluginKey ? '' : exampleSource,
      enabled: false,
      expectedRevision: 0,
    },
  })
  const [loading, setLoading] = useState(!!props.pluginKey)
  const [loadError, setLoadError] = useState<Error | null>(null)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    if (!props.pluginKey) return
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    void channelPluginsApi
      .detail(props.pluginKey, controller.signal)
      .then((row) => {
        if (controller.signal.aborted) return
        form.reset({
          key: row.key,
          source: row.source,
          enabled: row.enabled,
          expectedRevision: row.revision,
        })
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted)
          setLoadError(
            error instanceof Error ? error : new Error('Plugin read failed')
          )
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [props.pluginKey, form, retry])
  const pending = form.formState.isSubmitting
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onOpenChange: (open) => {
      if (!open && !pending) props.onClose()
    },
  })
  async function save(values: SourceValues) {
    await channelPluginsApi.save(values.key, {
      source: values.source,
      enabled: values.enabled,
      expectedRevision: values.expectedRevision,
    })
    form.reset(values)
    await client.invalidateQueries({ queryKey: channelPluginQueryKey })
    toast.success(t('channels.plugins.saved'))
    props.onClose()
  }
  const watchedSource = useWatch({ control: form.control, name: 'source' })
  const watchedKey = useWatch({ control: form.control, name: 'key' })
  const deferredSource = useDeferredValue(watchedSource ?? '')
  return (
    <>
      <Sheet open onOpenChange={handleOpenChange}>
        <SheetContent className='w-full sm:max-w-5xl'>
          <SheetHeader>
            <SheetTitle>
              {t(
                props.pluginKey
                  ? 'channels.plugins.edit'
                  : 'channels.plugins.install'
              )}
            </SheetTitle>
            <SheetDescription>
              {t('channels.plugins.sourceDescription')}
            </SheetDescription>
          </SheetHeader>
          <div className='min-h-0 flex-1 overflow-auto p-5'>
            <QueryErrorBanner
              error={loadError}
              messageKey='channels.plugins.loadError'
              onRetry={() => setRetry((value) => value + 1)}
              isRetrying={loading}
            />
            {loading ? (
              <p role='status'>{t('common.loading')}</p>
            ) : (
              !loadError && (
                <div className='grid gap-6 lg:grid-cols-[minmax(0,1.25fr)_minmax(280px,1fr)]'>
                  <Form {...form}>
                    <form
                      className='space-y-5'
                      onSubmit={form.handleSubmit((values) =>
                        save(values).catch(() => {})
                      )}
                    >
                      <FormField
                        control={form.control}
                        name='key'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('channels.plugins.key')}</FormLabel>
                            <FormControl>
                              <Input
                                {...field}
                                disabled={!!props.pluginKey || pending}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='source'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('channels.plugins.source')}
                            </FormLabel>
                            <FormDescription>
                              {t('channels.plugins.sourceLimits')}
                            </FormDescription>
                            <FormControl>
                              <Textarea
                                {...field}
                                spellCheck={false}
                                autoComplete='off'
                                className='min-h-72 font-mono text-xs'
                                disabled={pending}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='enabled'
                        render={({ field }) => (
                          <FormItem className='flex items-center gap-3'>
                            <FormLabel>
                              {t('channels.plugins.enabled')}
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
                      <div className='flex justify-end gap-2'>
                        <Button
                          type='button'
                          variant='outline'
                          disabled={pending}
                          onClick={() => handleOpenChange(false)}
                        >
                          {t('common.cancel')}
                        </Button>
                        <Button
                          type='submit'
                          disabled={
                            pending ||
                            (!form.formState.isDirty && !!props.pluginKey)
                          }
                        >
                          {t('common.save')}
                        </Button>
                      </div>
                    </form>
                  </Form>
                  <ChannelPluginMetaPreview
                    source={deferredSource}
                    pluginKey={watchedKey ?? ''}
                  />
                </div>
              )
            )}
          </div>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
