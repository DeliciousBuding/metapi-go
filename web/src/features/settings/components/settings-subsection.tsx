// metapi-go/features/settings/components — card-internal subsection primitives.
//
// The settings heading system (P1, wave 9 lane B):
//   L1 page title            → unique h1 (SettingsPage header)
//   L2 card title            → h2 (SettingsSectionCard)
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
  children,
  className,
}: SettingsSubsectionProps) {
  return (
    <section
      className={cn(
        'space-y-3 border-t pt-4 first:border-t-0 first:pt-0',
        className
      )}
    >
      <h3 className='text-sm font-medium'>{title}</h3>
      {children}
    </section>
  )
}
