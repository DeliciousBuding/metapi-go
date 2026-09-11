// metapi-go/layout — the navigation model: what a sidebar entry can be, and how
// a URL picks one.
//
// Two ideas carry the whole shell. A `NavItem` is either a leaf (`url`) or a
// branch (`items`), made mutually exclusive with `?: never` so a malformed
// entry is a type error rather than a runtime surprise. A `SidebarView` is a
// whole navigation set bound to a path pattern, which is what lets a workspace
// like Settings replace the root sidebar instead of nesting inside it.
//
// Every `title` / `badge` / `label` string in this model is an **i18n key**,
// resolved with `t()` at render time by `nav-group.tsx` and
// `sidebar-view-header.tsx` — the builders here stay pure data so they can be
// constructed outside React.

import type { LinkProps } from '@tanstack/react-router'

/** Fields every navigation entry carries, leaf or branch. */
type BaseNavItem = {
  title: string
  /** Optional i18n key of a small badge rendered beside the title (resolved via t()). */
  badge?: string
  icon?: React.ElementType
  activeUrls?: (LinkProps['to'] | (string & {}))[]
  configUrls?: (LinkProps['to'] | (string & {}))[]
  /** When set, the item stays active for any URL under this path prefix. */
  activePrefix?: string
  /** TanStack Link active matching (e.g. `{ exact: true }` for workspace roots). */
  activeOptions?: LinkProps['activeOptions']
}

/** A leaf entry: navigates somewhere, has no children. */
export type NavLink = BaseNavItem & {
  url: LinkProps['to'] | (string & {})
  items?: never
}

/** A branch entry: expands to sub-items, navigates nowhere itself. */
export type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps['to'] | (string & {}) })[]
  url?: never
}

export type NavItem = NavCollapsible | NavLink

/** A labelled section of the sidebar. */
export type NavGroup = {
  id?: string
  title: string
  items: NavItem[]
}

/** The root navigation set: console / configuration / models / system. */
export type SidebarData = {
  navGroups: NavGroup[]
}

/** Where a nested view's back affordance goes, and what it says. */
type SidebarViewParent = {
  to: LinkProps['to'] | (string & {})
  /** i18n key, e.g. `nav.backToHome`. */
  label: string
}

/**
 * A drill-in workspace that replaces the root navigation while its path
 * pattern matches — the Vercel / Cloudflare sidebar pattern: entering Settings
 * swaps the whole sidebar for a contextual one with a back affordance, rather
 * than nesting a second tree inside the first.
 */
export type SidebarView = {
  /** Stable identifier; also the animation key when the sidebar swaps views. */
  id: string
  /** Activates this view when it matches the current pathname. */
  pathPattern: RegExp
  parent: SidebarViewParent
  /** Called per render, so a view can reflect live manifests. */
  getNavGroups: () => NavGroup[]
}

/**
 * What `useSidebarView()` resolves for the current location.
 *
 * `view === null` means the root navigation and no header; a non-null `view`
 * means a drill-in workspace, which renders its back affordance above
 * `navGroups`.
 */
export type ResolvedSidebarView = {
  /** Animation / identity key; a sentinel for the root view. */
  key: string
  view: SidebarView | null
  navGroups: NavGroup[]
}
