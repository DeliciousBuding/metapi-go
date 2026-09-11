// metapi-go/layout — SidebarViewHeader: the back affordance of a drill-in
// sidebar view.
//
// Deliberately just a back link, with no title row: the workspace is already
// named by the group labels underneath it, so a second copy of the name is noise.
// The link also dismisses the mobile drawer, because on a phone the sidebar *is*
// an overlay — navigating without closing it would leave the new page covered.

import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'

import type { SidebarView } from '../types'

export function SidebarViewHeader({ view }: { view: SidebarView }) {
  const { setOpenMobile } = useSidebar()
  const { t } = useTranslation()
  // Resolved once: the same label is the visible text and the collapsed-rail
  // tooltip, and the two must not drift.
  const parentLabel = t(view.parent.label)

  return (
    <SidebarHeader className='border-sidebar-border border-b px-2 py-2'>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            tooltip={parentLabel}
            className='text-muted-foreground hover:text-foreground gap-1.5 font-medium'
            render={
              <Link to={view.parent.to} onClick={() => setOpenMobile(false)} />
            }
          >
            <ChevronLeft className='size-4 shrink-0' />
            <span className='truncate'>{parentLabel}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
  )
}
