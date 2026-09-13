// metapi-go/components/common — section loading skeleton, the single
// SectionSkeleton implementation.
//
// Two shapes share this one owner:
//
//   shelled (default) — the generic "a card section is fetching" placeholder:
//     header lines plus three body rows inside a Card. Sections inside the
//     settings workspace use it while their runtime query resolves, and the
//     standalone downstream-keys page uses it as the Suspense fallback for
//     its lazily loaded section.
//
//   shelled={false} — bare shimmer bars with no Card chrome, for Suspense
//     fallbacks at dashboard/observability section boundaries where the
//     section renders its own card once loaded.
//
// It lives here rather than under features/settings because several features
// need it, and it has no feature dependency of its own. (The bare variant
// used to be a second same-named component under components/ui; the pair
// merged here in #1359 so the name resolves to exactly one implementation.)

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function SectionSkeleton({ shelled = true }: { shelled?: boolean }) {
  if (!shelled) {
    return (
      <div className='space-y-4 p-4' aria-busy='true' aria-live='polite'>
        <Skeleton className='h-6 w-48' />
        <Skeleton className='h-4 w-full' />
        <Skeleton className='h-4 w-3/4' />
        <Skeleton className='h-32 w-full' />
        <Skeleton className='h-4 w-full' />
        <Skeleton className='h-4 w-2/3' />
      </div>
    )
  }
  return (
    <Card>
      <CardHeader>
        <Skeleton className='h-5 w-40' />
        <Skeleton className='h-4 w-64' />
      </CardHeader>
      <CardContent className='space-y-4'>
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-1/2' />
      </CardContent>
    </Card>
  )
}
