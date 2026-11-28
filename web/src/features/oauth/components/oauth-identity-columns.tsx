import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { DataTableColumnHeader } from '@/components/data-table'

import type { OAuthClient } from '../types'

function TruncatedText({
  text,
  maxLength = 160,
}: {
  text: string
  maxLength?: number
}) {
  const display =
    text.length > maxLength ? `${text.slice(0, maxLength)}…` : text
  return (
    <span className='block max-w-38 truncate text-sm' title={text}>
      {display}
    </span>
  )
}

export function oauthIdentityColumns(t: TFunction): ColumnDef<OAuthClient>[] {
  return [
    {
      id: 'provider',
      accessorKey: 'provider',
      size: 100,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.provider')}
        />
      ),
      cell: ({ row }) => (
        <span
          className='block max-w-22 truncate font-medium'
          title={row.original.provider}
        >
          {row.original.provider}
        </span>
      ),
    },
    {
      id: 'username',
      accessorFn: (row) => row.username ?? row.email ?? '',
      size: 170,
      meta: { mobileOrder: 1 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.username')}
        />
      ),
      cell: ({ row }) => {
        const client = row.original
        const label = client.username ?? client.email ?? client.accountKey
        return label ? (
          <TruncatedText text={label} />
        ) : (
          <span className='text-muted-foreground text-sm'>—</span>
        )
      },
    },
    {
      id: 'site',
      accessorFn: (row) => row.site?.name ?? '',
      size: 100,
      meta: { mobileHidden: true, mobileOrder: 10 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.site')}
        />
      ),
      cell: ({ row }) => {
        const siteName = row.original.site?.name
        return siteName ? (
          <span
            className='text-muted-foreground block max-w-24 truncate text-sm'
            title={siteName}
          >
            {siteName}
          </span>
        ) : (
          <span className='text-muted-foreground text-sm'>—</span>
        )
      },
    },
  ]
}
