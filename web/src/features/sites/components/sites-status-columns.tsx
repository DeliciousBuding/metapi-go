import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import {
  CheckCircle2 as CheckCircle2Icon,
  PauseCircle as PauseCircleIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PlatformBadge } from '@/components/common/platform-badge'
import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'

import type { Site, SiteStatus } from '../types'

const STATUS_LABEL_KEY: Record<SiteStatus, string> = {
  active: 'sites.status.active',
  disabled: 'sites.status.disabled',
}

function StatusBadge({ status }: { status: SiteStatus | undefined }) {
  const { t } = useTranslation()
  const resolved: SiteStatus = status === 'disabled' ? 'disabled' : 'active'
  const Icon = resolved === 'active' ? CheckCircle2Icon : PauseCircleIcon
  return (
    <Badge variant={resolved === 'active' ? 'success' : 'secondary'}>
      <Icon className='size-3' aria-hidden='true' />
      {t(STATUS_LABEL_KEY[resolved])}
    </Badge>
  )
}

export function statusColumns(t: TFunction): ColumnDef<Site>[] {
  return [
    {
      id: 'status',
      accessorKey: 'status',
      size: 120,
      meta: {
        label: t('sites.columns.status'),
        mobileBadge: true,
        mobileOrder: 1,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.status')}
        />
      ),
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as SiteStatus | undefined
        const resolved: SiteStatus =
          value === 'disabled' ? 'disabled' : 'active'
        if (Array.isArray(filterValue)) {
          return filterValue.length === 0 || filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'platform',
      accessorKey: 'platform',
      size: 140,
      meta: {
        label: t('sites.columns.platform'),
        mobileOrder: 20,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.platform')}
        />
      ),
      cell: ({ row }) => {
        const platform = row.original.platform
        return <PlatformBadge platform={platform} />
      },
    },
  ]
}
