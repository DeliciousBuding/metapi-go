// metapi-go/ui — notice primitive: the one owner of the bordered soft-fill
// notice recipe (W19-T2 §3.2 consolidation). Before this landed, 10+ pages
// hand-rolled the same shape with drifting padding (p-2 / p-2.5 / p-3 /
// px-3 py-2), radius (md / lg), text size (xs / sm) and border alpha
// (/30 / /35 / /40). The canonical recipe is the query-error-banner one:
// rounded-lg border, soft /10 fill, /40 border, <tone>-soft-fg text (the
// soft-fg tokens exist because base-on-soft text is sub-AA — see
// contrast-gate.test.ts). `size='compact'` is the only sanctioned variant
// for dense surfaces (dialogs, sheets, form hints).

import * as React from 'react'

import { cn } from '@/lib/utils'

export type NoticeTone = 'destructive' | 'warning' | 'info' | 'success'

const TONE_CLASSES: Record<NoticeTone, string> = {
  destructive:
    'border-destructive/40 bg-destructive/10 text-destructive-soft-fg',
  warning: 'border-warning/40 bg-warning/10 text-warning-soft-fg',
  info: 'border-info/40 bg-info/10 text-info-soft-fg',
  success: 'border-success/40 bg-success/10 text-success-soft-fg',
}

type NoticeProps = React.ComponentProps<'div'> & {
  /** Semantic color of the notice surface. */
  tone: NoticeTone
  /** `compact` tightens padding/text for dense surfaces (dialogs, sheets). */
  size?: 'default' | 'compact'
}

function Notice({ tone, size = 'default', className, ...props }: NoticeProps) {
  return (
    <div
      data-slot='notice'
      className={cn(
        'flex items-start gap-2 rounded-lg border text-sm',
        size === 'compact' ? 'p-2 text-xs' : 'p-3',
        TONE_CLASSES[tone],
        className
      )}
      {...props}
    />
  )
}

export { Notice }
