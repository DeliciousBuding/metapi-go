// metapi-go/layout — AuthenticatedLayout: the shell every signed-in route
// renders inside.
//
// SidebarProvider → SkipToMain → AppHeader → (AppSidebar | SidebarInset), with
// TanStack's <Outlet /> putting the matched child route (/dashboard/*, /sites,
// /accounts, /settings/*, …) into the inset panel.
//
// Two things live here rather than in a page because they are shell-wide:
//   - the sidebar's initial open state, read from the `sidebar_state` cookie the
//     SidebarProvider itself persists, so a reload keeps the rail collapsed;
//   - the command palette's open state. SearchModal is mounted inside the router
//     on purpose — picking a result navigates.

import { Outlet } from '@tanstack/react-router'
import * as React from 'react'

import { SearchModal } from '@/components/layout/search-modal'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

export function AuthenticatedLayout() {
  const defaultOpen = getCookie('sidebar_state') !== 'false'
  const [searchOpen, setSearchOpen] = React.useState(false)

  return (
    <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
      <SkipToMain />
      <AppHeader onSearchClick={() => setSearchOpen(true)} />
      <div className='flex min-h-0 w-full flex-1'>
        <AppSidebar />
        <SidebarInset
          id='content'
          className={cn(
            '@container/content',
            'h-[calc(100svh-var(--app-header-height,0px))]',
            'min-h-0 overflow-x-hidden overflow-y-auto',
            'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
          )}
        >
          <Outlet />
        </SidebarInset>
      </div>
      <SearchModal open={searchOpen} onOpenChange={setSearchOpen} />
    </SidebarProvider>
  )
}
