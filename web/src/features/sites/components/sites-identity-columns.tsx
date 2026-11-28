import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import { Pin as PinIcon } from 'lucide-react'

import { SafeExternalLink } from '@/components/common/safe-external-link'
import { DataTableColumnHeader } from '@/components/data-table'
import { Checkbox } from '@/components/ui/checkbox'

import type { Site } from '../types'

function TruncatedUrl({ url }: { url: string }) {
  return (
    <SafeExternalLink url={url} className='max-w-72 text-sm' title={url}>
      {url}
    </SafeExternalLink>
  )
}

export function identityColumns(t: TFunction): ColumnDef<Site>[] {
  return [
    {
      id: 'select',
      enableSorting: false,
      enableHiding: false,
      enableResizing: false,
      size: 36,
      meta: { mobileHidden: true },
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllRowsSelected()}
          indeterminate={
            table.getIsSomeRowsSelected() && !table.getIsAllRowsSelected()
          }
          onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
          aria-label={t('sites.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          onClick={(event) => event.stopPropagation()}
          aria-label={t('sites.columns.selectRow')}
        />
      ),
    },
    {
      id: 'name',
      accessorKey: 'name',
      size: 200,
      meta: {
        label: t('sites.columns.name'),
        mobileTitle: true,
        mobileOrder: 0,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.name')}
        />
      ),
      cell: ({ row }) => {
        const site = row.original
        return (
          <div className='flex min-w-0 items-center gap-2'>
            {site.isPinned && (
              <PinIcon className='text-muted-foreground size-3.5 shrink-0' />
            )}
            <SafeExternalLink
              url={site.url}
              className='max-w-55 font-medium max-[640px]:max-w-full max-[640px]:[&>span]:line-clamp-2 max-[640px]:[&>span]:break-words max-[640px]:[&>span]:whitespace-normal'
              title={site.name}
            >
              {site.name}
            </SafeExternalLink>
          </div>
        )
      },
    },
    {
      id: 'url',
      accessorKey: 'url',
      size: 280,
      meta: {
        label: t('sites.columns.url'),
        mobileFullWidth: true,
        mobileOrder: 10,
      },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('sites.columns.url')} />
      ),
      cell: ({ row }) => <TruncatedUrl url={row.original.url} />,
    },
  ]
}
