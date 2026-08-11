// metapi-go/layout — application header.
// Brand on the left; shared language, appearance, and color-scheme controls on the right.

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { metapiIdentity } from '@/lib/identity-branding'
import { cn } from '@/lib/utils'

type AppHeaderProps = {
  /** Whether to show the light/dark theme toggle button. */
  showThemeToggle?: boolean
  /** Custom left content, overrides the brand if provided. */
  leftContent?: React.ReactNode
  /** Custom right content, overrides the default interface controls if provided. */
  rightContent?: React.ReactNode
}

export function AppHeader({
  showThemeToggle = true,
  leftContent,
  rightContent,
}: AppHeaderProps) {
  return (
    <header
      className={cn(
        'bg-background/95 supports-[backdrop-filter]:bg-background/60',
        'sticky top-0 z-50 w-full',
        'flex h-14 items-center gap-2 border-b px-4',
        '[--app-header-height:3.5rem]'
      )}
    >
      {leftContent ?? (
        <div className='flex items-center gap-2'>
          <img
            src={metapiIdentity.logoPath}
            alt={metapiIdentity.name}
            className='size-6 rounded-sm'
          />
          <span className='text-sm font-semibold tracking-tight'>
            {metapiIdentity.name}
          </span>
        </div>
      )}

      {rightContent ?? (
        <InterfaceControls
          className='ms-auto'
          showThemeToggle={showThemeToggle}
        />
      )}
    </header>
  )
}
