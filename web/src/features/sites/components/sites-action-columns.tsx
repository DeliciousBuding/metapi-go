import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import {
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
  Pencil as PencilIcon,
  Pin as PinIcon,
  PinOff as PinOffIcon,
  Power as PowerIcon,
  Trash2 as Trash2Icon,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

import type { Site } from '../types'

export type SitesColumnActions = {
  onEdit: (site: Site) => void
  onView: (site: Site) => void
  onToggleStatus: (site: Site) => void
  onTogglePin: (site: Site) => void
  onDelete: (site: Site) => void
}

export function actionColumns(
  t: TFunction,
  actions: SitesColumnActions,
  pendingSiteId: number | null
): ColumnDef<Site>[] {
  return [
    {
      id: 'actions',
      size: 56,
      enableSorting: false,
      enableHiding: false,
      enableResizing: false,
      meta: {
        mobileHidden: false,
        mobileOrder: 5,
        mobileActionsInHeader: true,
      },
      header: () => (
        <span className='text-muted-foreground text-xs'>
          {t('sites.columns.actions')}
        </span>
      ),
      cell: ({ row }) => {
        const site = row.original
        const isActive = site.status !== 'disabled'
        const isThisRowPending =
          pendingSiteId !== null && pendingSiteId === site.id

        let pinToggleIcon = (
          <PinIcon className='text-muted-foreground/70 size-3.5' />
        )
        if (isThisRowPending) {
          pinToggleIcon = <Spinner className='size-3.5' />
        } else if (site.isPinned) {
          pinToggleIcon = (
            <PinOffIcon className='text-muted-foreground/70 size-3.5' />
          )
        }

        return (
          <div className={cn('flex justify-end')}>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='data-popup-open:bg-accent'
                    aria-label={t('sites.columns.rowActions')}
                  />
                }
              >
                <MoreHorizontalIcon className='size-4' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-44'>
                <DropdownMenuItem onClick={() => actions.onView(site)}>
                  <EyeIcon className='text-muted-foreground/70 size-3.5' />
                  {t('sites.actions.viewDetails')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => actions.onEdit(site)}>
                  <PencilIcon className='text-muted-foreground/70 size-3.5' />
                  {t('sites.actions.edit')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={isThisRowPending}
                  onClick={() => actions.onToggleStatus(site)}
                >
                  {isThisRowPending ? (
                    <Spinner className='size-3.5' />
                  ) : (
                    <PowerIcon className='text-muted-foreground/70 size-3.5' />
                  )}
                  {isActive
                    ? t('sites.actions.disable')
                    : t('sites.actions.enable')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={isThisRowPending}
                  onClick={() => actions.onTogglePin(site)}
                >
                  {pinToggleIcon}
                  {site.isPinned
                    ? t('sites.actions.unpin')
                    : t('sites.actions.pin')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant='destructive'
                  onClick={() => actions.onDelete(site)}
                >
                  <Trash2Icon className='size-3.5' />
                  {t('sites.actions.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
    },
  ]
}
