// metapi-go/layout — app-sidebar adapted from newapi. AGPL header stripped.
// Dropped LayoutProvider dependency (not in scope for skeleton); sidebar variant
// and collapsible mode are hardcoded to sensible defaults. View resolution and
// per-view header remain via useSidebarView + sidebar-view-registry.

import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { useSidebarView } from '@/hooks/use-sidebar-view'

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
export function AppSidebar() {
  const { key, view, navGroups } = useSidebarView()

  return (
    <Sidebar collapsible={SIDEBAR_COLLAPSIBLE} variant={SIDEBAR_VARIANT}>
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
