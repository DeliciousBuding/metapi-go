// metapi-go/layout — barrel re-exports for the layout layer.

export { AuthenticatedLayout } from './components/authenticated-layout'
export { AppSidebar } from './components/app-sidebar'
export { AppHeader } from './components/app-header'
export { NavGroup } from './components/nav-group'
export { SidebarViewHeader } from './components/sidebar-view-header'

export { resolveSidebarView, getNavGroupsForPath } from './lib/sidebar-view-registry'
export { checkIsActive, normalizeHref } from './lib/url-utils'

export { SYSTEM_SETTINGS_VIEW } from './config/system-settings.config'

export type {
  NavItem,
  NavLink,
  NavCollapsible,
  SidebarData,
  SidebarView,
  SidebarViewParent,
  ResolvedSidebarView,
} from './types'
