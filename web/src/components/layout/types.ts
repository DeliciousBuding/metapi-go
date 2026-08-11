// metapi-go/layout — navigation types adapted from newapi. AGPL header stripped.
// Dropped NavChatPresets (metapi has no chat) and requiredRole (metapi is fully open;
// route-level guards enforce access). getNavGroups takes no TFunction — i18n lands in
// phase 2, so nav builders return plain strings for now.

import type { LinkProps } from '@tanstack/react-router'

/**
 * Base navigation item type
 */
type BaseNavItem = {
  title: string
  badge?: string
  icon?: React.ElementType
  activeUrls?: (LinkProps['to'] | (string & {}))[]
  configUrls?: (LinkProps['to'] | (string & {}))[]
}

/**
 * Navigation link type - single link item
 */
export type NavLink = BaseNavItem & {
  url: LinkProps['to'] | (string & {})
  items?: never
}

/**
 * Navigation collapsible type - collapsible navigation with sub-items
 */
export type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps['to'] | (string & {}) })[]
  url?: never
}

/**
 * Navigation item union type
 */
export type NavItem = NavCollapsible | NavLink

/**
 * Navigation group type - a group of navigation items in sidebar
 */
export type NavGroup = {
  id?: string
  title: string
  items: NavItem[]
}

/**
 * Root sidebar data type
 *
 * Used by the default (top-level) sidebar view that lists primary
 * application navigation (console / configuration / models / system).
 */
export type SidebarData = {
  navGroups: NavGroup[]
}

/**
 * Back-navigation descriptor for a nested sidebar view
 */
type SidebarViewParent = {
  /** Destination URL for the back button */
  to: LinkProps['to'] | (string & {})
  /** Visible label, e.g. "Back to Home" */
  label: string
}

/**
 * Nested sidebar view configuration
 *
 * A nested view replaces the root navigation when the user enters a
 * dedicated workspace (e.g. Settings). Models the Vercel / Cloudflare
 * "drill-in" sidebar UX: clicking a top-level entry swaps the sidebar
 * to a contextual view with a "Back" affordance.
 */
export type SidebarView = {
  /** Stable identifier (also drives transition animation keys) */
  id: string
  /** Path matcher that activates this view */
  pathPattern: RegExp
  /** Back-navigation descriptor; required for nested views */
  parent: SidebarViewParent
  /** Nav group builder, called per render */
  getNavGroups: () => NavGroup[]
}

/**
 * Resolved sidebar view returned by useSidebarView()
 *
 * - `view === null`: root navigation (default sidebar)
 * - `view !== null`: nested workspace view (renders header + back button)
 */
export type ResolvedSidebarView = {
  /** Animation/identity key — falls back to a sentinel for the root view */
  key: string
  view: SidebarView | null
  navGroups: NavGroup[]
}
