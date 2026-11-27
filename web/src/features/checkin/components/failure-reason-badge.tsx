// metapi-go features/checkin/components — FailureReason badge.
//
// The centerpiece of the check-in failure-reason work: the backend's
// ClassifyFailureReason now persists a structured `failureReason` object on
// every non-success log row, and this component renders it with dual-channel
// geometric SoftToneBadge tokens so operators can triage failures at a glance
// without relying on color alone.
//
// Category → tone mapping (mirrors service/checkin/failure_reason.go):
//   auth         → destructive (red, X-mark)      — credentials expired, re-login
//   verification → warning     (amber, triangle)  — Cloudflare / Turnstile, human action
//   network      → info        (blue, diamond)    — transient, retry later
//   site         → neutral     (gray, ring)       — site-side issue
//   state        → success     (green, dot)       — already checked in, not an error
//   unknown      → neutral     (gray, ring)       — investigate

import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'

import type { FailureReason, FailureReasonCategory } from '../types'

interface FailureCategoryConfig {
  tone: SoftTone
  variant: 'destructive' | 'warning' | 'info' | 'success' | 'outline'
}

const FAILURE_CATEGORY_CONFIG: Record<
  FailureReasonCategory,
  FailureCategoryConfig
> = {
  auth: { tone: 'destructive', variant: 'destructive' },
  verification: { tone: 'warning', variant: 'warning' },
  network: { tone: 'info', variant: 'info' },
  site: { tone: 'neutral', variant: 'outline' },
  state: { tone: 'success', variant: 'success' },
  unknown: { tone: 'neutral', variant: 'outline' },
}

function resolveCategoryConfig(category: string): FailureCategoryConfig {
  const config = FAILURE_CATEGORY_CONFIG[category as FailureReasonCategory]
  return config ?? FAILURE_CATEGORY_CONFIG.unknown
}

interface FailureReasonBadgeProps {
  reason: FailureReason | null
  className?: string
}

export function FailureReasonBadge({
  reason,
  className,
}: FailureReasonBadgeProps) {
  const { t } = useTranslation()
  if (!reason || !reason.code) {
    return <span className='text-muted-foreground'>—</span>
  }

  const config = resolveCategoryConfig(reason.category)
  // Titles/hints arrive from the backend in English; the classifier codes are
  // stable, so localize by code with the backend string as fallback.
  const label = t(`checkin.failureReason.code.${reason.code}.title`, {
    defaultValue: reason.title || reason.code,
  })
  const localizedDetail = t(
    `checkin.failureReason.code.${reason.code}.detail`,
    {
      defaultValue: reason.detailHint || '',
    }
  )
  const localizedAction = t(
    `checkin.failureReason.code.${reason.code}.action`,
    {
      defaultValue: reason.actionHint || '',
    }
  )
  const tooltip = localizedDetail || localizedAction || undefined

  return (
    <SoftToneBadge
      tone={config.tone}
      data-slot='badge'
      data-variant={config.variant}
      className={className}
      title={tooltip}
    >
      <span className='max-w-35 truncate'>{label}</span>
    </SoftToneBadge>
  )
}
