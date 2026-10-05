import { type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type ExpandableTextProps = {
  text: string
  /** Preserve rich content such as a safe external link while clamping it. */
  children?: ReactNode
  className?: string
  /** Short values remain selectable without an unnecessary disclosure control. */
  threshold?: number
}

export function ExpandableText({
  text,
  children,
  className,
  threshold = 96,
}: ExpandableTextProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const id = useId()
  const canCollapse = text.length > threshold || text.split('\n').length > 2

  return (
    <div className='min-w-0'>
      <div
        id={id}
        className={cn(
          'min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]',
          canCollapse && !expanded && 'line-clamp-2',
          canCollapse && !expanded && children && 'max-h-[2lh] overflow-hidden',
          className
        )}
      >
        {children ?? text}
      </div>
      {canCollapse && (
        <button
          type='button'
          className='text-primary hover:text-primary/80 focus-visible:ring-focus-ring mt-1.5 inline-flex min-h-10 min-w-10 items-center justify-center rounded-sm px-1 text-xs leading-snug font-medium underline underline-offset-2 focus-visible:ring-2 focus-visible:outline-none'
          aria-controls={id}
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {t(expanded ? 'common.showLess' : 'common.showMore')}
        </button>
      )}
    </div>
  )
}
