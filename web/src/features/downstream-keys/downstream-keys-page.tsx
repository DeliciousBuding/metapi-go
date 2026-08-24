// metapi-go/features/downstream-keys — standalone page for downstream API keys.
//
// Previously the downstream-keys management lived only inside the Settings
// workspace (settings/downstream/keys). Operators asked for it as a first-class
// left-nav surface, so this page promotes the same section component to a
// top-level route. The section is still the single source of truth for the
// key list / create / edit / delete / connect flow; this page only supplies the
// page-level header and a Suspense boundary for the section's lazy chunk.

import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import { SettingsSectionSkeleton } from '@/features/settings/components/settings-section-card'

const LazyKeysSection = lazy(() =>
  import('@/features/settings/sections/downstream/components/keys-section').then(
    (module) => ({
      default: module.KeysSection,
    })
  )
)

export function DownstreamKeysPage() {
  const { t } = useTranslation()

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='text-lg font-normal'>
          {t('downstreamKeys.page.title')}
        </h1>
        <p className='text-muted-foreground text-sm'>
          {t('downstreamKeys.page.description')}
        </p>
      </div>
      <Suspense fallback={<SettingsSectionSkeleton />}>
        <LazyKeysSection />
      </Suspense>
    </div>
  )
}
