// metapi-go/hooks — use-sidebar-data adapted from newapi per plan.md §5.5.4.
// 4 collapsible groups (Console / Configuration / Models / System) with lucide icons.
// No NavChatPresets (metapi has no chat). No requiredRole (metapi is fully open).
// Titles are i18n keys resolved via t() at render time (nav-group.tsx).

import {
  Activity,
  Boxes,
  CalendarCheck,
  FlaskConical,
  Info,
  LayoutDashboard,
  ScrollText,
  Route,
  Scale,
  Server,
  Settings,
  ShieldCheck,
  Waypoints,
  Wrench,
} from 'lucide-react'

import type { SidebarData } from '@/components/layout/types'

/**
 * Root navigation groups for the metapi sidebar.
 *
 * Shown when the URL does not match any nested sidebar view registered in
 * components/layout/lib/sidebar-view-registry.ts (currently just Settings).
 * Grouped into 4 collapsible sections per the rewrite IA redesign.
 */
export function useSidebarData(): SidebarData {
  return {
    navGroups: [
      {
        id: 'console',
        title: 'sidebar.groups.console',
        items: [
          {
            title: 'sidebar.items.dashboard',
            url: '/',
            icon: LayoutDashboard,
          },
          {
            title: 'sidebar.items.sites',
            url: '/sites',
            icon: Server,
          },
          {
            title: 'sidebar.items.accounts',
            url: '/accounts',
            icon: ShieldCheck,
          },
          {
            title: 'sidebar.items.checkin',
            url: '/checkin',
            icon: CalendarCheck,
          },
          {
            title: 'sidebar.items.proxyLogs',
            url: '/proxy-logs',
            icon: ScrollText,
          },
          {
            title: 'sidebar.items.observability',
            url: '/observability',
            icon: Activity,
          },
        ],
      },
      {
        id: 'config',
        title: 'sidebar.groups.configuration',
        items: [
          {
            title: 'sidebar.items.tokenRoutes',
            url: '/token-routes',
            icon: Route,
          },
          {
            title: 'sidebar.items.channels',
            url: '/channels',
            icon: Waypoints,
          },
          {
            title: 'sidebar.items.oauth',
            url: '/oauth',
            icon: ShieldCheck,
          },
        ],
      },
      {
        id: 'models',
        title: 'sidebar.groups.models',
        items: [
          {
            title: 'sidebar.items.models',
            url: '/models',
            icon: Boxes,
          },
          {
            title: 'sidebar.items.modelTester',
            url: '/model-tester',
            icon: FlaskConical,
          },
          {
            title: 'sidebar.items.priceCompare',
            url: '/price-compare',
            icon: Scale,
          },
          {
            title: 'sidebar.items.fixCandidates',
            url: '/fix-candidates',
            icon: Wrench,
          },
        ],
      },
      {
        id: 'system',
        title: 'sidebar.groups.system',
        items: [
          {
            title: 'sidebar.items.settings',
            url: '/settings',
            icon: Settings,
          },
          {
            title: 'sidebar.items.about',
            url: '/about',
            icon: Info,
          },
        ],
      },
    ],
  }
}
