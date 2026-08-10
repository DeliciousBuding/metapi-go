// metapi-go/features/models — TanStack Table column definitions.
//
// `useModelsColumns` is a hook (calls `useTranslation` during render) that
// returns the `ColumnDef<ModelRow>[]` for the list page. Column `meta`
// flags drive the mobile-card layout (`mobileTitle` / `mobileBadge` /
// `mobileOrder` / `mobileHidden`) so the same definition serves the desktop
// table and the phone card list without a parallel layout file.
//
// The brand column is derived from the model name via `getBrand` (no separate
// backend field) and doubles as a faceted filter. The capabilities column
// surfaces `supportedEndpointTypes` as badges and is the second faceted
// filter. Pricing is collapsed to the cheapest per-million input seen
// across accounts so a glance at the list shows the floor price.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  getBrand,
  InlineBrandIcon,
} from '@/assets/brand-icons/BrandIcon'
import {
  BadgeListCell,
  DataTableColumnHeader,
  TruncatedCell,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { ModelRow } from '../types'

export type ModelsColumnActions = {
  onView: (model: ModelRow) => void
  onTest: (model: ModelRow) => void
}

function resolveBrandName(modelName: string): string {
  return getBrand(modelName)?.name ?? ''
}

function resolveLowestInputPrice(model: ModelRow): number | null {
  let lowest: number | null = null
  for (const source of model.pricingSources ?? []) {
    for (const groupKey of Object.keys(source.groupPricing ?? {})) {
      const group = source.groupPricing[groupKey]
      const input = group?.inputPerMillion
      if (typeof input === 'number' && Number.isFinite(input)) {
        if (lowest === null || input < lowest) lowest = input
      }
    }
  }
  return lowest
}

function formatPrice(price: number | null): string {
  if (price === null) return '—'
  if (price === 0) return '0'
  if (price < 0.01) return price.toFixed(4)
  return price.toFixed(2)
}

function formatLatency(latency: number | null | undefined): string {
  if (latency === null || latency === undefined) return '—'
  return `${Math.round(latency)}ms`
}

function formatSuccessRate(rate: number | null | undefined): string {
  if (rate === null || rate === undefined) return '—'
  return `${Math.round(rate * 100)}%`
}

/**
 * Build the model list columns. Must be called during render (it is a hook
 * because it reads i18n state). The `actions` callbacks are supplied by the
 * page so the columns stay free of mutation/query concerns.
 */
export function useModelsColumns(actions: ModelsColumnActions): ColumnDef<ModelRow>[] {
  const { t } = useTranslation()

  const columns: ColumnDef<ModelRow>[] = useMemo(
    () => [
      {
        id: 'brand',
        accessorFn: (row) => resolveBrandName(row.name),
        size: 160,
        meta: { mobileOrder: 0, mobileBadge: true },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.brand')}
          />
        ),
        cell: ({ row }) => {
          const modelName = row.original.name
          const brandName = resolveBrandName(modelName)
          return (
            <div className='flex items-center gap-2'>
              <InlineBrandIcon model={modelName} size={18} />
              <span className='text-sm font-medium'>
                {brandName || t('models.columns.unknownBrand')}
              </span>
            </div>
          )
        },
        filterFn: (row, columnId, filterValue) => {
          const value = String(row.getValue(columnId) ?? '')
          if (Array.isArray(filterValue)) {
            return filterValue.length === 0 || filterValue.includes(value)
          }
          return String(filterValue) === value
        },
      },
      {
        id: 'name',
        accessorKey: 'name',
        size: 240,
        meta: { mobileTitle: true, mobileOrder: 1 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.name')}
          />
        ),
        cell: ({ row }) => {
          const model = row.original
          return (
            <div className='flex flex-col'>
              <span className='font-medium'>{model.name}</span>
              {model.description ? (
                <TruncatedCell
                  className='text-muted-foreground max-w-[20rem] text-xs'
                >
                  {model.description}
                </TruncatedCell>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'capabilities',
        accessorFn: (row) => row.supportedEndpointTypes,
        size: 240,
        meta: { mobileHidden: true, mobileOrder: 20 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.capabilities')}
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
                <Badge key={type} variant='outline' className='gap-1'>
                  {type}
                </Badge>
              ))}
              max={3}
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
        id: 'accountCount',
        accessorKey: 'accountCount',
        size: 110,
        meta: { mobileOrder: 10 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.accountCount')}
          />
        ),
        cell: ({ row }) => (
          <span className='tabular-nums text-sm'>
            {row.original.accountCount}
          </span>
        ),
      },
      {
        id: 'avgLatency',
        accessorKey: 'avgLatency',
        size: 110,
        meta: { mobileHidden: true, mobileOrder: 30 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.latency')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground tabular-nums text-sm'>
            {formatLatency(row.original.avgLatency)}
          </span>
        ),
      },
      {
        id: 'successRate',
        accessorKey: 'successRate',
        size: 110,
        meta: { mobileHidden: true, mobileOrder: 40 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.successRate')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground tabular-nums text-sm'>
            {formatSuccessRate(row.original.successRate)}
          </span>
        ),
      },
      {
        id: 'priceInput',
        accessorFn: (row) => resolveLowestInputPrice(row),
        size: 120,
        meta: { mobileOrder: 50 },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('models.columns.priceInput')}
          />
        ),
        cell: ({ row }) => {
          const price = resolveLowestInputPrice(row.original)
          return (
            <span className='tabular-nums text-sm'>
              {price === null ? (
                <span className='text-muted-foreground'>—</span>
              ) : (
                `$${formatPrice(price)}/M`
              )}
            </span>
          )
        },
      },
      {
        id: 'actions',
        size: 64,
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        meta: { mobileHidden: false, mobileOrder: 5 },
        header: () => (
          <span className='text-muted-foreground text-xs'>
            {t('models.columns.actions')}
          </span>
        ),
        cell: ({ row }) => {
          const model = row.original
          return (
            <div className={cn('flex justify-end')}>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant='ghost'
                      size='icon-sm'
                      className='data-popup-open:bg-accent'
                      aria-label={t('models.columns.rowActions')}
                    />
                  }
                >
                  <MoreHorizontalIcon className='size-4' />
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end' className='w-44'>
                  <DropdownMenuItem onClick={() => actions.onView(model)}>
                    <EyeIcon className='text-muted-foreground/70 size-3.5' />
                    {t('models.actions.viewDetails')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => actions.onTest(model)}>
                    <EyeIcon className='text-muted-foreground/70 size-3.5' />
                    {t('models.actions.testModel')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )
        },
      },
    ],
    [actions, t],
  )

  return columns
}

/**
 * Build the brand faceted-filter options from the live model list. Returns
 * `[{ label, value }]` pairs for each distinct brand name resolved via
 * `getBrand`. Kept as a plain function (not a hook) so the page can call it
 * inside `useMemo` and feed the toolbar directly.
 */
export function buildBrandFilterOptions(models: ModelRow[]): Array<{
  label: string
  value: string
}> {
  const set = new Set<string>()
  for (const model of models) {
    const brandName = resolveBrandName(model.name)
    if (brandName) set.add(brandName)
  }
  return [...set].sort().map((name) => ({ label: name, value: name }))
}

/**
 * Build the capability faceted-filter options from the live model list.
 */
export function buildCapabilityFilterOptions(models: ModelRow[]): Array<{
  label: string
  value: string
}> {
  const set = new Set<string>()
  for (const model of models) {
    for (const endpointType of model.supportedEndpointTypes) {
      set.add(endpointType)
    }
    for (const tag of model.tags) {
      set.add(tag)
    }
  }
  return [...set].sort().map((name) => ({ label: name, value: name }))
}
