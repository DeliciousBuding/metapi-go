// metapi-go/layout — authenticated-layout adapted from newapi. AGPL header stripped.
// Simplified for skeleton: SidebarProvider + SkipToMain + AppHeader + flex row
// (AppSidebar + SidebarInset). Dropped LayoutProvider and SearchProvider (not in
// scope for skeleton; metapi has no global search). Uses children, not AnimatedOutlet,
// since route transitions are a later concern.

import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const defaultOpen = getCookie('sidebar_state') !== 'false'

  return (
    <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
      <SkipToMain />
      <AppHeader />
      <div className='flex min-h-0 w-full flex-1 [--app-header-height:3.5rem]'>
        <AppSidebar />
        <SidebarInset
          id='content'
          className={cn(
            '@container/content',
            'h-[calc(100svh-var(--app-header-height,0px))]',
            'min-h-0 overflow-hidden',
            'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
          )}
        >
          {props.children}
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}
