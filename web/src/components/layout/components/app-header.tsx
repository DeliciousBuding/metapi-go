// metapi-go/layout — application header.
// Brand on the left; global-search trigger, the attention bell, the shared
// language / appearance / color-scheme controls, and the user menu (version,
// About, documentation, sign-out) on the right.

import { Link } from '@tanstack/react-router'
import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AttentionBell } from '@/components/layout/components/attention-bell'
import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { UserMenu } from '@/components/layout/components/user-menu'
import { Button } from '@/components/ui/button'
import { isMacPlatform, Kbd } from '@/components/ui/kbd'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { metapiIdentity } from '@/lib/identity-branding'
import { cn } from '@/lib/utils'

type AppHeaderProps = {
  /** Called when the global-search trigger is clicked. */
  onSearchClick?: () => void
}

function SearchTrigger({ onClick }: { onClick?: () => void }) {
  const { t } = useTranslation()
  const modifierKey = isMacPlatform() ? '⌘' : 'Ctrl'

  return (
    <Button
      variant='outline'
      className='text-muted-foreground dark:border-input dark:bg-input/30 h-8 gap-2 px-2.5 font-normal max-md:h-10 max-md:px-3'
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

export function AppHeader({ onSearchClick }: AppHeaderProps) {
  return (
    <header
      className={cn(
        'bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg',
        'sticky top-0 z-50 w-full',
        'flex h-[var(--app-header-height)] items-center gap-2 border-b px-4'
      )}
    >
      <SidebarTrigger className='max-md:size-10 md:hidden' />
      {/* The brand is the only flexible header cell: on narrow viewports it
          truncates so the right-hand controls (search, attention bell,
          interface controls, user menu) keep their full hit targets. It is a
          home link: `/` redirects to the default dashboard section (same
          target as the sidebar "Dashboard" entry). */}
      <Link
        to='/'
        className='flex min-w-0 items-center gap-2'
        aria-label={metapiIdentity.name}
      >
        <img
          src={metapiIdentity.logoPath}
          alt=''
          className='size-6 shrink-0 rounded-sm'
        />
        <span className='truncate text-sm font-semibold tracking-tight max-[480px]:hidden'>
          {metapiIdentity.name}
        </span>
      </Link>

      <div className='ms-auto flex shrink-0 items-center gap-1'>
        <SearchTrigger onClick={onSearchClick} />
        <AttentionBell />
        {/* Mobile gets the language/palette/theme controls at the drawer
            footer instead: seven 36px icons across a 375px bar was below
            touch-target guidance and crowded the brand. */}
        <InterfaceControls className='max-md:hidden' />
        <UserMenu />
      </div>
    </header>
  )
}
