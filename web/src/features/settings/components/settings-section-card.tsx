// metapi-go/features/settings/components — shared layout primitives used by
// every real section. Keeping the Card header + save-button layout here lets
// each section file focus on its own fields instead of repeating markup.

import type { ReactNode } from 'react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type SettingsSectionCardProps = {
  title: string
  description?: string
  /** Slot for the section content (form fields, tables, etc.). */
  children: ReactNode
  /** Optional right-aligned actions in the header (test/save buttons). */
  actions?: ReactNode
  /**
   * Single-card sections: the page header (SettingsPage) already renders the
   * section's unique h1 + description, so a card header that repeats the
   * same title/description is verbatim duplication. When true, the actions
   * row still renders (buttons need a host) but the title/description copy
   * is dropped. Multi-card sections that need a per-card h2 omit this.
   */
  hideHeaderCopy?: boolean
}

/**
 * Card shell with a translated title + description. Header actions (test /
 * save) render right-aligned so long sections stay scannable.
 *
 * The header is omitted entirely when `actions` is absent: every section's
 * title + description already render in the SettingsPage page header (the
 * single h1, fed from the same i18n keys), so a headerless card avoids the
 * duplicated "title / same description twice" stack that single-card
 * sections used to show. Cards with actions keep their header — the buttons
 * need a host — but single-card sections pass `hideHeaderCopy` so the h2
 * does not re-state copy the page header already rendered (see DESIGN.md 4.2).
 */
export function SettingsSectionCard({
  title,
  description,
  children,
  actions,
  hideHeaderCopy = false,
}: SettingsSectionCardProps) {
  return (
    <Card>
      {actions ? (
        <CardHeader className='flex flex-row items-start justify-between gap-4'>
          {hideHeaderCopy ? null : (
            <div className='space-y-1'>
              {/* h2: the unique page-level h1 lives in the SettingsPage header
                  (single-h1 discipline, wave 8 lane C); card titles are L2. */}
              <h2 className='text-base leading-snug font-medium group-data-[size=sm]/card:text-sm'>
                {title}
              </h2>
              {description ? (
                <CardDescription>{description}</CardDescription>
              ) : null}
            </div>
          )}
          <div className='flex shrink-0 gap-2'>{actions}</div>
        </CardHeader>
      ) : null}
      <CardContent>{children}</CardContent>
    </Card>
  )
}

/** Loading placeholder rendered while the runtime-settings query is fetching. */
export function SettingsSectionSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className='h-5 w-40' />
        <Skeleton className='h-4 w-64' />
      </CardHeader>
      <CardContent className='space-y-4'>
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-1/2' />
      </CardContent>
    </Card>
  )
}
