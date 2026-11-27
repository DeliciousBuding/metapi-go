import { Link, useLocation } from '@tanstack/react-router'
import { Activity, History, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

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
    <div className='flex flex-wrap items-center gap-2 pt-1'>
      <nav
        aria-label={t('requestLogs.viewsLabel')}
        className='bg-muted inline-flex items-center gap-0.5 rounded-lg border p-0.5'
      >
        <Link
          to='/proxy-logs'
          search={{ ...shared, view: 'requests' }}
          aria-current={view === 'requests' ? 'page' : undefined}
          className={cn(
            'focus-visible:ring-ring inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs transition-colors outline-none focus-visible:ring-2',
            view === 'requests'
              ? 'bg-background text-foreground font-medium shadow-xs'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          <Activity className='size-3.5' aria-hidden='true' />
          {t('requestLogs.requestView')}
        </Link>
        <Link
          to='/proxy-logs'
          search={{ ...shared, view: 'attempts' }}
          aria-current={view === 'attempts' ? 'page' : undefined}
          className={cn(
            'focus-visible:ring-ring inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs transition-colors outline-none focus-visible:ring-2',
            view === 'attempts'
              ? 'bg-background text-foreground font-medium shadow-xs'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          <History className='size-3.5' aria-hidden='true' />
          {t('requestLogs.history')}
        </Link>
      </nav>
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
        </PopoverContent>
      </Popover>
    </div>
  )
}
