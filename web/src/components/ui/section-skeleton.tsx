import { Skeleton } from '@/components/ui/skeleton'

/**
 * Lightweight content-area skeleton for Suspense fallbacks. Renders a few
 * shimmer bars to avoid the blank flash when lazy-loaded sections are
 * downloading their async chunk. Intentionally generic — no Card chrome —
 * so it works for both dashboard and observability section boundaries.
 */
export function SectionSkeleton() {
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
