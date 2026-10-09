// metapi-go/components/common — model pill: a compact capsule that pairs a
// model name with its brand glyph (BrandIcon) wherever model identifiers
// appear in dense tables (proxy logs first). Token-driven surface only;
// Unknown models retain a deterministic lettermark instead of a guessed logo.

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { cn } from '@/lib/utils'

export type ModelPillProps = {
  /** Resolved model name shown in the pill (usually the actual upstream one). */
  model: string
  label?: string
  variant?: 'pill' | 'inline'
  /** Full label for the tooltip when the cell truncates (e.g. requested→actual). */
  title?: string
  className?: string
}

export function ModelPill({
  model,
  label,
  variant = 'pill',
  title,
  className,
}: ModelPillProps) {
  const name = model.trim()
  if (!name) return null
  return (
    <span
      className={cn(
        'inline-flex min-h-5 max-w-full items-center gap-1.5 align-middle font-sans text-sm leading-5 font-medium tracking-normal',
        variant === 'pill' &&
          'rounded-full border bg-muted/40 py-0.5 pr-2.5 pl-1.5',
        className
      )}
      title={title ?? name}
    >
      <span
        className='inline-flex size-4 shrink-0 items-center justify-center'
        aria-hidden='true'
      >
        <BrandGlyph model={name} size={14} />
      </span>
      <span className='block min-w-0 truncate leading-5'>{label || name}</span>
    </span>
  )
}
