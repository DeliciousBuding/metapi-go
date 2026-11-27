// metapi-go/components/common — the soft-tone pill recipe, single owner.
//
// The bordered soft-tint pill shared by HttpStatusBadge (here in common) and
// LatencyBadge (features/proxy-logs): `rounded-4xl` pill, `/10` tone fill,
// `/30` tone border, `<tone>-soft-fg` ink, and a dual-channel geometric status
// glyph (circle / diamond / triangle / cross / hollow ring) so severity stays
// legible without relying on color alone — including grayscale and color-vision
// deficiency modes — without gradients or left accent bars.
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
  { className: string; dotClassName: string; markerClassName: string }
> = {
  success: {
    className: 'bg-success/10 text-success-soft-fg border-success/30',
    dotClassName: 'bg-success',
    markerClassName: 'text-success',
  },
  info: {
    className: 'bg-info/10 text-info-soft-fg border-info/30',
    dotClassName: 'bg-info',
    markerClassName: 'text-info',
  },
  warning: {
    className: 'bg-warning/10 text-warning-soft-fg border-warning/30',
    dotClassName: 'bg-warning',
    markerClassName: 'text-warning',
  },
  destructive: {
    className:
      'bg-destructive/10 dark:bg-destructive/20 text-destructive-soft-fg border-destructive/30',
    dotClassName: 'bg-destructive',
    markerClassName: 'text-destructive',
  },
  neutral: {
    className: 'bg-muted/40 text-muted-foreground border-border',
    dotClassName: 'bg-muted-foreground',
    markerClassName: 'text-muted-foreground',
  },
}

const SOFT_TONE_BADGE_BASE =
  'inline-flex w-fit items-center gap-1.5 rounded-4xl border px-1.5 py-0.5 text-xs font-medium tabular-nums whitespace-nowrap'

/**
 * Dual-channel status marker: each semantic tone pairs its OKLCH color with a
 * distinct geometric silhouette (solid disc for success, diamond for info,
 * upward triangle for warning, crisp X-mark for destructive, hollow ring for
 * neutral) so badges stay distinguishable in monochrome or colorblind views.
 */
export function SoftToneDot({
  tone,
  className,
}: {
  tone: SoftTone
  className?: string
}) {
  const toneConfig = SOFT_TONE_BADGE_TONES[tone]

  if (tone === 'destructive') {
    return (
      <svg
        viewBox='0 0 8 8'
        aria-hidden='true'
        data-tone-marker={tone}
        className={cn(
          'inline-block size-2 shrink-0',
          toneConfig.markerClassName,
          className
        )}
      >
        <path
          d='M1.5 1.5L6.5 6.5M6.5 1.5L1.5 6.5'
          stroke='currentColor'
          strokeWidth='1.5'
          strokeLinecap='round'
        />
      </svg>
    )
  }

  if (tone === 'warning') {
    return (
      <svg
        viewBox='0 0 8 8'
        aria-hidden='true'
        data-tone-marker={tone}
        className={cn(
          'inline-block size-2 shrink-0',
          toneConfig.markerClassName,
          className
        )}
      >
        <path d='M4 1L7.25 6.75H0.75L4 1Z' fill='currentColor' />
      </svg>
    )
  }

  if (tone === 'info') {
    return (
      <svg
        viewBox='0 0 8 8'
        aria-hidden='true'
        data-tone-marker={tone}
        className={cn(
          'inline-block size-2 shrink-0',
          toneConfig.markerClassName,
          className
        )}
      >
        <path d='M4 0.75L7.25 4L4 7.25L0.75 4L4 0.75Z' fill='currentColor' />
      </svg>
    )
  }

  if (tone === 'neutral') {
    return (
      <svg
        viewBox='0 0 8 8'
        aria-hidden='true'
        data-tone-marker={tone}
        className={cn(
          'inline-block size-2 shrink-0',
          toneConfig.markerClassName,
          className
        )}
      >
        <circle
          cx='4'
          cy='4'
          r='2.5'
          fill='none'
          stroke='currentColor'
          strokeWidth='1.4'
        />
      </svg>
    )
  }

  return (
    <span
      data-tone-marker={tone}
      className={cn(
        'inline-block size-1.5 shrink-0 rounded-full',
        toneConfig.dotClassName,
        className
      )}
      aria-hidden='true'
    />
  )
}

export type SoftToneBadgeProps = React.ComponentProps<'span'> & {
  tone: SoftTone
  /** Render the leading status marker (default true). */
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
      data-tone={tone}
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
