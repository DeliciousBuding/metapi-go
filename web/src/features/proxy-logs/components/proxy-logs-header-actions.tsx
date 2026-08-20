// metapi-go/features/proxy-logs/components — responsive page-header actions
// for the proxy logs list page.
//
// Mobile contract (≤640px): the auto-refresh segmented control stays inline
// (owned by the page), while Export/Refresh collapse into a "More" dropdown —
// the row-action DropdownMenu pattern — so the cluster wraps instead of
// overflowing ~420px and being clipped by the layout's overflow-x-hidden.
// ≥sm renders Export/Refresh inline (no visual change from the old layout).

import {
  Download as DownloadIcon,
  MoreHorizontal,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

type ProxyLogsHeaderActionsProps = {
  onExport: () => void
  isExporting: boolean
  onRefresh: () => void
  isRefreshing: boolean
}

export function ProxyLogsHeaderActions(props: ProxyLogsHeaderActionsProps) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center gap-2'>
      {/* ≥sm: Export/Refresh inline. */}
      <div className='hidden items-center gap-2 sm:flex'>
        <Button
          variant='outline'
          size='sm'
          onClick={props.onExport}
          disabled={props.isExporting}
        >
          <DownloadIcon
            className={props.isExporting ? 'animate-pulse' : undefined}
          />
          {t('proxyLogs.page.exportCsv')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={props.onRefresh}
          disabled={props.isRefreshing}
        >
          <RefreshCw
            className={props.isRefreshing ? 'animate-spin' : undefined}
          />
          {t('proxyLogs.page.refresh')}
        </Button>
      </div>

      {/* <sm: Export/Refresh collapse into the More dropdown. */}
      <div className='sm:hidden'>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                aria-label={t('common.moreActions')}
              />
            }
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' sideOffset={4}>
            <DropdownMenuItem
              onClick={props.onExport}
              disabled={props.isExporting}
            >
              <DownloadIcon
                className={props.isExporting ? 'animate-pulse' : undefined}
              />
              {t('proxyLogs.page.exportCsv')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={props.onRefresh}
              disabled={props.isRefreshing}
            >
              <RefreshCw
                className={props.isRefreshing ? 'animate-spin' : undefined}
              />
              {t('proxyLogs.page.refresh')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}
