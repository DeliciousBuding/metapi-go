import { Link } from '@tanstack/react-router'
import { ArrowDown, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { toBcp47 } from '@/i18n/languages'
import type { ImportedUpstreamInventory } from '@/lib/api/imported-upstreams'
import type { UpstreamGroup } from '@/lib/api/upstream-catalog'
import { formatDateTime } from '@/lib/format'

import { memberProtocols } from '../lib/upstream-config'
import { memberAvailability } from '../lib/upstream-view'

export function UpstreamRouteTopology(props: {
  group: UpstreamGroup
  inventory?: ImportedUpstreamInventory
  channelId: number
  now: number
}) {
  const { t, i18n } = useTranslation()
  const group = props.group
  const members = [...group.members].sort(
    (a, b) => a.priority - b.priority || a.id - b.id
  )
  return (
    <section
      aria-label={t('channels.visual.topology', { name: group.name })}
      className='space-y-3 border-t p-4'
    >
      <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3'>
        <div className='min-w-0 space-y-1'>
          <p className='text-muted-foreground text-xs'>
            {t('channels.visual.publicModel')}
          </p>
          <Link
            to='/token-routes'
            search={{ routeId: group.routeId }}
            className='focus-visible:outline-ring block max-w-full hover:underline'
          >
            <ModelPill model={group.modelPattern} />
          </Link>
          {group.displayName && group.displayName !== group.modelPattern && (
            <p className='text-muted-foreground text-xs break-words'>
              {group.displayName}
            </p>
          )}
        </div>
        <Badge variant={group.enabled ? 'outline' : 'secondary'}>
          {t(
            group.enabled
              ? `channels.group.${group.mode}`
              : 'channels.group.inactive'
          )}
        </Badge>
      </div>
      <p className='text-muted-foreground flex items-center gap-2 text-xs'>
        <ArrowDown aria-hidden='true' className='size-3.5' />
        {t(
          group.mode === 'manual'
            ? 'channels.visual.manualHint'
            : 'channels.visual.candidatesHint'
        )}
      </p>
      <ol aria-label={t('channels.visual.candidates')} className='space-y-2'>
        {members.map((member) => {
          const observed = props.inventory?.members.find(
            (row) => row.id === member.id
          )
          const channel = props.inventory?.items.find(
            (row) => row.id === member.channelId
          )
          let state = memberAvailability(observed, props.now)
          const selected =
            group.mode === 'manual' && group.activeMemberId === member.id
          if (!group.enabled || (group.mode === 'manual' && !selected)) {
            state = 'inactive'
          }
          let tone: 'success' | 'warning' | 'secondary' = 'secondary'
          if (state === 'configured') tone = 'success'
          else if (state === 'cooling') tone = 'warning'
          return (
            <li
              key={member.id}
              className='min-w-0 space-y-2 rounded-lg border p-3'
            >
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <span className='text-muted-foreground text-xs tabular-nums'>
                  {t('channels.visual.priority', { value: member.priority })} ·{' '}
                  {t('channels.visual.weight', { value: member.weight })}
                </span>
                <div className='flex flex-wrap gap-1'>
                  {selected && (
                    <Badge variant='outline'>
                      {t('channels.visual.manuallySelected')}
                    </Badge>
                  )}
                  <Badge variant={tone}>
                    {t(`channels.visual.memberState.${state}`)}
                  </Badge>
                </div>
              </div>
              <p className='text-sm font-medium break-words'>
                {channel?.name ??
                  t('channels.visual.platformId', { id: member.channelId })}
                {member.channelId === props.channelId && (
                  <span className='text-muted-foreground ml-2 text-xs'>
                    {t('channels.visual.thisPlatform')}
                  </span>
                )}
              </p>
              <ModelPill model={member.modelName} />
              <p className='text-muted-foreground flex items-center gap-1.5 text-xs break-words'>
                <KeyRound aria-hidden='true' className='size-3.5 shrink-0' />
                {member.credentialName}
              </p>
              {observed && (
                <p className='text-muted-foreground text-xs break-words'>
                  {memberProtocols(observed)}
                </p>
              )}
              {state === 'cooling' && observed?.cooldownUntil && (
                <p className='text-warning text-xs'>
                  {t('channels.visual.coolingUntil', {
                    time: formatDateTime(
                      observed.cooldownUntil,
                      toBcp47(i18n.language)
                    ),
                  })}
                </p>
              )}
            </li>
          )
        })}
      </ol>
      <p className='text-muted-foreground text-xs'>
        {t('channels.visual.configurationOnly')}
      </p>
    </section>
  )
}
