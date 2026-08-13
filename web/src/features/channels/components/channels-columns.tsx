// metapi-go/features/channels — TanStack Table column definitions for the
// read-only Channels list. Status uses the routing package vocabulary
// (enabled / cooldown / breaker_open / manually_disabled) and renders it with
// color + icon + text (dual-channel, never color-only).

import type { ColumnDef } from '@tanstack/react-table'
import {
  Ban,
  CheckCircle2,
  Clock,
  TriangleAlert,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { ChannelRow, ChannelStatus } from '../types'

const STATUS_CONFIG: Record<
  ChannelStatus,
  { labelKey: string; variant: 'default' | 'warning' | 'destructive' | 'secondary'; dotClass: string; Icon: typeof CheckCircle2 }
> = {
  enabled: {
    labelKey: 'channels.status.enabled',
    variant: 'default',
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

function formatResponse(ms: number | null): string {
  if (ms === null || ms === undefined) return '—'
  return `${Math.round(ms)}ms`
}

function formatCooldown(until: string | null): string {
  if (!until) return '—'
  return new Date(until).toLocaleString()
}

export function useChannelsColumns(): ColumnDef<ChannelRow>[] {
  const { t } = useTranslation()

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
          <span className='text-muted-foreground text-sm'>
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
        cell: ({ row }) => <span className='text-sm'>{row.original.priority}</span>,
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
        cell: ({ row }) => <span className='text-sm'>{row.original.weight}</span>,
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
          <span className='text-muted-foreground text-sm'>
            {formatResponse(row.original.responseMs)}
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
          <span className='text-muted-foreground text-sm'>
            {formatCooldown(row.original.cooldownUntil)}
          </span>
        ),
      },
    ],
    [t]
  )
}
