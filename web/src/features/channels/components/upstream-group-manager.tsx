import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import type { ImportedUpstreamInventory } from '@/lib/api/imported-upstreams'
import type { UpstreamGroup } from '@/lib/api/upstream-catalog'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { useUpstreamClock } from '../hooks/use-upstream-clock'
import { upstreamKeys } from '../lib/upstream-config'
import { GroupBindingForm } from './upstream-group-binding-form'
import { UpstreamGroupForm, type GroupGrantOption } from './upstream-group-form'
import { UpstreamGroupItem } from './upstream-group-item'

export function UpstreamGroupManager(props: {
  channelId: number
  active: boolean
  inventory?: ImportedUpstreamInventory
  onDirtyChange: (key: string, dirty: boolean) => void
  beforeDelete?: (action: () => void) => void
}) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [binding, setBinding] = useState(false)
  const [expanded, setExpanded] = useState<number[]>([])
  const now = useUpstreamClock(props.inventory?.members ?? [])
  const modelsQuery = useQuery({
    queryKey: upstreamKeys.models(props.channelId),
    queryFn: () => api.getUpstreamModels(props.channelId),
    enabled: props.active,
  })
  const credentialsQuery = useQuery({
    queryKey: upstreamKeys.credentials(props.channelId),
    queryFn: () => api.getImportedCredentials(props.channelId),
    enabled: props.active,
  })
  const groupsQuery = useQuery({
    queryKey: upstreamKeys.groups,
    queryFn: api.getUpstreamGroups,
    enabled: props.active,
  })
  const grants: GroupGrantOption[] = (modelsQuery.data?.items ?? []).flatMap(
    (model) =>
      model.grants.map((grant) => ({
        ...grant,
        label: `${model.name} · ${credentialsQuery.data?.items.find((item) => item.id === grant.credentialId)?.name ?? grant.credentialName}${model.enabled && grant.enabled && credentialsQuery.data?.items.find((item) => item.id === grant.credentialId)?.enabled ? '' : ` · ${t('channels.group.inactive')}`}`,
      }))
  )
  const groups = groupsQuery.data?.items ?? []
  const relatedGroups = groups.filter((group) =>
    group.members.some((member) => member.channelId === props.channelId)
  )
  const { requestDeletion, dialog, isPending } = useUpstreamDeletion()
  const undoableDelete = useUndoableDelete()
  function removeGroup(group: UpstreamGroup) {
    return requestDeletion(
      { kind: 'group', id: group.id, name: group.name },
      (_preview, commit) =>
        undoableDelete<{ items: UpstreamGroup[] }, UpstreamGroup>({
          item: group,
          queryKey: upstreamKeys.groups,
          removeFromCache: (data, item) => ({
            ...data,
            items: data.items.filter((entry) => entry.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.group.deleted'),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.group.deleteError'),
        }),
      () => {
        props.onDirtyChange(`group-${group.id}`, false)
        group.members.forEach((member) =>
          props.onDirtyChange(`member-${member.id}`, false)
        )
      }
    )
  }
  const queries = {
    models: modelsQuery,
    credentials: credentialsQuery,
    groups: groupsQuery,
  }
  const loading = Object.values(queries).some((query) => query.isLoading)
  const failed = Object.values(queries).some((query) => query.isError)
  return (
    <section className='space-y-3' aria-label={t('channels.group.title')}>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3 className='text-sm font-semibold'>{t('channels.group.title')}</h3>
        <div className='flex flex-wrap gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={loading || failed}
            aria-expanded={creating}
            onClick={() => setCreating((value) => !value)}
          >
            <Plus className='size-3.5' />
            {t('channels.group.create')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={loading || failed || !groups.length}
            aria-expanded={binding}
            onClick={() => setBinding((value) => !value)}
          >
            {t('channels.group.bind')}
          </Button>
        </div>
      </div>
      {Object.entries(queries).map(
        ([key, query]) =>
          query.error && (
            <QueryErrorBanner
              key={key}
              error={query.error}
              messageKey='channels.group.loadError'
              onRetry={() => void query.refetch()}
              isRetrying={query.isFetching}
            />
          )
      )}
      {loading && (
        <p className='text-muted-foreground text-sm'>
          {t('channels.upstream.loading')}
        </p>
      )}
      <div hidden={!creating} className='overflow-hidden rounded-xl border'>
        <UpstreamGroupForm
          grants={grants}
          onDirtyChange={props.onDirtyChange}
          onCreated={() => setCreating(false)}
        />
      </div>
      <div hidden={!binding}>
        <GroupBindingForm
          groups={groups}
          grants={grants}
          onDirtyChange={props.onDirtyChange}
          onSaved={() => setBinding(false)}
        />
      </div>
      {relatedGroups.map((group) => (
        <UpstreamGroupItem
          key={group.id}
          group={group}
          grants={grants}
          channelId={props.channelId}
          now={now}
          inventory={props.inventory}
          expanded={expanded.includes(group.id)}
          isPending={isPending}
          onDirtyChange={props.onDirtyChange}
          onToggle={() =>
            setExpanded((ids) =>
              ids.includes(group.id)
                ? ids.filter((id) => id !== group.id)
                : [...ids, group.id]
            )
          }
          onDelete={() => {
            const action = () => {
              void removeGroup(group)
            }
            if (props.beforeDelete) props.beforeDelete(action)
            else action()
          }}
        />
      ))}
      {dialog}
    </section>
  )
}
