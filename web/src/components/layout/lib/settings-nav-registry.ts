// metapi-go/layout — settings nav registry (S5 boundary inversion).
//
// The Settings workspace (features/settings) owns its 5-subarea manifest;
// the shell only needs a declarative projection of it (titles / icons /
// paths / section nav items) to build the drill-in sidebar and the ⌘K
// palette. The shell must not import the feature (components ↛ features,
// see docs/internal/web-package-boundaries.md), so this module owns a port:
// the feature's `getSettingsSubareas` is registered through the
// authenticated route's composition root (routes/_authenticated/route.tsx)
// before first render.

import type { LinkProps } from '@tanstack/react-router'
import type { ElementType } from 'react'

/** Nav projection of a settings section (sidebar / ⌘K palette surface). */
type SettingsSectionNavRef = {
  title: string
  url: LinkProps['to'] | (string & {})
  readonly?: boolean
}

/** Nav projection of a settings subarea (sidebar / ⌘K palette surface). */
export type SettingsSubareaNavRef = {
  id: string
  title: string
  icon?: ElementType
  basePath: string
  defaultSection: string
  getSectionNavItems: () => readonly SettingsSectionNavRef[]
}

type SettingsNavProvider = () => readonly SettingsSubareaNavRef[]

let provider: SettingsNavProvider | null = null

/**
 * Register the settings nav provider. Called once from the authenticated
 * route's composition root with the feature's `getSettingsSubareas`.
 */
export function registerSettingsNavProvider(next: SettingsNavProvider): void {
  provider = next
}

/**
 * All 5 settings subareas as nav metadata.
 *
 * Throws when unregistered: the composition-root wiring is mandatory for
 * any surface that renders settings navigation, and a silent empty list
 * would render a broken sidebar/palette instead of a loud failure.
 */
export function getSettingsSubareas(): readonly SettingsSubareaNavRef[] {
  if (provider === null) {
    throw new Error(
      'layout settings nav is unregistered: call registerSettingsNavProvider() from the authenticated route composition root'
    )
  }
  return provider()
}
