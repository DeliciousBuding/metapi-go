// metapi-go/layout — the root navigation set: what the sidebar shows when the
// URL matches no drill-in view.
//
// Four collapsible groups (Console / Configuration / Models / System). Every
// `title` is an i18n key resolved with t() by `nav-group.tsx`, so this module
// stays pure data and can be read outside React — the command palette does
// exactly that to index navigation alongside its other sources.
//
// Static by design: it lives beside `system-settings.config.ts` (the Settings
// view's equivalent) rather than in `hooks/`, because there is nothing to
// observe. Both callers get the same object reference, so identity comparisons
// downstream stay cheap.

import {
  Activity,
  Boxes,
  CalendarCheck,
  FlaskConical,
  Info,
  KeyRound,
  Megaphone,
  LayoutDashboard,
  ScrollText,
  Route,
  Scale,
  Server,
  Settings,
  ShieldCheck,
  Waypoints,
} from 'lucide-react'

import type { SidebarData } from '@/components/layout/types'

export const ROOT_NAVIGATION: SidebarData = {
  navGroups: [
    {
      id: 'console',
      title: 'sidebar.groups.console',
      items: [
        {
          title: 'sidebar.items.dashboard',
          url: '/',
          // `/` redirects to `/dashboard/overview`, so the dashboard item
          // stays highlighted for every section under /dashboard/*.
          activePrefix: '/dashboard',
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
          title: 'sidebar.items.siteAnnouncements',
          url: '/site-announcements',
          icon: Megaphone,
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
          title: 'sidebar.items.downstreamKeys',
          url: '/downstream-keys',
          icon: KeyRound,
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
