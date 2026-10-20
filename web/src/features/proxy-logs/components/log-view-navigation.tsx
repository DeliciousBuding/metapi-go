import { Link, useLocation } from '@tanstack/react-router'
import { GitBranch, Info, ListChecks } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
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
    <div className='flex flex-wrap items-center gap-1 pt-1'>
      <nav
        aria-label={t('requestLogs.viewsLabel')}
        className='bg-muted inline-flex shrink-0 items-center rounded-lg p-0.5'
      >
        {(['requests', 'attempts'] as const).map((target) => {
          const Icon = target === 'requests' ? ListChecks : GitBranch
          return (
            <Link
              key={target}
              to='/proxy-logs'
              search={{ ...shared, view: target }}
              className={cn(
                buttonVariants({ size: 'sm', variant: 'ghost' }),
                view === target
                  ? 'bg-background shadow-sm'
                  : 'text-muted-foreground'
              )}
              aria-current={view === target ? 'page' : undefined}
            >
              <Icon className='size-3.5' aria-hidden='true' />
              {t(
                target === 'requests'
                  ? 'requestLogs.requestView'
                  : 'requestLogs.history'
              )}
            </Link>
          )
        })}
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
          {t(
            view === 'attempts'
              ? 'requestLogs.historyHint'
              : 'requestLogs.description'
          )}
        </PopoverContent>
      </Popover>
    </div>
  )
}
