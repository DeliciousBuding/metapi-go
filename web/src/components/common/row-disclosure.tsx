import { ChevronRight } from 'lucide-react'
import {
  useEffect,
  useRef,
  type ComponentProps,
  type MouseEvent,
  type ReactNode,
} from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** Keep row-surface activation separate from controls inside the row. */
export function rowDisclosureClick(
  event: MouseEvent<HTMLElement>,
  onToggle: () => void
) {
  if (event.defaultPrevented || event.button !== 0) return
  const target = event.target
  if (!(target instanceof Element)) return
  if (
    target.closest(
      'button, a, input, select, textarea, summary, [role="button"], [role="checkbox"], [role="switch"], [role="menuitem"], [role="combobox"], [contenteditable="true"], [data-row-disclosure-ignore]'
    )
  )
    return
  // Finishing a text selection should leave the detail where the reader put it.
  if (window.getSelection()?.toString()) return
  onToggle()
}

export function RowDisclosureTrigger({
  expanded,
  controls,
  children,
  className,
  ...props
}: Omit<ComponentProps<typeof Button>, 'aria-expanded' | 'aria-controls'> & {
  expanded: boolean
  controls: string
}) {
  return (
    <Button
      variant='ghost'
      size='sm'
      className={cn(
        'h-auto min-h-8 min-w-0 justify-start gap-1.5 px-1 text-left whitespace-normal',
        className
      )}
      {...props}
      aria-expanded={expanded}
      aria-controls={controls}
    >
      <ChevronRight
        aria-hidden='true'
        className={cn(
          'size-4 transition-transform motion-reduce:transition-none',
          expanded && 'rotate-90'
        )}
      />
      {children}
    </Button>
  )
}

export function RowDisclosureContent({
  id,
  label,
  children,
  className,
}: {
  id: string
  label: string
  children: ReactNode
  className?: string
}) {
  const ref = useRef<HTMLDivElement>(null)

  // The row that opened may sit at the bottom of the viewport. Without this,
  // the disclosure is mounted below the fold and the click looks like a no-op.
  // Only nudge the nearest scroll container when the panel is actually out of
  // view, so expanding a visible row never moves the page.
  useEffect(() => {
    const element = ref.current
    if (!element || typeof element.scrollIntoView !== 'function') return
    const frame = requestAnimationFrame(() => {
      const rect = element.getBoundingClientRect()
      if (rect.top >= 0 && rect.bottom <= window.innerHeight) return
      const reduceMotion = window.matchMedia?.(
        '(prefers-reduced-motion: reduce)'
      ).matches
      element.scrollIntoView({
        block: 'nearest',
        behavior: reduceMotion ? 'auto' : 'smooth',
      })
    })
    return () => cancelAnimationFrame(frame)
  }, [])

  return (
    <div
      ref={ref}
      id={id}
      role='region'
      aria-label={label}
      data-row-disclosure-ignore
      className={cn(
        'animate-in fade-in-0 slide-in-from-top-1 min-w-0 scroll-mt-4 duration-150 motion-reduce:animate-none',
        className
      )}
    >
      {children}
    </div>
  )
}
