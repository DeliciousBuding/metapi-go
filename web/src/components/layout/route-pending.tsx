// metapi-go/layout — route transition pending skeleton.
//
// Rendered by TanStack Router as the `defaultPendingComponent` while a
// code-split route's chunk + loader are in flight (see main.tsx). Replaces
// the blank (black in dark mode) content area that otherwise flashes during
// slow route transitions with a design-system skeleton.
//
// The shell mirrors the LIST page pattern (p-4 / gap-3 / text-lg title),
// which covers the majority of routes; hub-style pages (p-6 sections) may
// shift slightly on load — an acceptable trade for one static fallback that
// never suspends itself (no data, no Suspense), which is what makes it a
// valid Suspense fallback. (Section-level loading uses the shared
// `ui/section-skeleton` instead — this component is route-level only.)

import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

export function RoutePending() {
  const { t } = useTranslation()

  return (
    <div
      role='status'
      aria-busy='true'
      aria-label={t('common.loading')}
      className='flex h-full flex-col gap-3 p-4'
    >
      <header className='flex flex-col gap-2'>
        <Skeleton className='h-5 w-48 max-w-full' />
        <Skeleton className='h-4 w-72 max-w-full' />
      </header>
      <Skeleton className='h-9 w-full rounded-lg' />
      <Skeleton className='min-h-64 w-full flex-1 rounded-lg' />
    </div>
  )
}
