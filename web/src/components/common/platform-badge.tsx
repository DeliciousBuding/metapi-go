import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import {
  getPlatformDefinition,
  getPlatformDisplayName,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

export function PlatformBadge(props: {
  platform: string | null | undefined
  className?: string
}) {
  const name = getPlatformDisplayName(props.platform)
  if (!name) return <span className='text-muted-foreground'>—</span>

  return (
    <span
      className={cn(
        'inline-flex max-w-full items-center gap-1.5 rounded-full border bg-muted/30 py-0.5 pr-2.5 pl-1.5 text-xs font-medium',
        props.className
      )}
      title={name}
    >
      <span aria-hidden='true' className='inline-flex shrink-0'>
        <BrandGlyph
          icon={getPlatformDefinition(props.platform)?.icon}
          size={14}
          fallbackText={name}
        />
      </span>
      <span className='min-w-0 truncate'>{name}</span>
    </span>
  )
}
