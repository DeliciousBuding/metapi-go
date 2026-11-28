import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { BadgeListCell, DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { formatLatency, formatSuccessRate } from '@/lib/format'

import { supplyKindLabels } from '../lib/supply-kind-labels'
import type { ModelRow } from '../types'

export function modelSupplyColumns(t: TFunction): ColumnDef<ModelRow>[] {
  return [
    {
      id: 'endpointTypes',
      accessorFn: (row) => row.supportedEndpointTypes,
      enableSorting: false,
      size: 160,
      meta: { mobileHidden: true, mobileOrder: 20 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.columns.endpointTypes')}
        />
      ),
      cell: ({ row }) => {
        const endpointTypes = row.original.supportedEndpointTypes ?? []
        if (endpointTypes.length === 0) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <BadgeListCell
            items={endpointTypes.map((type) => (
              <span key={type} className='shrink-0 px-1.5 text-xs'>
                {type}
              </span>
            ))}
            max={2}
          />
        )
      },
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId)
        const values: string[] = Array.isArray(value) ? value : []
        if (Array.isArray(filterValue)) {
          if (filterValue.length === 0) return true
          return filterValue.some((facet) => values.includes(String(facet)))
        }
        return values.includes(String(filterValue))
      },
    },
    {
      id: 'capabilities',
      accessorFn: (row) => row.tags,
      enableSorting: false,
      size: 200,
      meta: { mobileHidden: true, mobileOrder: 21 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.columns.capabilities')}
        />
      ),
      cell: ({ row }) => {
        const tags = row.original.tags ?? []
        if (tags.length === 0) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <BadgeListCell
            items={tags.map((tag) => (
              <span
                key={tag}
                className='text-muted-foreground shrink-0 px-1.5 text-xs'
              >
                {tag}
              </span>
            ))}
            max={2}
          />
        )
      },
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId)
        const values: string[] = Array.isArray(value) ? value : []
        if (Array.isArray(filterValue)) {
          if (filterValue.length === 0) return true
          return filterValue.some((facet) => values.includes(String(facet)))
        }
        return values.includes(String(filterValue))
      },
    },
    {
      id: 'sourceKinds',
      accessorFn: (row) => [
        ...new Set((row.supplySources ?? []).map((source) => source.kind)),
      ],
      // Hidden by default; it exists so the toolbar's supply-source faceted
      // filter has a column to bind to. Filter semantics are server-side.
      enableSorting: false,
      size: 140,
      meta: {
        mobileHidden: true,
        mobileOrder: 25,
        label: t('models.filters.sourceKind'),
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.filters.sourceKind')}
        />
      ),
      cell: ({ row }) => {
        const kinds = [
          ...new Set(
            (row.original.supplySources ?? []).map((source) => source.kind)
          ),
        ]
        if (kinds.length === 0) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <BadgeListCell
            items={kinds.map((kind) => (
              <Badge key={kind} variant='outline' className='gap-1'>
                {t(supplyKindLabels[kind])}
              </Badge>
            ))}
            max={3}
          />
        )
      },
    },
    {
      id: 'supplyCount',
      accessorKey: 'supplyCount',
      size: 110,
      meta: { mobileOrder: 9, label: t('models.supply.title') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.supply.title')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.supplyCount ?? '—'}
        </span>
      ),
    },
    {
      id: 'accountCount',
      accessorKey: 'accountCount',
      size: 110,
      meta: { mobileOrder: 10, label: t('models.columns.accountCount') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.columns.accountCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.accountCount}
        </span>
      ),
    },
    {
      id: 'avgLatency',
      accessorKey: 'avgLatency',
      size: 110,
      meta: { mobileOrder: 30, label: t('models.columns.latency') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.columns.latency')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {formatLatency(row.original.avgLatency)}
        </span>
      ),
    },
    {
      id: 'successRate',
      accessorKey: 'successRate',
      size: 110,
      meta: { mobileOrder: 40, label: t('models.columns.successRate') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('models.columns.successRate')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {formatSuccessRate(row.original.successRate)}
        </span>
      ),
    },
  ]
}
