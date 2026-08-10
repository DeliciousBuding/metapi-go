// metapi-go/hooks — use-sidebar-data adapted from newapi per plan.md §5.5.4.
// 4 collapsible groups (Console / Configuration / Models / System) with lucide icons.
// No NavChatPresets (metapi has no chat). No requiredRole (metapi is fully open).
// TODO(phase 2): wrap titles in useTranslation once i18n is wired.

import {
  Activity,
  Boxes,
  CalendarCheck,
  FlaskConical,
  Info,
  LayoutDashboard,
  Megaphone,
  ScrollText,
  Route,
  Server,
  Settings,
  ShieldCheck,
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
        title: 'Console',
        items: [
          {
            title: 'Dashboard',
            url: '/',
            icon: LayoutDashboard,
          },
          {
            title: 'Sites',
            url: '/sites',
            icon: Server,
          },
          {
            title: 'Accounts',
            url: '/accounts',
            icon: ShieldCheck,
          },
          {
            title: 'Checkin',
            url: '/checkin',
            icon: CalendarCheck,
          },
          {
            title: 'Proxy Logs',
            url: '/proxy-logs',
            icon: ScrollText,
          },
          {
            title: 'Monitors',
            url: '/monitors',
            icon: Activity,
          },
        ],
      },
      {
        id: 'config',
        title: 'Configuration',
        items: [
          {
            title: 'Token Routes',
            url: '/token-routes',
            icon: Route,
          },
          {
            title: 'OAuth',
            url: '/oauth',
            icon: ShieldCheck,
          },
          {
            title: 'Site Announcements',
            url: '/site-announcements',
            icon: Megaphone,
          },
        ],
      },
      {
        id: 'models',
        title: 'Models',
        items: [
          {
            title: 'Models',
            url: '/models',
            icon: Boxes,
          },
          {
            title: 'Model Tester',
            url: '/playground',
            icon: FlaskConical,
          },
        ],
      },
      {
        id: 'system',
        title: 'System',
        items: [
          {
            title: 'Settings',
            url: '/settings',
            icon: Settings,
          },
          {
            title: 'About',
            url: '/about',
            icon: Info,
          },
        ],
      },
    ],
  }
}
