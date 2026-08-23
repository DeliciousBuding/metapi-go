// metapi-go/features/settings — shared types for the 5-subarea drill-in
// Settings workspace (plan.md §5.5.2). Settings is split into general /
// downstream / models / content / system-info; each subarea owns a
// section-registry that drives both the in-page settings sidebar and the
// content dispatcher (SettingsPage).

import type { LinkProps } from '@tanstack/react-router'
import type { ElementType, ReactNode } from 'react'

/**
 * A single settings section — the leaf unit of the Settings workspace.
 *
 * Each section is a lazy builder (`build`) returning its content ReactNode.
 * Phase 2 stubs return a `StubSection`; phase 3 will swap in real forms wired
 * to the runtime-settings API (`GET/PUT /api/settings/runtime`).
 */
export type SettingsSection = {
  /** Stable id used in the URL (`/settings/<subarea>/<id>`). */
  id: string
  /** Human label shown in the settings sidebar + page header. */
  title: string
  /** Short description shown under the page header (optional). */
  description?: string
  /** Optional lucide icon for the sidebar (phase 2 may leave unset). */
  icon?: ElementType
  /** Read-only / external surface (rates, audit, update center) - shown as a badge. */
  readonly?: boolean
  /** Lazy content builder. Receives nothing in phase 2; phase 3 adds settings. */
  build: () => ReactNode
}

/**
 * Nav item produced by a section registry for the settings sidebar.
 *
 * `url` is typed to accept TanStack route-path literals as well as plain
 * dynamic strings (the `(string & {})` escape hatch mirrors the layout
 * NavItem convention in components/layout/types.ts).
 */
export type SettingsSectionNavItem = {
  title: string
  url: LinkProps['to'] | (string & {})
  /** Read-only / external surface - shown as a badge. */
  readonly?: boolean
}

/**
 * The 5 settings subareas (drill-in workspaces). Each maps to a top-level
 * entry in the main sidebar's Settings nested view
 * (components/layout/config/system-settings.config.ts).
 *
 * Wave 9 lane B semantic regroup (settings-ia-plan.md §3.2 方案 A):
 *   basic        基础            — site, authentication
 *   proxy-models 代理与模型      — proxy-transport, routing, redirects,
 *                                 rates, allowlist, catalog-sources
 *   downstream   下游            — keys, proxy-token
 *   content      通知与数据      — notifications, announcements,
 *                                 import-export
 *   operations   系统与运维      — scheduling, database, data-migration,
 *                                 maintenance, program-logs, audit-logs,
 *                                 update-center, danger-zone
 * Old URLs (`/settings/general/*`, `/settings/models/*`,
 * `/settings/system-info/*`) are redirected by the legacy route map
 * (lib/legacy-redirects.ts).
 */
type SettingsSubareaId =
  | 'basic'
  | 'proxy-models'
  | 'downstream'
  | 'content'
  | 'operations'

/**
 * A fully-assembled subarea — the string-typed surface consumed by the
 * generic SettingsPage dispatcher and the settings-config manifest.
 *
 * Built by each subarea's section-registry as an adapter over its typed
 * `SectionRegistry<TSectionId>`. The cast from `string` to the subarea's
 * `TSectionId` is safe because the registry falls back to `sections[0]`
 * on unknown ids.
 */
export type SettingsSubarea = {
  id: SettingsSubareaId
  title: string
  /** Short description shown on the settings overview landing (optional). */
  description?: string
  /** Optional lucide icon shown on the settings overview + main sidebar. */
  icon?: ElementType
  /** Base path, e.g. '/settings/general'. Section URLs become `${basePath}/${id}`. */
  basePath: string
  /** Section navigated to when no `$section` param is present. */
  defaultSection: string
  /** All valid section ids for this subarea (used by route guards). */
  sectionIds: readonly string[]
  getSectionNavItems: () => SettingsSectionNavItem[]
  getSectionContent: (sectionId: string) => ReactNode
  getSectionMeta: (sectionId: string) => SettingsSection
}
