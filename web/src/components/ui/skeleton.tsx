// metapi-go/ui — skeleton component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
import { cn } from '@/lib/utils'

function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='skeleton'
      className={cn('bg-muted animate-pulse rounded-md', className)}
      {...props}
    />
  )
}

export { Skeleton }
