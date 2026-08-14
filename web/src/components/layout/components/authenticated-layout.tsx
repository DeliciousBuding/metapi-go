// metapi-go/layout — authenticated-layout adapted from newapi. AGPL header stripped.
// SidebarProvider + SkipToMain + AppHeader + flex row (AppSidebar + SidebarInset).
// Uses <Outlet /> from TanStack Router so matched child routes (/dashboard/*, /sites,
// /accounts, /settings/*, ...) render inside SidebarInset. Also owns the global
// search palette state and mounts SearchModal here (inside the router, since
// result clicks navigate).

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
      <div className='flex min-h-0 w-full flex-1 [--app-header-height:3.5rem]'>
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
