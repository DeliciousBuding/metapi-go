// metapi-go/features/dashboard/components — section dispatcher.
//
// Presentational shell that lays out the dashboard header + section tabs +
// the active section's content. Kept presentational (no URL reading) so the
// component is trivially testable and route files stay thin — the route layer
// reads the `$section` param and passes `activeSection` + `onSectionChange`
// (TanStack Router navigate), matching the settings feature's SettingsPage
// pattern. When `onSectionChange` is omitted the tabs render read-only
// (useful for embeds / tests).
//
// Phase 3: lift shared section state here (chart preferences, time-range
// filters) once the traffic / models sections grow their own filter controls,
// mirroring newapi's dashboard/index.tsx parent-owned state.

import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  DASHBOARD_DEFAULT_SECTION,
  getDashboardSectionContent,
  getDashboardSectionMeta,
  getDashboardSectionNavItems,
} from '../config/dashboard-config'
import type { DashboardSectionId } from '../types'

export type DashboardPageProps = {
  /** Active section id (defaults to overview when unset / unknown). */
  activeSection?: string
  /**
   * Section change handler. When provided, tab clicks navigate via the route
   * layer; when omitted, the tabs are read-only.
   */
  onSectionChange?: (sectionId: DashboardSectionId) => void
}

function resolveSectionId(
  activeSection: string | undefined,
): DashboardSectionId {
  if (!activeSection) return DASHBOARD_DEFAULT_SECTION
  const known = new Set<string>(
    (getDashboardSectionNavItems() as Array<{ url: string }>).map(
      (item) => item.url.split('/').pop() ?? '',
    ),
  )
  return known.has(activeSection)
    ? (activeSection as DashboardSectionId)
    : DASHBOARD_DEFAULT_SECTION
}

export function DashboardPage({
  activeSection,
  onSectionChange,
}: DashboardPageProps) {
  const { t } = useTranslation()
  const sectionId = resolveSectionId(activeSection)
  const meta = getDashboardSectionMeta(sectionId)
  const navItems = getDashboardSectionNavItems()
  const content = getDashboardSectionContent(sectionId)

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

      <Tabs
        value={sectionId}
        onValueChange={(value) =>
          onSectionChange?.(value as DashboardSectionId)
        }
      >
        <TabsList className='w-fit'>
          {navItems.map((item) => {
            const id = item.url.split('/').pop() ?? item.title
            return (
              <TabsTrigger key={id} value={id}>
                {t(item.title)}
              </TabsTrigger>
            )
          })}
        </TabsList>
      </Tabs>

      <main className='min-w-0 flex-1'>{content}</main>
    </div>
  )
}
