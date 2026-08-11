// metapi-go/layout — sidebar-view-header ported from newapi. AGPL header stripped.
// Renders the back affordance for a nested drill-in sidebar view.
// Labels are i18n keys resolved via t() at render time.

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
import { cn } from '@/lib/utils'

import type { SidebarView } from '../types'

type SidebarViewHeaderProps = {
  view: SidebarView
}

/**
 * Header for a nested sidebar view (Vercel / Cloudflare drill-in pattern).
 *
 * Renders only the back affordance — workspace context is conveyed by
 * the nav groups below, not a redundant title row.
 */
export function SidebarViewHeader(props: SidebarViewHeaderProps) {
  const { setOpenMobile } = useSidebar()
  const { t } = useTranslation()

  return (
    <SidebarHeader className='border-sidebar-border border-b px-2 py-2'>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            tooltip={t(props.view.parent.label)}
            className={cn(
              'text-muted-foreground hover:text-foreground',
              'gap-1.5 font-medium'
            )}
            render={
              <Link
                to={props.view.parent.to}
                onClick={() => setOpenMobile(false)}
              />
            }
          >
            <ChevronLeft className='size-4 shrink-0' />
            <span className='truncate'>{t(props.view.parent.label)}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
  )
}
