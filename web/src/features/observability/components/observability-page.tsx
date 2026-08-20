// metapi-go/features/observability/components — workspace shell + section
// dispatcher. Renders the hub header + section tabs + the active section's
// content. Presentational (no URL reading); the route layer passes the active
// section and a navigate-backed `onSectionChange`, mirroring DashboardPage.

import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  OBSERVABILITY_DEFAULT_SECTION,
  getObservabilitySectionContent,
  getObservabilitySectionNavItems,
} from '../config/observability-config'
import { ObservabilityAutoRefreshProvider } from '../context/auto-refresh-context'
import type { ObservabilitySectionId } from '../types'
import { ObservabilityAutoRefreshToggle } from './observability-auto-refresh-toggle'

export type ObservabilityPageProps = {
  activeSection?: string
  onSectionChange?: (sectionId: ObservabilitySectionId) => void
}

function resolveSectionId(
  activeSection: string | undefined
): ObservabilitySectionId {
  if (!activeSection) return OBSERVABILITY_DEFAULT_SECTION
  const known = new Set<string>(
    getObservabilitySectionNavItems().map(
      (item) => item.url.split('=').pop() ?? ''
    )
  )
  return known.has(activeSection)
    ? (activeSection as ObservabilitySectionId)
    : OBSERVABILITY_DEFAULT_SECTION
}

export function ObservabilityPage({
  activeSection,
  onSectionChange,
}: ObservabilityPageProps) {
  const { t } = useTranslation()
  const sectionId = resolveSectionId(activeSection)
  const navItems = getObservabilitySectionNavItems()

  return (
    <ObservabilityAutoRefreshProvider>
      <div className='flex flex-col gap-6 p-6'>
        <header className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex flex-col gap-1'>
            <h1 className='text-2xl font-normal tracking-tight'>
              {t('observability.title')}
            </h1>
            <p className='text-muted-foreground text-sm'>
              {t('observability.description')}
            </p>
          </div>
          <ObservabilityAutoRefreshToggle />
        </header>

        <Tabs
          value={sectionId}
          onValueChange={(value) =>
            onSectionChange?.(value as ObservabilitySectionId)
          }
        >
          <TabsList className='w-fit max-w-full overflow-x-auto'>
            {navItems.map((item) => {
              const id = item.url.split('=').pop() ?? item.title
              return (
                <TabsTrigger key={id} value={id}>
                  {t(item.title)}
                </TabsTrigger>
              )
            })}
          </TabsList>
        </Tabs>

        <main className='min-w-0 flex-1'>
          {getObservabilitySectionContent(sectionId)}
        </main>
      </div>
    </ObservabilityAutoRefreshProvider>
  )
}
