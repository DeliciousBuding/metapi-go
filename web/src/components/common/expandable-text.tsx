import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type ExpandableTextProps = {
  text: string
  className?: string
  /** Short values remain selectable without an unnecessary disclosure control. */
  threshold?: number
}

export function ExpandableText({
  text,
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
          className
        )}
      >
        {text}
      </div>
      {canCollapse && (
        <button
          type='button'
          className='text-primary hover:text-primary/80 focus-visible:ring-focus-ring mt-1 rounded-sm text-xs leading-snug font-medium underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:outline-none'
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
