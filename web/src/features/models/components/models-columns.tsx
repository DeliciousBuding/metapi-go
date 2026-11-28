// metapi-go/features/models — TanStack Table column definitions.
//
// `useModelsColumns` is a hook (calls `useTranslation` during render) that
// returns the `ColumnDef<ModelRow>[]` for the list page. Column `meta`
// flags drive the mobile-card layout (`mobileTitle` / `mobileBadge` /
// `mobileOrder` / `mobileHidden`) so the same definition serves the desktop
// table and the phone card list without a parallel layout file.
//
// The brand column is derived from the model name via `getBrand` — a
// client-side visual identity with no server-side owner, so it renders as a
// display column only (no sort, no filter); the q search covers the intent.
// The capability / endpoint-type / supply-source faceted filters are server
// dimensions: their options and counts arrive with the page response
// (facets), and the columns only render the values. Sorting is server-side
// too, so only whitelisted columns keep clickable sort headers.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye as EyeIcon,
  FlaskConical as FlaskConicalIcon,
  MoreHorizontal as MoreHorizontalIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { getBrand } from '@/assets/brand-icons/BrandIcon'
import { ModelPill } from '@/components/common/model-pill'
import { DataTableColumnHeader, TruncatedCell } from '@/components/data-table'
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
import { modelPricingColumn } from './model-pricing-column'
import { modelSupplyColumns } from './model-supply-columns'

export { pricingSiteLabel } from './model-pricing-column'

export type ModelsColumnActions = {
  onView: (model: ModelRow) => void
  onTest: (model: ModelRow) => void
}

function resolveBrandName(modelName: string): string {
  return getBrand(modelName)?.name ?? ''
}

/**
 * Build the model list columns. Must be called during render (it is a hook
 * because it reads i18n state). The `actions` callbacks are supplied by the
 * page so the columns stay free of mutation/query concerns.
 */
export function useModelsColumns(
  actions: ModelsColumnActions
): ColumnDef<ModelRow>[] {
  const { t } = useTranslation()

  const columns: ColumnDef<ModelRow>[] = useMemo(
    () => [
      {
        id: 'brand',
        accessorFn: (row) => resolveBrandName(row.name),
        // Display-only: the name-to-brand mapping lives in BrandIcon, and the
        // server-side sort whitelist deliberately has no brand column.
        enableSorting: false,
        size: 160,
        meta: { mobileOrder: 0, mobileHidden: true },
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
              <span className='text-muted-foreground text-sm'>
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
            <div className='flex min-w-0 flex-col gap-0.5'>
              <div className='flex min-w-0 items-center gap-2'>
                <Button
                  variant='link'
                  aria-label={`${t('models.actions.viewDetails')}: ${model.name}`}
                  className='text-foreground h-auto max-w-full min-w-0 justify-start p-0 text-left hover:no-underline max-[640px]:min-h-10'
                  onClick={() => actions.onView(model)}
                >
                  <ModelPill
                    model={model.name}
                    variant='inline'
                    className='text-sm'
                  />
                </Button>
                {model.deprecated ? (
                  <Badge
                    variant='outline'
                    className='text-muted-foreground gap-1'
                  >
                    {t('models.columns.deprecated')}
                  </Badge>
                ) : null}
              </div>
              {model.description ? (
                <TruncatedCell className='text-muted-foreground max-w-80 text-xs leading-5 font-normal'>
                  {model.description}
                </TruncatedCell>
              ) : null}
            </div>
          )
        },
      },
      ...modelSupplyColumns(t),
      modelPricingColumn(t),
      {
        id: 'actions',
        size: 64,
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
                    <FlaskConicalIcon className='text-muted-foreground/70 size-3.5' />
                    {t('models.actions.testModel')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )
        },
      },
    ],
    [actions, t]
  )

  return columns
}
