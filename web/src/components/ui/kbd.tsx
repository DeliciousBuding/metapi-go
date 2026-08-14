// metapi-go/ui — keyboard key-cap badge (e.g. ⌘K hints), base-nova style.
import { cn } from '@/lib/utils'

/**
 * Detect whether the current platform uses the macOS key convention (⌘)
 * instead of Ctrl. `navigator.platform` is deprecated but universally
 * supported; fall back to the user agent string when it is empty.
 */
export function isMacPlatform(): boolean {
  if (typeof navigator === 'undefined') return false
  const platform = navigator.platform || ''
  if (platform) return /mac|iphone|ipad|ipod/i.test(platform)
  return /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent)
}

function Kbd({ className, ...props }: React.ComponentProps<'kbd'>) {
  return (
    <kbd
      data-slot='kbd'
      className={cn(
        'bg-muted text-muted-foreground pointer-events-none inline-flex h-5 w-fit min-w-5 items-center justify-center gap-1 rounded-sm border px-1 font-sans text-xs font-medium select-none',
        className
      )}
      {...props}
    />
  )
}

export { Kbd }
