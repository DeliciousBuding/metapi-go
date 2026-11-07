import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Puzzle, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
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
  type ChannelPlugin,
} from '@/lib/api/channel-plugins'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { ChannelPluginHistory } from './channel-plugin-history'

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

export function ChannelPluginManager() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery({
    queryKey: channelPluginQueryKey,
    queryFn: channelPluginsApi.list,
  })
  const [editing, setEditing] = useState<string | null>(null)
  const undoDelete = useUndoableDelete()
  const toggle = useMutation({
    mutationFn: ({
      item,
      enabled,
    }: {
      item: ChannelPlugin
      enabled: boolean
    }) => channelPluginsApi.setEnabled(item, enabled),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: channelPluginQueryKey })
      toast.success(t('channels.plugins.saved'))
    },
  })
  const items = query.data?.items ?? []
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <p className='text-muted-foreground max-w-2xl text-sm'>
          {t('channels.plugins.description')}
        </p>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='icon'
            aria-label={t('channels.imported.refresh')}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw />
          </Button>
          <Button onClick={() => setEditing('')}>
            <Plus />
            {t('channels.plugins.install')}
          </Button>
        </div>
      </div>
      <QueryErrorBanner
        error={query.error}
        messageKey='channels.plugins.loadError'
        onRetry={() => void query.refetch()}
        isRetrying={query.isFetching}
      />
      {query.isPending && <p role='status'>{t('common.loading')}</p>}
      {!query.isPending && !query.isError && !items.length && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <Puzzle />
            </EmptyMedia>
            <EmptyTitle>{t('channels.plugins.empty')}</EmptyTitle>
            <EmptyDescription>
              {t('channels.plugins.emptyDescription')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {!query.isError &&
        items.map((item) => (
          <section
            key={item.key}
            className='flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3'
          >
            <div className='min-w-0 space-y-1'>
              <h3 className='text-sm font-medium'>
                {item.manifest?.name ?? item.key}
              </h3>
              <p className='text-muted-foreground text-xs'>
                {item.manifest &&
                  t(
                    item.manifest.apiVersion === 2
                      ? 'channels.plugins.providerType'
                      : 'channels.plugins.wireType'
                  )}{' '}
                · {item.key} ·{' '}
                {item.manifest?.version ?? t('channels.plugins.unavailable')} ·{' '}
                {t('channels.plugins.revision', { revision: item.revision })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {item.manifest?.protocols
                  .map((protocol) =>
                    t(`channels.capabilities.names.${protocol}`)
                  )
                  .join(' · ')}
              </p>
            </div>
            <div className='flex flex-wrap items-center gap-3'>
              <Badge variant={item.enabled ? 'secondary' : 'outline'}>
                {t(
                  item.loadError
                    ? 'channels.plugins.unavailable'
                    : item.enabled
                      ? 'channels.plugins.enabled'
                      : 'channels.plugins.disabled'
                )}
              </Badge>
              <Switch
                aria-label={t('channels.plugins.enableName', {
                  name: item.manifest?.name ?? item.key,
                })}
                checked={item.enabled}
                disabled={toggle.isPending || !!item.loadError}
                onCheckedChange={(enabled) => toggle.mutate({ item, enabled })}
              />
              <Button
                variant='outline'
                size='sm'
                onClick={() => setEditing(item.key)}
              >
                {t('channels.plugins.edit')}
              </Button>
              <Button
                variant='ghost'
                size='sm'
                disabled={item.enabled || toggle.isPending}
                onClick={() =>
                  undoDelete<{ items: ChannelPlugin[] }, ChannelPlugin>({
                    item,
                    queryKey: channelPluginQueryKey,
                    removeFromCache: (data, removed) => ({
                      items: data.items.filter(
                        (current) => current.key !== removed.key
                      ),
                    }),
                    deleteFn: channelPluginsApi.remove,
                    title: t('channels.plugins.removed', {
                      name: item.manifest?.name ?? item.key,
                    }),
                    undoLabel: t('common.undo'),
                    errorTitle: t('channels.plugins.removeError'),
                  })
                }
              >
                {t('channels.plugins.remove')}
              </Button>
            </div>
          </section>
        ))}
      <ChannelPluginHistory />
      {editing !== null && (
        <ChannelPluginEditor
          key={editing || 'new'}
          pluginKey={editing}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

function ChannelPluginEditor(props: {
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
  return (
    <>
      <Sheet open onOpenChange={handleOpenChange}>
        <SheetContent className='w-full sm:max-w-3xl'>
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
                          <FormLabel>{t('channels.plugins.source')}</FormLabel>
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
                          <FormLabel>{t('channels.plugins.enabled')}</FormLabel>
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
              )
            )}
          </div>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
