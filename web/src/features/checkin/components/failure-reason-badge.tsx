/* eslint-disable react/only-export-components -- badge component co-located with category config */
// metapi-go features/checkin/components — FailureReason badge.
//
// The centerpiece of the 签到幽灵功能修复 (plan §5.5.5): the backend's
// ClassifyFailureReason now persists a structured `failureReason` object on
// every non-success log row, and this component renders it with
// category-coded colouring so operators can triage failures at a glance.
//
// Category → colour mapping (mirrors service/checkin/failure_reason.go):
//   auth         → destructive (red)    — credentials expired, re-login
//   verification → warning     (amber)  — Cloudflare / Turnstile, human action
//   network      → info        (blue)   — transient, retry later
//   site         → outline     (gray)   — site-side issue
//   state        → success     (green)  — already checked in, not an error
//   unknown      → outline     (gray)   — investigate

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { FailureReason, FailureReasonCategory } from '../types'

export interface FailureCategoryConfig {
  variant: 'destructive' | 'warning' | 'info' | 'success' | 'outline'
  dotClassName: string
}

const FAILURE_CATEGORY_CONFIG: Record<
  FailureReasonCategory,
  FailureCategoryConfig
> = {
  auth: { variant: 'destructive', dotClassName: 'bg-destructive' },
  verification: { variant: 'warning', dotClassName: 'bg-warning' },
  network: { variant: 'info', dotClassName: 'bg-info' },
  site: { variant: 'outline', dotClassName: 'bg-muted-foreground' },
  state: { variant: 'success', dotClassName: 'bg-success' },
  unknown: { variant: 'outline', dotClassName: 'bg-muted-foreground' },
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
    <Badge variant={config.variant} className={className} title={tooltip}>
      <span className={cn('size-1.5 rounded-full', config.dotClassName)} />
      <span className='max-w-[140px] truncate'>{label}</span>
    </Badge>
  )
}
