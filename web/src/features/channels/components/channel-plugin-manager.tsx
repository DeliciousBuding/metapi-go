import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Puzzle, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  channelPluginQueryKey,
  channelPluginsApi,
  type ChannelPlugin,
} from '@/lib/api/channel-plugins'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { ChannelPluginEditor } from './channel-plugin-editor'
import { ChannelPluginHistory } from './channel-plugin-history'
import { ChannelPluginRow } from './channel-plugin-row'

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
  function remove(item: ChannelPlugin) {
    undoDelete<{ items: ChannelPlugin[] }, ChannelPlugin>({
      item,
      queryKey: channelPluginQueryKey,
      removeFromCache: (data, removed) => ({
        items: data.items.filter((current) => current.key !== removed.key),
      }),
      deleteFn: channelPluginsApi.remove,
      title: t('channels.plugins.removed', {
        name: item.manifest?.name ?? item.key,
      }),
      undoLabel: t('common.undo'),
      errorTitle: t('channels.plugins.removeError'),
    })
  }
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
          <ChannelPluginRow
            key={item.key}
            item={item}
            togglePending={toggle.isPending}
            onToggle={(enabled) => toggle.mutate({ item, enabled })}
            onEdit={() => setEditing(item.key)}
            onRemove={() => remove(item)}
          />
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
