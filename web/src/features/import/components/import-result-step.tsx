import {
  Check as CheckIcon,
  Minus as MinusIcon,
  X as XIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { ImportSiteResultStatus, ImportSitesResult } from '../types'

function statusBadge(status: ImportSiteResultStatus) {
  if (status === 'imported') {
    return {
      variant: 'outline' as const,
      icon: CheckIcon,
      label: 'import.result.imported',
    }
  }
  if (status === 'merged') {
    return {
      variant: 'outline' as const,
      icon: CheckIcon,
      label: 'import.result.merged',
    }
  }
  if (status === 'failed') {
    return {
      variant: 'destructive' as const,
      icon: XIcon,
      label: 'import.result.failed',
    }
  }
  return {
    variant: 'secondary' as const,
    icon: MinusIcon,
    label: 'import.result.skipped',
  }
}

export function ImportResultStep({ result }: { result: ImportSitesResult }) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant='outline'>
          <CheckIcon className='size-3' />
          {t('import.done.imported', { count: result.imported })}
        </Badge>
        <Badge variant='secondary'>
          <MinusIcon className='size-3' />
          {t('import.done.skipped', { count: result.skipped })}
        </Badge>
        <Badge variant='destructive'>
          <XIcon className='size-3' />
          {t('import.done.failed', { count: result.failed })}
        </Badge>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t('import.done.rebuildHint')}
      </p>
      <div className='grid gap-2'>
        {result.results.map((item) => {
          const badge = statusBadge(item.status)
          const Icon = badge.icon
          const showReason =
            item.reason != null &&
            item.reason !== '' &&
            (item.status === 'failed' || item.status === 'skipped')
          return (
            <div
              key={`${item.url}-${item.status}`}
              className='flex items-start justify-between gap-3 rounded-lg border p-2.5'
            >
              <div className='min-w-0'>
                <div className='truncate text-sm'>{item.name}</div>
                <div className='text-muted-foreground truncate text-xs'>
                  {item.url}
                </div>
              </div>
              <div className='flex flex-col items-end gap-1'>
                <Badge variant={badge.variant}>
                  <Icon className='size-3' />
                  {t(badge.label)}
                </Badge>
                {showReason && (
                  <span
                    className={
                      item.status === 'failed'
                        ? 'text-destructive text-xs'
                        : 'text-muted-foreground text-xs'
                    }
                  >
                    {t('import.result.reason', {
                      reason: item.reason,
                    })}
                  </span>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
