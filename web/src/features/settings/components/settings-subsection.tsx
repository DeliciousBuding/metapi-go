// metapi-go/features/settings/components — card-internal subsection primitives.
//
// The settings heading system:
//   L1 page title            → unique h1 (SettingsPage header)
//   L2 card title            → h2 (SectionCard, @/components/common/section-card)
//   L3 card subsection title → h3 (this module)
//
// `SettingsSubsection` is the full L3 zone: the h3 title plus a `border-t`
// separator so multiple flat zones inside one card read as distinct sections
// (e.g. import/export's export / import / WebDAV zones). Boxed zones that
// already carry their own `rounded-lg border` (e.g. schedule groups) render
// a plain h3 with the same `text-sm font-medium` styling — same L3 level,
// separator comes from the box.

import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type SettingsSubsectionProps = {
  /** Translated L3 title. */
  title: string
  description?: string
  icon?: ReactNode
  actions?: ReactNode
  variant?: 'flat' | 'panel'
  children: ReactNode
  className?: string
}

/**
 * Flat card zone with the L3 separator convention: `border-t pt-4` above
 * every zone after the first (the card header itself separates zone one).
 * Internal spacing matches the sections' existing `space-y-3` rhythm.
 */
export function SettingsSubsection({
  title,
  description,
  icon,
  actions,
  variant = 'flat',
  children,
  className,
}: SettingsSubsectionProps) {
  return (
    <section
      className={cn(
        'space-y-4',
        variant === 'panel'
          ? 'rounded-xl border bg-muted/20 p-4 sm:p-5'
          : 'border-t pt-5 first:border-t-0 first:pt-0',
        className
      )}
    >
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 flex-1 items-start gap-3'>
          {icon ? (
            <span
              className='bg-primary/10 text-primary grid size-10 shrink-0 place-items-center rounded-xl'
              aria-hidden='true'
            >
              {icon}
            </span>
          ) : null}
          <div className='min-w-0 space-y-1'>
            <h3 className='text-sm leading-snug font-semibold'>{title}</h3>
            {description ? (
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {description}
              </p>
            ) : null}
          </div>
        </div>
        {actions ? (
          <div className='flex flex-wrap items-center gap-2'>{actions}</div>
        ) : null}
      </div>
      {children}
    </section>
  )
}
