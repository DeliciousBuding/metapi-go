// metapi-go/layout — AppSidebar: the navigation shell, and the mount point for
// the drill-in view swap.

import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { metapiIdentity } from '@/lib/identity-branding'

import { NavGroup } from './nav-group'
import { SidebarViewHeader } from './sidebar-view-header'

// Fixed chrome, not user-configurable: `inset` gives the content panel its own
// card inside the sidebar frame, and `icon` collapses the rail to icons rather
// than hiding it, so the entries stay one click away on a narrow desktop.
const SIDEBAR_VARIANT = 'inset' as const
const SIDEBAR_COLLAPSIBLE = 'icon' as const

/**
 * Mobile-only drawer header: brand row plus an explicit close button.
 *
 * The sheet template's own close button is hidden on the mobile sidebar
 * (`[&>button]:hidden` in ui/sidebar.tsx), so without this the drawer would
 * open straight into a group label with no visible way out. It sits inside a
 * wrapper div so that `> button` selector cannot reach it, and renders only on
 * mobile — desktop keeps the icon-collapsed rail and its trigger.
 */
function SidebarMobileHeader() {
  const { setOpenMobile } = useSidebar()
  const { t } = useTranslation()
  return (
    <SidebarHeader className='flex-row items-center justify-between border-b p-2.5'>
      <div className='flex min-w-0 items-center gap-2'>
        <img
          src={metapiIdentity.logoPath}
          alt=''
          className='size-6 shrink-0 rounded-sm'
        />
        <span className='truncate text-sm font-semibold tracking-tight'>
          {metapiIdentity.name}
        </span>
      </div>
      <Button
        variant='ghost'
        size='icon-sm'
        aria-label={t('common.close')}
        onClick={() => setOpenMobile(false)}
      >
        <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
      </Button>
    </SidebarHeader>
  )
}

/**
 * The application sidebar.
 *
 * The URL decides which *view* is shown (Vercel / Cloudflare drill-in): entering
 * Settings swaps the whole sidebar for a contextual workspace with a back
 * affordance, rather than stacking a second tree inside the root one. Three
 * pieces own that, and this component only composes them —
 * {@link useSidebarView} resolves the view, `layout/lib/sidebar-view-registry`
 * holds the registrations, {@link SidebarViewHeader} renders the back row. A new
 * workspace is therefore a registry entry, not a change here.
 *
 * The swap animates with the `.sidebar-view-enter` keyframe (styles/index.css)
 * rather than a motion library: `key={key}` remounts the container, which
 * re-triggers the enter animation on a view change only — never on first render —
 * and `prefers-reduced-motion` suppresses it.
 */
export function AppSidebar() {
  const { key, view, navGroups } = useSidebarView()
  const { isMobile } = useSidebar()

  return (
    <Sidebar collapsible={SIDEBAR_COLLAPSIBLE} variant={SIDEBAR_VARIANT}>
      {isMobile && <SidebarMobileHeader />}
      {view && <SidebarViewHeader view={view} />}

      <SidebarContent className='py-2'>
        {/* `key` remounts the subtree on a view switch so the CSS enter
         * animation re-runs. */}
        <div key={key} className='sidebar-view-enter flex flex-col'>
          {navGroups.map((props) => (
            <NavGroup key={props.id || props.title} {...props} />
          ))}
        </div>
      </SidebarContent>

      {/* Mobile-only footer: the interface controls (language, palette,
          light/dark) live here so the top bar keeps five reachable targets
          instead of seven cramped ones. */}
      {isMobile && (
        <div className='mt-auto flex justify-center border-t p-2.5'>
          <InterfaceControls />
        </div>
      )}

      <SidebarRail />
    </Sidebar>
  )
}
