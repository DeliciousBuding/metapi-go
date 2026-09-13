// metapi-go/components/common — HttpStatusBadge: an HTTP outcome as a pill.
//
// Shared rather than feature-local because two features render it: proxy logs
// (per row, from a numeric status) and the observability overview (recent
// failures, where the upstream may report a word instead of a code). Both need
// the same three rules, which is what this file owns:
//
//   a number  → its class band (2xx/3xx/4xx/5xx), rendered as the number
//   a word    → mapped to the nearest band ("timeout" is a failure, not an
//               unknown), rendered as the translated word
//   neither   → the neutral band and the translated "unknown"
//
// The label is never invented: an unrecognised string is shown verbatim, so a
// new upstream status reads as itself instead of silently becoming "unknown".
//
// The pill recipe itself (soft-tint classes, dot) is owned by
// ./soft-tone-badge — shared with features/proxy-logs' LatencyBadge.

import { useTranslation } from 'react-i18next'

import type { SoftTone } from './soft-tone-badge'
import { SoftToneBadge } from './soft-tone-badge'

type StatusTier = {
  tone: SoftTone
  fallbackLabelKey: string
}

const STATUS_TIERS = {
  success: {
    tone: 'success',
    fallbackLabelKey: 'httpStatus.success',
  },
  redirect: {
    tone: 'info',
    fallbackLabelKey: 'httpStatus.redirect',
  },
  clientError: {
    tone: 'warning',
    fallbackLabelKey: 'httpStatus.clientError',
  },
  serverError: {
    tone: 'destructive',
    fallbackLabelKey: 'httpStatus.serverError',
  },
  neutral: {
    tone: 'neutral',
    fallbackLabelKey: 'httpStatus.unknown',
  },
} as const satisfies Record<string, StatusTier>

function resolveTierFromHttpStatus(httpStatus: number): StatusTier {
  if (httpStatus >= 200 && httpStatus < 300) return STATUS_TIERS.success
  if (httpStatus >= 300 && httpStatus < 400) return STATUS_TIERS.redirect
  if (httpStatus >= 400 && httpStatus < 500) return STATUS_TIERS.clientError
  if (httpStatus >= 500) return STATUS_TIERS.serverError
  return STATUS_TIERS.neutral
}

function resolveTierFromStatusString(status: string): {
  tier: StatusTier
  labelKey: string | null
} {
  const normalized = status.toLowerCase()
  if (['success', 'ok', 'succeeded', 'succeed'].includes(normalized)) {
    return { tier: STATUS_TIERS.success, labelKey: 'httpStatus.success' }
  }
  if (
    ['failed', 'error', 'failure', 'timeout', 'timeouterror'].includes(
      normalized
    )
  ) {
    return {
      tier: STATUS_TIERS.serverError,
      labelKey: 'httpStatus.failed',
    }
  }
  if (normalized.includes('redirect')) {
    return {
      tier: STATUS_TIERS.redirect,
      labelKey: 'httpStatus.redirect',
    }
  }
  if (normalized.includes('client')) {
    return {
      tier: STATUS_TIERS.clientError,
      labelKey: 'httpStatus.clientError',
    }
  }
  return { tier: STATUS_TIERS.neutral, labelKey: null }
}

function resolveTier(
  httpStatus: number | null | undefined,
  status: string | null | undefined
): { tier: StatusTier; labelKey: string | null; rawLabel: string | null } {
  const numericStatus =
    typeof httpStatus === 'number' && httpStatus > 0 ? httpStatus : null
  if (numericStatus !== null) {
    return {
      tier: resolveTierFromHttpStatus(numericStatus),
      labelKey: null,
      rawLabel: String(numericStatus),
    }
  }
  const statusString =
    typeof status === 'string' && status.trim().length > 0
      ? status.trim()
      : null
  if (statusString !== null) {
    const parsed = Number.parseInt(statusString, 10)
    if (Number.isFinite(parsed) && parsed > 0) {
      return {
        tier: resolveTierFromHttpStatus(parsed),
        labelKey: null,
        rawLabel: String(parsed),
      }
    }
    const resolved = resolveTierFromStatusString(statusString)
    return {
      tier: resolved.tier,
      labelKey: resolved.labelKey,
      rawLabel: statusString,
    }
  }
  return {
    tier: STATUS_TIERS.neutral,
    labelKey: STATUS_TIERS.neutral.fallbackLabelKey,
    rawLabel: null,
  }
}

export type HttpStatusBadgeProps = {
  httpStatus?: number | null
  status?: string | null
  className?: string
  showDot?: boolean
}

export function HttpStatusBadge({
  httpStatus,
  status,
  className,
  showDot = true,
}: HttpStatusBadgeProps) {
  const { t } = useTranslation()
  const resolved = resolveTier(httpStatus, status)
  const tier = resolved.tier
  const label = resolved.labelKey
    ? t(resolved.labelKey)
    : (resolved.rawLabel ?? '')
  return (
    <SoftToneBadge
      tone={tier.tone}
      showDot={showDot}
      title={t('httpStatus.titlePrefix', { label })}
      className={className}
    >
      {label}
    </SoftToneBadge>
  )
}
