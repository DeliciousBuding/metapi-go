// metapi-go/features/channels — TanStack Table column definitions for the
// read-only Channels list. Status uses the routing package vocabulary
// (enabled / cooldown / breaker_open / manually_disabled) and renders it with
// color + icon + text (dual-channel, never color-only).

import type { ColumnDef } from '@tanstack/react-table'
import {
  Ban,
  CheckCircle2,
  Clock,
  Eye as EyeIcon,
  TriangleAlert,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ChannelRow, ChannelStatus } from '../types'

export type ChannelsColumnActions = { onView: (channel: ChannelRow) => void }

const STATUS_CONFIG: Record<
  ChannelStatus,
  {
    labelKey: string
    variant: 'success' | 'warning' | 'destructive' | 'secondary'
    dotClass: string
    Icon: typeof CheckCircle2
  }
> = {
  enabled: {
    labelKey: 'channels.status.enabled',
    variant: 'success',
    dotClass: 'bg-success',
    Icon: CheckCircle2,
  },
  cooldown: {
    labelKey: 'channels.status.cooldown',
    variant: 'warning',
    dotClass: 'bg-warning',
    Icon: Clock,
  },
  breaker_open: {
    labelKey: 'channels.status.breakerOpen',
    variant: 'destructive',
    dotClass: 'bg-destructive',
    Icon: TriangleAlert,
  },
  manually_disabled: {
    labelKey: 'channels.status.manuallyDisabled',
    variant: 'secondary',
    dotClass: 'bg-muted-foreground',
    Icon: Ban,
  },
}

export function useChannelsColumns(
  actions?: ChannelsColumnActions
): ColumnDef<ChannelRow>[] {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  return useMemo<ColumnDef<ChannelRow>[]>(
    () => [
      {
        id: 'name',
        accessorKey: 'name',
        size: 200,
        meta: { mobileTitle: true, mobileOrder: 0 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.name')}
          />
        ),
        cell: ({ row }) => (
          <span className='font-medium'>{row.original.name}</span>
        ),
      },
      {
        id: 'site',
        accessorFn: (row) => row.site.name,
        size: 160,
        meta: { mobileOrder: 1 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.site')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-sm'>
            {row.original.site.name || '—'}
          </span>
        ),
      },
      {
        id: 'type',
        accessorKey: 'type',
        size: 110,
        meta: { mobileHidden: true, mobileOrder: 30 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.type')}
          />
        ),
        cell: ({ row }) => {
          const type = row.original.type
          return (
            <Badge variant='outline' className='capitalize'>
              {t(`channels.type.${type}`)}
            </Badge>
          )
        },
      },
      {
        id: 'status',
        accessorKey: 'status',
        size: 170,
        meta: { mobileBadge: true, mobileOrder: 2 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.status')}
          />
        ),
        cell: ({ row }) => {
          const status = row.original.status
          const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.enabled
          const Icon = config.Icon
          return (
            <Badge variant={config.variant} className='gap-1.5'>
              <span
                aria-hidden='true'
                className={cn('size-1.5 rounded-full', config.dotClass)}
              />
              <Icon aria-hidden='true' />
              {t(config.labelKey)}
            </Badge>
          )
        },
      },
      {
        id: 'models',
        accessorKey: 'models',
        size: 180,
        meta: { mobileHidden: true, mobileOrder: 31 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.models')}
          />
        ),
        cell: ({ row }) => (
          <span
            className='text-muted-foreground block max-w-[180px] truncate text-sm'
            title={row.original.models || undefined}
          >
            {row.original.models || '—'}
          </span>
        ),
      },
      {
        id: 'priority',
        accessorKey: 'priority',
        size: 90,
        meta: { mobileHidden: true, mobileOrder: 32 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.priority')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-sm tabular-nums'>{row.original.priority}</span>
        ),
      },
      {
        id: 'weight',
        accessorKey: 'weight',
        size: 90,
        meta: { mobileHidden: true, mobileOrder: 33 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.weight')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-sm tabular-nums'>{row.original.weight}</span>
        ),
      },
      {
        id: 'response',
        accessorFn: (row) => row.responseMs,
        size: 110,
        meta: { mobileHidden: true, mobileOrder: 34 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.response')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-sm tabular-nums'>
            {formatLatency(row.original.responseMs, { autoSeconds: true })}
          </span>
        ),
      },
      {
        id: 'cooldown',
        accessorFn: (row) => row.cooldownUntil,
        size: 180,
        meta: { mobileHidden: true, mobileOrder: 35 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('channels.columns.cooldown')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-sm tabular-nums'>
            {formatDateTime(row.original.cooldownUntil, locale)}
          </span>
        ),
      },
      {
        id: 'actions',
        size: 56,
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        meta: { mobileHidden: false, mobileOrder: 4 },
        header: () => (
          <span className='text-muted-foreground text-xs'>
            {t('common.actions')}
          </span>
        ),
        cell: ({ row }) => {
          if (!actions) return null
          const channel = row.original
          return (
            <div className={cn('flex justify-end')}>
              <Button
                variant='ghost'
                size='icon-sm'
                className='data-popup-open:bg-accent'
                aria-label={t('channels.columns.viewDetails')}
                onClick={() => actions.onView(channel)}
              >
                <EyeIcon className='size-4' />
              </Button>
            </div>
          )
        },
      },
    ],
    [t, locale, actions]
  )
}
