// metapi-go/ui — aspect-ratio component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
import { cn } from '@/lib/utils'

function AspectRatio({
  ratio,
  className,
  ...props
}: React.ComponentProps<'div'> & { ratio: number }) {
  return (
    <div
      data-slot='aspect-ratio'
      style={
        {
          '--ratio': ratio,
        } as React.CSSProperties
      }
      className={cn('relative aspect-(--ratio)', className)}
      {...props}
    />
  )
}

export { AspectRatio }
