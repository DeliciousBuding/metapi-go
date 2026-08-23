// metapi-go/features/settings — generic dispatcher.
//
// Each per-subarea route renders this component with its subarea config + the
// active section id (read from the `$section` route param by the route file).
// The dispatcher renders the single-h1 page header (section title +
// description) and the active section's content. Keeping it presentational
// (no URL reading) means route files stay ~10 lines and the component is
// trivially testable.
//
// IA restructure (wave 8 lane C): the breadcrumb header and the in-page
// settings sidebar were removed — the main sidebar's nested collapsible tree
// (components/layout/config/system-settings.config.ts) is now the single
// navigation surface, so the page header only needs the section title.
// Section cards carry their own h2 title + description.
//
// Phase 3 will extend this to fetch the runtime-settings map and pass it into
// each section's `build` (mirroring newapi's SettingsPage + useSystemOptions).

import { Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import type { SettingsSubarea } from '../types'
import { SettingsSectionSkeleton } from './settings-section-card'

type SettingsPageProps = {
  /** The assembled subarea (sections + nav + content). */
  subarea: SettingsSubarea
  /** Active section id (from the `$section` route param). */
  activeSection: string
}

export function SettingsPage({ subarea, activeSection }: SettingsPageProps) {
  const { t } = useTranslation()
  const activeSectionMeta = subarea.getSectionMeta(activeSection)

  return (
    <div className='flex flex-col gap-6 p-6'>
      <header className='flex flex-col gap-1'>
        <h1 className='text-lg font-bold tracking-tight sm:text-xl'>
          {t(activeSectionMeta.title)}
        </h1>
        {activeSectionMeta.description ? (
          <p className='text-muted-foreground text-sm'>
            {t(activeSectionMeta.description)}
          </p>
        ) : null}
      </header>
      <main className='min-w-0 flex-1'>
        {/* Single Suspense boundary catches the React.lazy sections emitted
            by each subarea registry's build(). On the first visit to a
            section its chunk suspends and the settings-shaped skeleton
            shows; on revisits the lazy module is already resolved so the
            section renders instantly without re-fetching. */}
        <Suspense fallback={<SettingsSectionSkeleton />}>
          {subarea.getSectionContent(activeSection)}
        </Suspense>
      </main>
    </div>
  )
}
