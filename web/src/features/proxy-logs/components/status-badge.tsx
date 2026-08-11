// metapi-go/features/proxy-logs/components — HTTP status badge.
// i18n: fallback labels resolved via t().

import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type StatusTier = {
  className: string
  dotClassName: string
  fallbackLabelKey: string
}

const STATUS_TIERS = {
  success: {
    className: 'bg-success/10 text-success border-success/30',
    dotClassName: 'bg-success',
    fallbackLabelKey: 'proxyLogs.status.success',
  },
  redirect: {
    className: 'bg-info/10 text-info border-info/30',
    dotClassName: 'bg-info',
    fallbackLabelKey: 'proxyLogs.status.redirect',
  },
  clientError: {
    className: 'bg-warning/10 text-warning border-warning/30',
    dotClassName: 'bg-warning',
    fallbackLabelKey: 'proxyLogs.status.clientError',
  },
  serverError: {
    className: 'bg-destructive/10 text-destructive border-destructive/30',
    dotClassName: 'bg-destructive',
    fallbackLabelKey: 'proxyLogs.status.serverError',
  },
  neutral: {
    className: 'bg-muted/40 text-muted-foreground border-border',
    dotClassName: 'bg-muted-foreground',
    fallbackLabelKey: 'proxyLogs.status.unknown',
  },
} as const

function resolveTierFromHttpStatus(httpStatus: number): StatusTier {
  if (httpStatus >= 200 && httpStatus < 300) return STATUS_TIERS.success
  if (httpStatus >= 300 && httpStatus < 400) return STATUS_TIERS.redirect
  if (httpStatus >= 400 && httpStatus < 500) return STATUS_TIERS.clientError
  if (httpStatus >= 500) return STATUS_TIERS.serverError
  return STATUS_TIERS.neutral
}

function resolveTierFromStatusString(status: string): StatusTier {
  const normalized = status.toLowerCase()
  if (['success', 'ok', 'succeeded', 'succeed'].includes(normalized))
    return STATUS_TIERS.success
  if (
    ['failed', 'error', 'failure', 'timeout', 'timeouterror'].includes(
      normalized
    )
  )
    return STATUS_TIERS.serverError
  if (normalized.includes('redirect')) return STATUS_TIERS.redirect
  if (normalized.includes('client')) return STATUS_TIERS.clientError
  return STATUS_TIERS.neutral
}

function resolveTier(
  httpStatus: number | null | undefined,
  status: string | null | undefined
): { tier: StatusTier; label: string } {
  const numericStatus =
    typeof httpStatus === 'number' && httpStatus > 0 ? httpStatus : null
  if (numericStatus !== null)
    return {
      tier: resolveTierFromHttpStatus(numericStatus),
      label: String(numericStatus),
    }
  const statusString =
    typeof status === 'string' && status.trim().length > 0
      ? status.trim()
      : null
  if (statusString !== null) {
    const parsed = Number.parseInt(statusString, 10)
    if (Number.isFinite(parsed) && parsed > 0)
      return { tier: resolveTierFromHttpStatus(parsed), label: String(parsed) }
    return {
      tier: resolveTierFromStatusString(statusString),
      label: statusString,
    }
  }
  return {
    tier: STATUS_TIERS.neutral,
    label: STATUS_TIERS.neutral.fallbackLabelKey,
  }
}

export type StatusBadgeProps = {
  httpStatus?: number | null
  status?: string | null
  className?: string
  showDot?: boolean
}

export function StatusBadge({
  httpStatus,
  status,
  className,
  showDot = true,
}: StatusBadgeProps) {
  const { t } = useTranslation()
  const resolved = resolveTier(httpStatus, status)
  const tier = resolved.tier
  const label =
    resolved.tier === STATUS_TIERS.neutral && !httpStatus && !status
      ? t(tier.fallbackLabelKey)
      : resolved.label
  return (
    <span
      title={t('proxyLogs.status.titlePrefix', { label })}
      className={cn(
        'inline-flex w-fit items-center gap-1 rounded-4xl border px-1.5 py-0.5 text-xs font-medium tabular-nums whitespace-nowrap',
        tier.className,
        className
      )}
    >
      {showDot && (
        <span
          className={cn(
            'inline-block size-1.5 rounded-full',
            tier.dotClassName
          )}
          aria-hidden='true'
        />
      )}
      {label}
    </span>
  )
}
