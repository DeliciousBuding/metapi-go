// metapi-go/layout — app-sidebar adapted from newapi. AGPL header stripped.
// Dropped LayoutProvider dependency (not in scope for skeleton); sidebar variant
// and collapsible mode are hardcoded to sensible defaults. View resolution and
// per-view header remain via useSidebarView + sidebar-view-registry.

import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

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

// Skeleton defaults — newapi reads these from LayoutProvider (cookie-backed).
// metapi will wire a layout provider in a later phase; for now these match
// the newapi defaults (variant=inset, collapsible=icon).
const SIDEBAR_VARIANT = 'inset' as const
const SIDEBAR_COLLAPSIBLE = 'icon' as const

/**
 * Application sidebar.
 *
 * Adopts the Vercel / Cloudflare "drill-in" pattern: the URL drives
 * which sidebar *view* is rendered. Clicking a top-level entry like
 * `Settings` swaps the sidebar to a contextual workspace — with a
 * `← Back to Home` affordance — instead of stacking the sub-navigation
 * inside the root tree.
 *
 * Architecture:
 *   - View resolution: {@link useSidebarView}
 *   - View registry: layout/lib/sidebar-view-registry.ts
 *   - Per-view header: {@link SidebarViewHeader}
 *
 * Adding a new nested view only requires registering a SidebarView
 * in the registry; this component requires no changes.
 *
 * Animation: the view swap uses the `.sidebar-view-enter` CSS keyframe
 * (defined in styles/index.css) instead of `motion`/`framer-motion`. The
 * `key={key}` prop forces React to remount the container on every view
 * change, which re-triggers the CSS enter animation. This removed the
 * `motion` package (~78 kB gz) from the eager authenticated chunk; the
 * animation only fires on view switches (never on initial page render —
 * matching the previous `AnimatePresence initial={false}`), and
 * `prefers-reduced-motion` users see no animation.
 */
/**
 * Mobile-only drawer header: brand row + an explicit close button.
 *
 * The sheet template's own close button is hidden on the mobile sidebar
 * (`[&>button]:hidden` in ui/sidebar.tsx) and the drawer previously opened
 * straight into the first group label with no way to tell where the close
 * affordance was. This header lives inside a wrapper div, so the `> button`
 * selector never hits it. It is rendered only on mobile (desktop keeps its
 * icon-collapsed rail + trigger).
 */
function SidebarMobileHeader() {
  const { setOpenMobile } = useSidebar()
  const { t } = useTranslation()
  return (
    <SidebarHeader className='flex-row items-center justify-between border-b p-2.5'>
      <div className='flex min-w-0 items-center gap-2'>
        <img
          src={metapiIdentity.logoPath}
          alt={metapiIdentity.name}
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

export function AppSidebar() {
  const { key, view, navGroups } = useSidebarView()
  const { isMobile } = useSidebar()

  return (
    <Sidebar collapsible={SIDEBAR_COLLAPSIBLE} variant={SIDEBAR_VARIANT}>
      {isMobile && <SidebarMobileHeader />}
      {view && <SidebarViewHeader view={view} />}

      <SidebarContent className='py-2'>
        {/* `key` remounts the subtree on every view switch so the CSS
         * enter animation re-runs. Plain <div> replaces the old
         * <AnimatePresence><motion.div> pair. */}
        <div key={key} className='sidebar-view-enter flex flex-col'>
          {navGroups.map((props) => (
            <NavGroup key={props.id || props.title} {...props} />
          ))}
        </div>
      </SidebarContent>

      <SidebarRail />
    </Sidebar>
  )
}
