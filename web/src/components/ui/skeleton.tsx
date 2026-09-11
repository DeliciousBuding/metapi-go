// metapi-go/ui — skeleton (base-nova style, @base-ui/react). Based on shadcn/ui (MIT); adapted to metapi-go conventions.
import { cn } from '@/lib/utils'

function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='skeleton'
      className={cn('animate-shimmer rounded-md', className)}
      {...props}
    />
  )
}

export { Skeleton }
