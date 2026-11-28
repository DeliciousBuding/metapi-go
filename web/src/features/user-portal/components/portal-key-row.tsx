import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SoftToneBadge } from '@/components/common/soft-tone-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import type { PersonalKey } from '../api'
import { personalKeyStatus, personalKeyStatusTone } from '../key-status'

export function PortalKeyRow({
  item,
  onRotate,
  onRevoke,
}: {
  item: PersonalKey
  onRotate: (item: PersonalKey) => void
  onRevoke: (item: PersonalKey) => void
}) {
  const { t } = useTranslation()
  const status = personalKeyStatus(item)
  return (
    <div className='flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between'>
      <div className='min-w-0 space-y-1.5'>
        <div className='flex flex-wrap items-center gap-2'>
          <p className='font-medium wrap-anywhere'>{item.name}</p>
          <SoftToneBadge tone={personalKeyStatusTone(status)}>
            {t('userPortal.keyStatus.' + status)}
          </SoftToneBadge>
        </div>
        <p className='text-muted-foreground font-mono text-xs break-all'>
          {item.keyMasked}
        </p>
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-xs tabular-nums'>
          <span>
            {t('userPortal.requestCount', { count: item.usedRequests })}
          </span>
          <span aria-hidden='true'>·</span>
          <span>
            {item.expiresAt
              ? new Date(item.expiresAt).toLocaleString()
              : t('userPortal.noExpiry')}
          </span>
        </div>
        {item.supportedModels.length > 0 && (
          <div className='flex flex-wrap gap-1 pt-0.5'>
            {item.supportedModels.map((modelName) => (
              <Badge
                key={modelName}
                variant='secondary'
                className='max-w-full font-mono text-xs break-all'
              >
                {modelName}
              </Badge>
            ))}
          </div>
        )}
      </div>
      <div className='flex shrink-0 flex-wrap gap-2'>
        <Button
          variant='outline'
          size='sm'
          aria-label={t('userPortal.rotation.actionFor', {
            name: item.name,
          })}
          onClick={() => onRotate(item)}
        >
          <RefreshCw aria-hidden='true' />
          {t('userPortal.rotation.action')}
        </Button>
        <Button variant='destructive' size='sm' onClick={() => onRevoke(item)}>
          {t('userPortal.revokeKey')}
        </Button>
      </div>
    </div>
  )
}
