// metapi-go/components/common — card-shaped loading placeholder.
//
// The generic "a card section is fetching" skeleton: header lines plus three
// body rows. Sections inside the settings workspace use it while their runtime
// query resolves, and the standalone downstream-keys page uses it as the
// Suspense fallback for its lazily loaded section. It lives here rather than
// under features/settings because a second feature needs it, and it has no
// feature dependency of its own.

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function SectionSkeleton() {
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
