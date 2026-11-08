import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import {
  channelPluginVersionsApi,
  channelPluginVersionsQueryKey,
  type ChannelPluginVersion,
} from '@/lib/api/channel-plugin-versions'
import { toast } from '@/lib/toast'

export function ChannelPluginHistory() {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [restoring, setRestoring] = useState<ChannelPluginVersion | null>(null)
  const query = useQuery({
    queryKey: channelPluginVersionsQueryKey,
    queryFn: channelPluginVersionsApi.list,
    enabled: expanded,
  })
  return (
    <section className='space-y-3 border-t pt-4'>
      <Button
        variant='outline'
        aria-expanded={expanded}
        aria-controls='channel-plugin-history'
        onClick={() => setExpanded((value) => !value)}
      >
        {t('channels.plugins.historyTitle')}
      </Button>
      {expanded && (
        <div id='channel-plugin-history' className='space-y-3'>
          <p className='text-muted-foreground max-w-2xl text-sm'>
            {t('channels.plugins.historyDescription')}
          </p>
          <QueryErrorBanner
            error={query.error}
            messageKey='channels.plugins.historyLoadError'
            onRetry={() => void query.refetch()}
            isRetrying={query.isFetching}
          />
          {query.isPending && <p role='status'>{t('common.loading')}</p>}
          {!query.isPending && !query.isError && !query.data?.items.length && (
            <p className='text-muted-foreground text-sm'>
              {t('channels.plugins.historyEmpty')}
            </p>
          )}
          {!query.isError &&
            query.data?.items.map((version) => (
              <section
                key={`${version.key}:${version.fingerprint}`}
                className='space-y-2 rounded-xl border p-3'
              >
                <h3 className='text-sm font-medium'>{version.key}</h3>
                <p className='text-muted-foreground font-mono text-xs break-all'>
                  {version.fingerprint}
                </p>
                <div className='flex flex-wrap items-center gap-3'>
                  <Badge
                    variant={version.available ? 'secondary' : 'destructive'}
                  >
                    {t(
                      version.available
                        ? 'channels.plugins.historyStored'
                        : 'channels.plugins.historyMissing'
                    )}
                  </Badge>
                  <span className='text-muted-foreground text-xs'>
                    {t('channels.plugins.historyReferences', {
                      count: version.taskReferences,
                    })}
                  </span>
                  {!version.available && version.taskReferences > 0 && (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setRestoring(version)}
                    >
                      {t('channels.plugins.historyRestore')}
                    </Button>
                  )}
                </div>
              </section>
            ))}
        </div>
      )}
      {restoring && (
        <RestorePluginVersion
          key={restoring.fingerprint}
          version={restoring}
          onClose={() => setRestoring(null)}
        />
      )}
    </section>
  )
}

const restoreSchema = z.object({
  source: z
    .string()
    .min(1, 'channels.plugins.requiredSource')
    .max(256 * 1024),
})

function RestorePluginVersion({
  version,
  onClose,
}: {
  version: ChannelPluginVersion
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<z.infer<typeof restoreSchema>>({
    resolver: zodResolver(restoreSchema),
    defaultValues: { source: '' },
  })
  const restore = useMutation({
    mutationFn: ({ source }: z.infer<typeof restoreSchema>) =>
      channelPluginVersionsApi.restore(version, source),
    onSuccess: async () => {
      await client.invalidateQueries({
        queryKey: channelPluginVersionsQueryKey,
      })
      form.reset()
      toast.success(t('channels.plugins.historyRestored'))
      onClose()
    },
  })
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onOpenChange: (open) => {
      if (!open && !restore.isPending) onClose()
    },
  })
  return (
    <Sheet open onOpenChange={handleOpenChange}>
      <SheetContent className='overflow-y-auto sm:max-w-2xl'>
        <SheetHeader>
          <SheetTitle>{t('channels.plugins.historyRestore')}</SheetTitle>
          <SheetDescription>
            {t('channels.plugins.historyRestoreDescription')}
          </SheetDescription>
        </SheetHeader>
        <div className='space-y-4 p-4'>
          <p className='text-sm font-medium'>{version.key}</p>
          <p className='font-mono text-xs break-all'>{version.fingerprint}</p>
          <Form {...form}>
            <form
              className='space-y-4'
              onSubmit={form.handleSubmit((values) => restore.mutate(values))}
            >
              <FormField
                control={form.control}
                name='source'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('channels.plugins.historySource')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={16}
                        className='font-mono text-xs'
                        spellCheck={false}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button type='submit' disabled={restore.isPending}>
                {t('channels.plugins.historyRestore')}
              </Button>
            </form>
          </Form>
        </div>
        {guard}
      </SheetContent>
    </Sheet>
  )
}
