// metapi-go/layout — app-sidebar adapted from newapi. AGPL header stripped.
// Dropped LayoutProvider dependency (not in scope for skeleton); sidebar variant
// and collapsible mode are hardcoded to sensible defaults. View resolution and
// per-view header remain via useSidebarView + sidebar-view-registry.

import { AnimatePresence, motion, useReducedMotion } from 'motion/react'

import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { MOTION_TRANSITION, MOTION_VARIANTS } from '@/lib/motion'

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
 */
export function AppSidebar() {
  const { key, view, navGroups } = useSidebarView()
  const shouldReduce = useReducedMotion()

  return (
    <Sidebar collapsible={SIDEBAR_COLLAPSIBLE} variant={SIDEBAR_VARIANT}>
      {view && <SidebarViewHeader view={view} />}

      <SidebarContent className='py-2'>
        <AnimatePresence mode='wait' initial={false}>
          <motion.div
            key={key}
            initial={
              shouldReduce ? false : MOTION_VARIANTS.sidebarSlide.initial
            }
            animate={MOTION_VARIANTS.sidebarSlide.animate}
            exit={shouldReduce ? undefined : MOTION_VARIANTS.sidebarSlide.exit}
            transition={MOTION_TRANSITION.fast}
            className='flex flex-col'
          >
            {navGroups.map((props) => (
              <NavGroup key={props.id || props.title} {...props} />
            ))}
          </motion.div>
        </AnimatePresence>
      </SidebarContent>

      <SidebarRail />
    </Sidebar>
  )
}
