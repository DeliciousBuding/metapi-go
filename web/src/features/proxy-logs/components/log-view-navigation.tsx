import { Link, useLocation } from '@tanstack/react-router'
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import { proxyLogsSearchSchema } from '../lib/proxy-logs-schema'

// Only shared filters cross the boundary. Attempt status, latency, site and
// request detail identities have no equivalent in the other view.
export function LogViewNavigation({ view }: { view: 'requests' | 'attempts' }) {
  const { t } = useTranslation()
  const location = useLocation()
  const search = proxyLogsSearchSchema.parse(
    Object.fromEntries(new URLSearchParams(location.searchStr))
  )
  const shared = {
    ...(search.downstreamKeyId
      ? { downstreamKeyId: search.downstreamKeyId }
      : {}),
    ...(search.from ? { from: search.from } : {}),
    ...(search.to ? { to: search.to } : {}),
    ...(search.pageSize !== 20 ? { pageSize: search.pageSize } : {}),
  }
  return (
    <div className='flex flex-wrap items-center gap-1 pt-1'>
      {view === 'attempts' && (
        <Link
          to='/proxy-logs'
          search={{ ...shared, view: 'requests' }}
          className={buttonVariants({ size: 'sm', variant: 'ghost' })}
        >
          {t('requestLogs.backToRequests')}
        </Link>
      )}
      <Popover>
        <PopoverTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('requestLogs.viewHelp')}
            />
          }
        >
          <Info className='text-muted-foreground size-3.5' aria-hidden='true' />
        </PopoverTrigger>
        <PopoverContent
          className='max-w-[calc(100vw-2rem)]'
          aria-label={t('requestLogs.viewHelp')}
        >
          <p>
            {t(
              view === 'attempts'
                ? 'requestLogs.historyHint'
                : 'requestLogs.description'
            )}
          </p>
          {view === 'requests' && (
            <Link
              to='/proxy-logs'
              search={{ ...shared, view: 'attempts' }}
              className='text-primary mt-2 inline-block text-sm hover:underline'
            >
              {t('requestLogs.history')}
            </Link>
          )}
        </PopoverContent>
      </Popover>
    </div>
  )
}
