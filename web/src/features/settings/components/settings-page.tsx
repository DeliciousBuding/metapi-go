// metapi-go/features/settings — generic dispatcher.
//
// Each per-subarea route renders this component with its subarea config + the
// active section id (read from the `$section` route param by the route file).
// The dispatcher lays out the settings sidebar (sections) + the active
// section's content. Keeping it presentational (no URL reading) means route
// files stay ~10 lines and the component is trivially testable.
//
// The header is a breadcrumb (Settings / subarea / section): each section
// card already carries its own h1 + description, so the page header only
// needs to say where the user is inside the settings tree. The subarea
// crumb links back to the subarea's default section so users can step up
// one level without losing the drill-in context.
//
// Phase 3 will extend this to fetch the runtime-settings map and pass it into
// each section's `build` (mirroring newapi's SettingsPage + useSystemOptions).

import { Link, type LinkProps } from '@tanstack/react-router'
import { Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'

import type { SettingsSubarea } from '../types'
import { SettingsSectionSkeleton } from './settings-section-card'
import { SettingsSidebar } from './settings-sidebar'

type SettingsPageProps = {
  /** The assembled subarea (sections + nav + content). */
  subarea: SettingsSubarea
  /** Active section id (from the `$section` route param). */
  activeSection: string
}

export function SettingsPage({ subarea, activeSection }: SettingsPageProps) {
  const { t } = useTranslation()
  const navItems = subarea.getSectionNavItems()
  const activeSectionMeta = subarea.getSectionMeta(activeSection)

  return (
    <div className='flex flex-col gap-6 p-6'>
      <header>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <Link
                to={'/settings' as LinkProps['to'] | (string & {})}
                // WCAG 2.5.8 best-effort (F-line residual D): 20px text links
                // get `py-0.5` click padding → 24px hit height; the matching
                // `-my-0.5` keeps the breadcrumb row exactly as tall as before.
                className='hover:text-foreground -my-0.5 py-0.5 transition-colors'
              >
                {t('settings.overview.title')}
              </Link>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <Link
                to={
                  `${subarea.basePath}/${subarea.defaultSection}` as
                    | LinkProps['to']
                    | (string & {})
                }
                className='hover:text-foreground -my-0.5 py-0.5 transition-colors'
              >
                {t(subarea.title)}
              </Link>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t(activeSectionMeta.title)}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </header>
      <div className='flex flex-col gap-6 lg:flex-row lg:items-start'>
        <SettingsSidebar items={navItems} title={t(subarea.title)} />
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
    </div>
  )
}
