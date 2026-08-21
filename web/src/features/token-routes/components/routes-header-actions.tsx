// metapi-go/features/token-routes/components — responsive page-header action
// cluster for the routes list page.
//
// Mobile contract (≤640px, English locale included): the primary "Add Route"
// CTA stays visible at all widths; the secondary actions (Rebuild, Refresh
// decisions) collapse into a "More" dropdown — the same DropdownMenu pattern
// the per-row actions use — so the cluster can never overflow and get
// clipped by the SidebarInset `overflow-x-hidden` ancestor.
// ≥sm renders every action inline (no visual change from the old layout).

import { MoreHorizontal, Plus, RefreshCw, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'

type RoutesHeaderActionsProps = {
  onRebuild: () => void
  isRebuildPending: boolean
  onRefreshDecisions: () => void
  isRefreshDecisionsPending: boolean
  onAddRoute: () => void
}

export function RoutesHeaderActions(props: RoutesHeaderActionsProps) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center gap-2'>
      {/* ≥sm: all secondary actions inline. */}
      <div className='hidden items-center gap-2 sm:flex'>
        <Button
          variant='outline'
          size='sm'
          onClick={props.onRebuild}
          disabled={props.isRebuildPending}
        >
          {props.isRebuildPending ? <Spinner className='size-3.5' /> : <Zap />}
          {t('tokenRoutes.page.rebuild')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={props.onRefreshDecisions}
          disabled={props.isRefreshDecisionsPending}
        >
          <RefreshCw
            className={
              props.isRefreshDecisionsPending ? 'animate-spin' : undefined
            }
          />
          {t('tokenRoutes.page.refreshDecisions')}
        </Button>
      </div>

      {/* <sm: secondary actions collapse into the More dropdown. */}
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
              onClick={props.onRebuild}
              disabled={props.isRebuildPending}
            >
              {props.isRebuildPending ? <Spinner /> : <Zap />}
              {t('tokenRoutes.page.rebuild')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={props.onRefreshDecisions}
              disabled={props.isRefreshDecisionsPending}
            >
              <RefreshCw
                className={
                  props.isRefreshDecisionsPending ? 'animate-spin' : undefined
                }
              />
              {t('tokenRoutes.page.refreshDecisions')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Primary CTA: always visible, never collapsed. */}
      <Button onClick={props.onAddRoute}>
        <Plus />
        {t('tokenRoutes.page.addButton')}
      </Button>
    </div>
  )
}
