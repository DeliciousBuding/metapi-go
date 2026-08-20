// metapi-go/layout — application header.
// Brand on the left; global-search trigger, the shared language /
// appearance / color-scheme controls, and the user menu (version, About,
// documentation, sign-out) on the right.

import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { UserMenu } from '@/components/layout/components/user-menu'
import { Button } from '@/components/ui/button'
import { isMacPlatform, Kbd } from '@/components/ui/kbd'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { metapiIdentity } from '@/lib/identity-branding'
import { cn } from '@/lib/utils'

type AppHeaderProps = {
  /** Whether to show the light/dark theme toggle button. */
  showThemeToggle?: boolean
  /** Custom left content, overrides the brand if provided. */
  leftContent?: React.ReactNode
  /** Custom right content, overrides the default interface controls if provided. */
  rightContent?: React.ReactNode
  /** Called when the global-search trigger is clicked. */
  onSearchClick?: () => void
}

function SearchTrigger({ onClick }: { onClick?: () => void }) {
  const { t } = useTranslation()
  const modifierKey = isMacPlatform() ? '⌘' : 'Ctrl'

  return (
    <Button
      variant='outline'
      className='text-muted-foreground dark:border-input dark:bg-input/30 h-8 gap-2 px-2.5 font-normal'
      onClick={onClick}
      aria-label={t('search.trigger')}
    >
      <Search className='size-4' />
      <span className='hidden md:inline'>{t('search.trigger')}</span>
      <span className='hidden items-center gap-1 md:flex'>
        <Kbd>{modifierKey}</Kbd>
        <Kbd>K</Kbd>
      </span>
    </Button>
  )
}

export function AppHeader({
  showThemeToggle = true,
  leftContent,
  rightContent,
  onSearchClick,
}: AppHeaderProps) {
  return (
    <header
      className={cn(
        'bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg',
        'sticky top-0 z-50 w-full',
        'flex h-[var(--app-header-height)] items-center gap-2 border-b px-4'
      )}
    >
      <SidebarTrigger className='md:hidden' />
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
        <div className='ms-auto flex items-center gap-1'>
          <SearchTrigger onClick={onSearchClick} />
          <InterfaceControls showThemeToggle={showThemeToggle} />
          <UserMenu />
        </div>
      )}
    </header>
  )
}
