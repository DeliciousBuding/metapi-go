// metapi-go/components/common — the soft-tone pill recipe, single owner.
//
// The bordered soft-tint pill shared by HttpStatusBadge (here in common) and
// LatencyBadge (features/proxy-logs): `rounded-4xl` pill, `/10` tone fill,
// `/30` tone border, `<tone>-soft-fg` ink, optional status dot. Both badges
// keep their own tier-resolution logic (HTTP code → band, latency ms →
// tier); the class recipe lives here exactly once so the two implementations
// can no longer drift — they were character-for-character copies before
// #1359 merged them.
//
// The border alpha is /30 on purpose: at pill size a /40 border reads
// heavier than the label it frames (the larger Notice surface uses /40).
// The soft-fg inks exist because base-tone-on-soft text is sub-AA — see
// styles/__tests__/contrast-gate.test.ts.

import * as React from 'react'

import { cn } from '@/lib/utils'

export type SoftTone =
  | 'success'
  | 'info'
  | 'warning'
  | 'destructive'
  | 'neutral'

// Module-private: the recipe table belongs to this file's components only.
const SOFT_TONE_BADGE_TONES: Record<
  SoftTone,
  { className: string; dotClassName: string }
> = {
  success: {
    className: 'bg-success/10 text-success-soft-fg border-success/30',
    dotClassName: 'bg-success',
  },
  info: {
    className: 'bg-info/10 text-info-soft-fg border-info/30',
    dotClassName: 'bg-info',
  },
  warning: {
    className: 'bg-warning/10 text-warning-soft-fg border-warning/30',
    dotClassName: 'bg-warning',
  },
  destructive: {
    className:
      'bg-destructive/10 text-destructive-soft-fg border-destructive/30',
    dotClassName: 'bg-destructive',
  },
  neutral: {
    className: 'bg-muted/40 text-muted-foreground border-border',
    dotClassName: 'bg-muted-foreground',
  },
}

const SOFT_TONE_BADGE_BASE =
  'inline-flex w-fit items-center gap-1 rounded-4xl border px-1.5 py-0.5 text-xs font-medium tabular-nums whitespace-nowrap'

export function SoftToneDot({
  tone,
  className,
}: {
  tone: SoftTone
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-block size-1.5 rounded-full',
        SOFT_TONE_BADGE_TONES[tone].dotClassName,
        className
      )}
      aria-hidden='true'
    />
  )
}

export type SoftToneBadgeProps = React.ComponentProps<'span'> & {
  tone: SoftTone
  /** Render the leading status dot (default true). */
  showDot?: boolean
}

export function SoftToneBadge({
  tone,
  showDot = true,
  className,
  children,
  ...props
}: SoftToneBadgeProps) {
  return (
    <span
      className={cn(
        SOFT_TONE_BADGE_BASE,
        SOFT_TONE_BADGE_TONES[tone].className,
        className
      )}
      {...props}
    >
      {showDot ? <SoftToneDot tone={tone} /> : null}
      {children}
    </span>
  )
}
