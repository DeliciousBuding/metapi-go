import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { api } from '@/lib/api'
import type {
  ImportedMember,
  ImportedUpstreamInventory,
} from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { upstreamKeys } from '../lib/upstream-config'
import { UpstreamGroupManager } from './upstream-group-manager'
import { UpstreamRouteBindingRow } from './upstream-route-binding-row'

export function UpstreamRouteBindings(props: {
  channelId?: number
  active?: boolean
  inventory?: ImportedUpstreamInventory
  beforeDelete?: (action: () => void) => void
  members: ImportedMember[]
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [expanded, setExpanded] = useState<number[]>([])
  const [clearing, setClearing] = useState<number | null>(null)
  const { requestDeletion, dialog, isPending } = useUpstreamDeletion()
  const undoableDelete = useUndoableDelete()
  function deleteMember(member: ImportedMember) {
    return requestDeletion(
      {
        kind: 'member',
        id: member.id,
        name: `${member.modelName} · ${member.credentialName}`,
      },
      (_preview, commit) =>
        undoableDelete<ImportedUpstreamInventory, ImportedMember>({
          item: member,
          queryKey: upstreamKeys.all,
          removeFromCache: (data, item) => ({
            ...data,
            members: data.members.filter((entry) => entry.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.group.memberDeleted'),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.group.deleteError'),
        }),
      () => props.onDirtyChange(`member-${member.id}`, false)
    )
  }
  async function clearCooldown(id: number) {
    setClearing(id)
    try {
      const result = await api.clearImportedMemberCooldown(id)
      if (result.success) {
        await client.invalidateQueries({ queryKey: upstreamKeys.all })
        await client.invalidateQueries({ queryKey: ['routes'] })
        toast.success(t('channels.upstream.cooldownCleared'))
      }
    } finally {
      setClearing(null)
    }
  }
  return (
    <div className='space-y-3'>
      {props.channelId !== undefined && (
        <UpstreamGroupManager
          channelId={props.channelId}
          active={props.active ?? false}
          inventory={props.inventory}
          onDirtyChange={props.onDirtyChange}
          beforeDelete={props.beforeDelete}
        />
      )}
      {props.channelId !== undefined && (
        <h3 className='pt-3 text-sm font-semibold'>
          {t('channels.group.members')}
        </h3>
      )}
      {props.members.map((member) => (
        <UpstreamRouteBindingRow
          key={member.id}
          member={member}
          expanded={expanded.includes(member.id)}
          clearing={clearing !== null}
          deleting={isPending}
          onDirtyChange={props.onDirtyChange}
          onToggle={() =>
            setExpanded((ids) =>
              ids.includes(member.id)
                ? ids.filter((id) => id !== member.id)
                : [...ids, member.id]
            )
          }
          onDelete={() => {
            const action = () => {
              void deleteMember(member)
            }
            if (props.beforeDelete) props.beforeDelete(action)
            else action()
          }}
          onClearCooldown={() => void clearCooldown(member.id).catch(() => {})}
        />
      ))}
      {!props.members.length && (
        <p className='text-muted-foreground py-8 text-center'>
          {t('channels.upstream.noRoutes')}
        </p>
      )}
      {dialog}
    </div>
  )
}
