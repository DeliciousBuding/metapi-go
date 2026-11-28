import { Link } from '@tanstack/react-router'
import { Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ImportedUpstreamInventory } from '@/lib/api/imported-upstreams'
import type { UpstreamGroup } from '@/lib/api/upstream-catalog'

import { UpstreamGroupForm, type GroupGrantOption } from './upstream-group-form'
import { UpstreamRouteTopology } from './upstream-route-topology'

export function UpstreamGroupItem({
  group,
  grants,
  channelId,
  now,
  inventory,
  expanded,
  isPending,
  onToggle,
  onDelete,
  onDirtyChange,
}: {
  group: UpstreamGroup
  grants: GroupGrantOption[]
  channelId: number
  now: number
  inventory?: ImportedUpstreamInventory
  expanded: boolean
  isPending: boolean
  onToggle: () => void
  onDelete: () => void
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <article className='overflow-hidden rounded-xl border'>
      <div className='flex flex-wrap items-center justify-between gap-3 p-4'>
        <div className='min-w-0 flex-1'>
          <p className='text-sm font-semibold break-words'>{group.name}</p>
          <Link
            to='/token-routes'
            search={{ routeId: group.routeId }}
            className='text-muted-foreground text-xs break-all hover:underline'
          >
            {group.displayName || group.modelPattern}
          </Link>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('channels.group.memberCount', {
              count: group.members.length,
            })}
          </p>
        </div>
        <div className='flex items-center gap-1'>
          <Badge variant={group.enabled ? 'success' : 'secondary'}>
            {t(
              group.enabled
                ? `channels.group.${group.mode}`
                : 'channels.group.inactive'
            )}
          </Badge>
          <Button
            size='icon-sm'
            variant='ghost'
            aria-label={t('channels.group.edit', { name: group.name })}
            aria-expanded={expanded}
            onClick={onToggle}
          >
            <Pencil className='size-4' />
          </Button>
          <Button
            size='icon-sm'
            variant='ghost'
            disabled={isPending}
            aria-label={t('channels.group.delete', { name: group.name })}
            onClick={onDelete}
          >
            <Trash2 className='size-4' />
          </Button>
        </div>
      </div>
      <UpstreamRouteTopology
        group={group}
        inventory={inventory}
        channelId={channelId}
        now={now}
      />
      <div hidden={!expanded}>
        <UpstreamGroupForm
          group={group}
          grants={grants}
          onDirtyChange={onDirtyChange}
        />
      </div>
    </article>
  )
}
