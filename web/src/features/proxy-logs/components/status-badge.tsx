// metapi-go/features/proxy-logs/components — HTTP status badge.
//
// Renders an HTTP status code with bucket-based colouring so the list column
// and the detail sheet share one visual vocabulary:
//   2xx → success (green)
//   3xx → info (blue) — redirect / cached
//   4xx → warning (amber) — client error
//   5xx → destructive (red) — server error
// The component also accepts the backend's coarse `status` string
// (`success` / `failed` / `error` …) as a fallback when the precise HTTP
// status isn't surfaced (e.g. the list row carries only the outcome string).

import { cn } from '@/lib/utils'

type StatusTier = {
  className: string
  dotClassName: string
  fallbackLabel: string
}

const STATUS_TIERS = {
  success: {
    className: 'bg-success/10 text-success border-success/30',
    dotClassName: 'bg-success',
    fallbackLabel: '成功',
  },
  redirect: {
    className: 'bg-info/10 text-info border-info/30',
    dotClassName: 'bg-info',
    fallbackLabel: '重定向',
  },
  clientError: {
    className: 'bg-warning/10 text-warning border-warning/30',
    dotClassName: 'bg-warning',
    fallbackLabel: '客户端错误',
  },
  serverError: {
    className: 'bg-destructive/10 text-destructive border-destructive/30',
    dotClassName: 'bg-destructive',
    fallbackLabel: '服务端错误',
  },
  neutral: {
    className: 'bg-muted/40 text-muted-foreground border-border',
    dotClassName: 'bg-muted-foreground',
    fallbackLabel: '未知',
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
  if (['success', 'ok', 'succeeded', 'succeed'].includes(normalized)) {
    return STATUS_TIERS.success
  }
  if (
    ['failed', 'error', 'failure', 'timeout', 'timeouterror'].includes(normalized)
  ) {
    return STATUS_TIERS.serverError
  }
  if (normalized.includes('redirect')) return STATUS_TIERS.redirect
  if (normalized.includes('client')) return STATUS_TIERS.clientError
  return STATUS_TIERS.neutral
}

function resolveTier(
  httpStatus: number | null | undefined,
  status: string | null | undefined,
): { tier: StatusTier; label: string } {
  const numericStatus = typeof httpStatus === 'number' && httpStatus > 0 ? httpStatus : null
  if (numericStatus !== null) {
    return {
      tier: resolveTierFromHttpStatus(numericStatus),
      label: String(numericStatus),
    }
  }

  const statusString =
    typeof status === 'string' && status.trim().length > 0 ? status.trim() : null
  if (statusString !== null) {
    // The status string may already be a numeric HTTP code ("500").
    const parsed = Number.parseInt(statusString, 10)
    if (Number.isFinite(parsed) && parsed > 0) {
      return {
        tier: resolveTierFromHttpStatus(parsed),
        label: String(parsed),
      }
    }
    return {
      tier: resolveTierFromStatusString(statusString),
      label: statusString,
    }
  }

  return { tier: STATUS_TIERS.neutral, label: STATUS_TIERS.neutral.fallbackLabel }
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
  const { tier, label } = resolveTier(httpStatus, status)

  return (
    <span
      title={`状态 ${label}`}
      className={cn(
        'inline-flex w-fit items-center gap-1 rounded-4xl border px-1.5 py-0.5 text-xs font-medium tabular-nums whitespace-nowrap',
        tier.className,
        className,
      )}
    >
      {showDot && (
        <span
          className={cn('inline-block size-1.5 rounded-full', tier.dotClassName)}
          aria-hidden='true'
        />
      )}
      {label}
    </span>
  )
}
