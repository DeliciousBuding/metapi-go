// metapi-go/components/common — card shell for a settings-style section: a
// Card with an optional header (title + description) and an optional
// right-aligned actions row. Keeping that layout here lets each section file
// focus on its own fields instead of repeating markup. Hosts are the settings
// sections and the standalone downstream-keys page. The loading placeholder
// lives beside it in `./section-skeleton`.

import type { ReactNode } from 'react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'

type SectionCardProps = {
  title: string
  description?: string
  /** Slot for the section content (form fields, tables, etc.). */
  children: ReactNode
  /** Optional right-aligned actions in the header (test/save buttons). */
  actions?: ReactNode
  /**
   * Single-card sections: the host page header already renders the section's
   * unique h1 + description, so a card header repeating the same
   * title/description is verbatim duplication. When true, the actions row
   * still renders (buttons need a host) but the title/description copy is
   * dropped. Multi-card sections that need a per-card h2 omit this.
   */
  hideHeaderCopy?: boolean
}

/**
 * Card shell with a translated title + description. Header actions (test /
 * save) render right-aligned so long sections stay scannable.
 *
 * The header is omitted entirely when `actions` is absent: a single-card
 * section's title + description already render in its page header (the single
 * h1, fed from the same i18n keys), so a headerless card avoids the duplicated
 * "title / same description twice" stack. Cards with actions keep their header
 * — the buttons need a host — but single-card sections pass `hideHeaderCopy`
 * so the h2 does not re-state copy the page header already rendered (see
 * DESIGN.md 4.2).
 */
export function SectionCard({
  title,
  description,
  children,
  actions,
  hideHeaderCopy = false,
}: SectionCardProps) {
  return (
    <Card>
      {actions ? (
        <CardHeader className='flex flex-row items-start justify-between gap-4'>
          {hideHeaderCopy ? null : (
            <div className='space-y-1'>
              {/* h2: the unique page-level h1 lives in the host page header
                  (single-h1 discipline); card titles are L2. */}
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
