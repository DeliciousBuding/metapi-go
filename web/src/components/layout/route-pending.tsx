// metapi-go/layout — route transition pending skeleton.
//
// Rendered by TanStack Router as the `defaultPendingComponent` while a
// code-split route's chunk + loader are in flight (see main.tsx). Replaces
// the blank (black in dark mode) content area that otherwise flashes during
// slow route transitions with a design-system skeleton that mirrors the
// shared page shell (p-6 header + content) used across features.
//
// Static (no data, no Suspense) so it can never itself suspend, which is what
// makes it a valid Suspense fallback. (Section-level loading uses the shared
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
      className='flex flex-col gap-6 p-6'
    >
      <header className='flex flex-col gap-2'>
        <Skeleton className='h-7 w-48 max-w-full' />
        <Skeleton className='h-4 w-72 max-w-full' />
      </header>
      <Skeleton className='h-64 w-full rounded-lg' />
      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3'>
        <Skeleton className='h-24 rounded-lg' />
        <Skeleton className='h-24 rounded-lg' />
        <Skeleton className='h-24 rounded-lg' />
      </div>
    </div>
  )
}
