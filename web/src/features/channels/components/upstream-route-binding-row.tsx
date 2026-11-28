import { Link } from '@tanstack/react-router'
import { Pencil, RotateCcw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toBcp47 } from '@/i18n/languages'
import type { ImportedMember } from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'

import { memberProtocols } from '../lib/upstream-config'
import { MemberForm } from './upstream-member-form'

export function UpstreamRouteBindingRow({
  member,
  expanded,
  deleting,
  clearing,
  onToggle,
  onDelete,
  onClearCooldown,
  onDirtyChange,
}: {
  member: ImportedMember
  expanded: boolean
  deleting: boolean
  clearing: boolean
  onToggle: () => void
  onDelete: () => void
  onClearCooldown: () => void
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const cooling =
    !!member.cooldownUntil && Date.parse(member.cooldownUntil) > Date.now()
  let status = 'channels.upstream.routable'
  let tone: 'success' | 'warning' | 'secondary' = 'success'
  if (!member.effectiveEnabled) {
    status = 'channels.upstream.inactive'
    tone = 'secondary'
  } else if (cooling) {
    status = 'channels.imported.coolingDown'
    tone = 'warning'
  }
  return (
    <article className='overflow-hidden rounded-xl border'>
      <div className='space-y-3 p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 flex-1 space-y-2'>
            <Link
              to='/token-routes'
              search={{ routeId: member.routeId }}
              className='hover:text-primary text-sm font-semibold hover:underline'
            >
              {member.groupName}
            </Link>
            <div>
              <ModelPill model={member.modelName} />
            </div>
          </div>
          <div className='flex items-center gap-1'>
            <Badge variant={tone}>{t(status)}</Badge>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('channels.upstream.editRoute', {
                name: member.groupName,
              })}
              aria-expanded={expanded}
              onClick={onToggle}
            >
              <Pencil className='size-4' />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              disabled={deleting}
              aria-label={t('channels.group.deleteMember', {
                name: member.modelName,
              })}
              onClick={onDelete}
            >
              <Trash2 className='size-4' />
            </Button>
          </div>
        </div>
        <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
          <span>{member.credentialName}</span>
          <span>{memberProtocols(member)}</span>
          <span>
            {t('channels.upstream.attempts', {
              success: member.successCount ?? 0,
              fail: member.failCount ?? 0,
            })}
          </span>
        </div>
        {!member.effectiveEnabled && (
          <div className='text-muted-foreground flex flex-wrap gap-2 text-xs'>
            {(
              [
                'channelEnabled',
                'credentialEnabled',
                'modelEnabled',
                'routeEnabled',
                'groupEnabled',
                'grantEnabled',
                'selectedByGroup',
              ] as const
            )
              .filter((key) => member[key] === false)
              .map((key) => (
                <span key={key}>{t(`channels.upstream.reasons.${key}`)}</span>
              ))}
          </div>
        )}
        {cooling && (
          <div className='bg-muted/50 flex flex-wrap items-center justify-between gap-3 rounded-lg px-3 py-2'>
            <div className='space-y-1 text-xs'>
              <p>
                {t('channels.upstream.cooldownUntil', {
                  value: formatDateTime(
                    member.cooldownUntil,
                    toBcp47(i18n.language)
                  ),
                })}
              </p>
              {member.cooldownReasonCode && (
                <p className='text-muted-foreground'>
                  {member.cooldownReasonCode}
                </p>
              )}
              <p className='text-muted-foreground'>
                {t('channels.upstream.sharedCooldown')}
              </p>
            </div>
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={clearing}
              onClick={onClearCooldown}
            >
              <RotateCcw className='size-3.5' />
              {t('channels.upstream.clearCooldown')}
            </Button>
          </div>
        )}
      </div>
      <div hidden={!expanded}>
        <MemberForm member={member} onDirtyChange={onDirtyChange} />
      </div>
    </article>
  )
}
