// metapi-go/features/downstream-keys — the /downstream-keys route body.
// Key management used to live only inside the Settings workspace; operators
// asked for it as a first-class left-nav surface, so it is its own route now
// and `/settings/downstream/keys` redirects here.
//
// `./components/keys-section` owns the whole list / create / edit / delete /
// connect flow; this page only supplies the page-level header and the Suspense
// boundary for the section's lazy chunk.

import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionSkeleton } from '@/components/common/section-skeleton'

const LazyKeysSection = lazy(() =>
  import('./components/keys-section').then((module) => ({
    default: module.KeysSection,
  }))
)

export function DownstreamKeysPage() {
  const { t } = useTranslation()

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='page-title'>{t('downstreamKeys.page.title')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('downstreamKeys.page.description')}
        </p>
      </div>
      <Suspense fallback={<SectionSkeleton />}>
        <LazyKeysSection />
      </Suspense>
    </div>
  )
}
