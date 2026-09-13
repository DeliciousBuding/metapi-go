// metapi-go/components/common — model pill: a compact capsule that pairs a
// model name with its brand glyph (BrandIcon) wherever model identifiers
// appear in dense tables (proxy logs first). Token-driven surface only;
// unknown models degrade to the plain truncated name (no empty pill).

import { InlineBrandIcon } from '@/assets/brand-icons/BrandIcon'
import { cn } from '@/lib/utils'

export type ModelPillProps = {
  /** Resolved model name shown in the pill (usually the actual upstream one). */
  model: string
  /** Full label for the tooltip when the cell truncates (e.g. requested→actual). */
  title?: string
  className?: string
}

export function ModelPill({ model, title, className }: ModelPillProps) {
  const name = model.trim()
  if (!name) return null
  return (
    <span
      className={cn(
        'inline-flex max-w-full items-center gap-1.5 rounded-full border bg-muted/40 py-0.5 pr-2.5 pl-1.5 text-xs font-medium',
        className
      )}
      title={title ?? name}
    >
      <InlineBrandIcon model={name} size={14} />
      <span className='min-w-0 truncate'>{name}</span>
    </span>
  )
}
