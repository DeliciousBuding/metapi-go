import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import {
  CheckCircle2,
  Clock,
  HelpCircle,
  PauseCircle,
  TriangleAlert,
  XCircle,
  type LucideIcon,
} from 'lucide-react'

import {
  ProbeHealthBar,
  type ProbeHistoryMap,
} from '@/components/common/probe-health-bar'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { Account, RuntimeHealthState } from '../types'

interface HealthBadgeConfig {
  labelKey: string
  variant: 'success' | 'secondary' | 'destructive' | 'warning' | 'outline'
  icon: LucideIcon
}

const HEALTH_BADGE_CONFIG: Record<RuntimeHealthState, HealthBadgeConfig> = {
  healthy: {
    labelKey: 'accounts.columns.healthHealthy',
    variant: 'success',
    icon: CheckCircle2,
  },
  degraded: {
    labelKey: 'accounts.columns.healthDegraded',
    variant: 'warning',
    icon: TriangleAlert,
  },
  unhealthy: {
    labelKey: 'accounts.columns.healthUnhealthy',
    variant: 'destructive',
    icon: XCircle,
  },
  disabled: {
    labelKey: 'accounts.columns.healthDisabled',
    variant: 'secondary',
    icon: PauseCircle,
  },
  unknown: {
    labelKey: 'accounts.columns.healthUnknown',
    variant: 'outline',
    icon: HelpCircle,
  },
}

function resolveHealth(
  account: Account,
  t: TFunction
): HealthBadgeConfig & { label: string } {
  if (account.status === 'expired') {
    return {
      labelKey: 'accounts.columns.healthExpired',
      label: t('accounts.columns.healthExpired'),
      variant: 'destructive',
      icon: Clock,
    }
  }
  const state = account.runtimeHealth?.state ?? 'unknown'
  const config = HEALTH_BADGE_CONFIG[state] ?? HEALTH_BADGE_CONFIG.unknown
  return { ...config, label: t(config.labelKey) }
}
export function accountHealthColumns(
  t: TFunction,
  probeHistory?: ProbeHistoryMap
): ColumnDef<Account>[] {
  return [
    {
      id: 'status',
      accessorFn: (row) => row.status,
      header: t('common.status'),
      cell: ({ row }) => {
        const account = row.original
        const config = resolveHealth(account, t)
        const HealthIcon = config.icon
        const reason = account.runtimeHealth?.reason?.trim()
        const badge = (
          <Badge variant={config.variant}>
            <HealthIcon className='size-3' aria-hidden='true' />
            {config.label}
          </Badge>
        )
        if (!reason) {
          return badge
        }
        return (
          <TooltipProvider delay={200}>
            <Tooltip>
              <TooltipTrigger render={<span className='w-fit'>{badge}</span>} />
              <TooltipContent side='top' className='max-w-xs'>
                <div className='flex flex-col gap-0.5'>
                  <span className='font-medium'>
                    {t('accounts.columns.healthDetail')}
                  </span>
                  <span className='text-xs'>{reason}</span>
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) {
          return true
        }
        return filterValue.includes(row.original.status)
      },
      meta: { mobileBadge: true },
    },
    {
      id: 'probeHealth',
      accessorFn: (row) => probeHistory?.[row.id]?.length ?? 0,
      enableSorting: false,
      header: () => (
        <span className='text-muted-foreground text-xs'>
          {t('probeHealth.column')}
        </span>
      ),
      cell: ({ row }) => (
        <ProbeHealthBar
          results={probeHistory?.[row.original.id]}
          pending={probeHistory === undefined}
        />
      ),
      meta: { mobileHidden: true },
    },
  ]
}
