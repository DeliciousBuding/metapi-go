import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { PlatformBadge } from '@/components/common/platform-badge'
import { RowDisclosureTrigger } from '@/components/common/row-disclosure'
import { SafeExternalLink } from '@/components/common/safe-external-link'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'

import { resolveAccountDisplayName } from '../lib/accounts-display-name'
import type { Account, AccountRowActions } from '../types'

export interface AccountsRowDisclosureOptions {
  expandedIds: ReadonlySet<number>
  toggle: (id: number) => void
  getContentId: (id: number) => string
}

export function accountIdentityColumns(
  t: TFunction,
  actions: AccountRowActions,
  disclosure?: AccountsRowDisclosureOptions
): ColumnDef<Account>[] {
  return [
    {
      id: 'select',
      size: 40,
      enableSorting: false,
      enableHiding: false,
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          onCheckedChange={(value) =>
            table.toggleAllPageRowsSelected(Boolean(value))
          }
          aria-label={t('accounts.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
          aria-label={t('accounts.columns.selectRow')}
        />
      ),
      meta: { mobileHidden: true },
    },
    {
      id: 'name',
      accessorFn: (row) => row.username ?? '',
      header: t('accounts.columns.name'),
      cell: ({ row }) => {
        const account = row.original
        const displayName = resolveAccountDisplayName(
          account,
          t('accounts.columns.fallbackApiKey'),
          t('accounts.columns.fallbackUnnamed')
        )
        const isExpanded = disclosure?.expandedIds.has(account.id) ?? false
        return (
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-1.5'>
              {disclosure && (
                <RowDisclosureTrigger
                  expanded={isExpanded}
                  controls={disclosure.getContentId(account.id)}
                  aria-label={displayName}
                  className='text-muted-foreground hover:text-foreground size-6 shrink-0 justify-center p-0'
                  onClick={() => disclosure.toggle(account.id)}
                />
              )}
              <button
                type='button'
                data-slot='account-name'
                aria-label={`${t('accounts.columns.viewDetails')}: ${displayName}`}
                title={displayName}
                onClick={() => actions.onViewDetail(account)}
                className='hover:text-primary focus-visible:outline-focus-ring max-w-55 min-w-0 truncate rounded-sm text-left font-medium hover:underline focus-visible:outline-2 max-[640px]:line-clamp-2 max-[640px]:min-h-10 max-[640px]:max-w-full max-[640px]:break-all max-[640px]:whitespace-normal'
              >
                {displayName}
              </button>
              <Badge variant='secondary'>
                {account.credentialMode === 'session'
                  ? t('accounts.columns.credentialModeSession')
                  : t('accounts.columns.credentialModeApiKey')}
              </Badge>
            </div>
            {account.tags && account.tags.length > 0 && (
              <div className='flex flex-wrap gap-1'>
                {account.tags.slice(0, 3).map((tag) => (
                  <Badge key={tag} variant='outline' className='text-2xs'>
                    {tag}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'site',
      accessorFn: (row) => String(row.siteId),
      header: t('accounts.columns.site'),
      cell: ({ row }) => {
        const account = row.original
        const site = account.site
        if (!site) return <span className='text-muted-foreground'>—</span>
        return (
          <div className='flex flex-col'>
            {/* Quick jump to the upstream site (#1108): the site name is a
                  safe external link when the stored URL is a valid http(s)
                  endpoint, plain text otherwise (same ladder as the sites
                  page columns, #985). */}
            <SafeExternalLink
              url={site.url ?? ''}
              className='max-w-40 text-sm max-[640px]:max-w-full max-[640px]:[&>span]:line-clamp-2 max-[640px]:[&>span]:break-words max-[640px]:[&>span]:whitespace-normal'
              title={
                site.name && site.url
                  ? `${site.name} — ${site.url}`
                  : site.name || site.url || undefined
              }
            >
              {site.name || site.url || `#${site.id}`}
            </SafeExternalLink>
            {site.platform && (
              <PlatformBadge
                platform={site.platform}
                className='text-muted-foreground text-2xs w-fit'
              />
            )}
          </div>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) {
          return true
        }
        return filterValue.includes(String(row.original.siteId))
      },
      meta: { mobileOrder: 1 },
    },
  ]
}
