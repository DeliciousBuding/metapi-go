// metapi-go/features/settings — generic dispatcher.
//
// Each per-subarea route renders this component with its subarea config + the
// active section id (read from the `$section` route param by the route file).
// The dispatcher lays out the settings sidebar (sections) + the active
// section's content. Keeping it presentational (no URL reading) means route
// files stay ~10 lines and the component is trivially testable.
//
// Phase 3 will extend this to fetch the runtime-settings map and pass it into
// each section's `build` (mirroring newapi's SettingsPage + useSystemOptions).

import { useTranslation } from 'react-i18next'

import type { SettingsSubarea } from '../types'
import { SettingsSidebar } from './settings-sidebar'

type SettingsPageProps = {
  /** The assembled subarea (sections + nav + content). */
  subarea: SettingsSubarea
  /** Active section id (from the `$section` route param). */
  activeSection: string
}

export function SettingsPage({
  subarea,
  activeSection,
}: SettingsPageProps) {
  const { t } = useTranslation()
  const meta = subarea.getSectionMeta(activeSection)
  const navItems = subarea.getSectionNavItems()

  return (
    <div className='flex flex-col gap-6 p-6'>
      <header className='flex flex-col gap-1'>
        <h1 className='text-2xl font-semibold tracking-tight'>
          {t(meta.title)}
        </h1>
        {meta.description ? (
          <p className='text-muted-foreground text-sm'>
            {t(meta.description)}
          </p>
        ) : null}
      </header>
      <div className='flex gap-6'>
        <SettingsSidebar items={navItems} title={t(subarea.title)} />
        <main className='min-w-0 flex-1'>
          {subarea.getSectionContent(activeSection)}
        </main>
      </div>
    </div>
  )
}
