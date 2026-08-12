// metapi-go/features/settings/config — 5-subarea manifest + validation helpers.
// Assembles each subarea's typed section-registry adapter into a single
// string-typed list consumed by route guards + the generic SettingsPage.

import { contentSubarea } from '../sections/content'
import { downstreamSubarea } from '../sections/downstream'
import { generalSubarea } from '../sections/general'
import { modelsSubarea } from '../sections/models'
import { systemInfoSubarea } from '../sections/system-info'
import type { SettingsSubarea } from '../types'

const SETTINGS_SUBAREAS: readonly SettingsSubarea[] = [
  generalSubarea,
  downstreamSubarea,
  modelsSubarea,
  contentSubarea,
  systemInfoSubarea,
]

/** Stable id list for route validation. */

/**
 * All 5 settings subareas in main-sidebar order
 * (matches components/layout/config/system-settings.config.ts).
 */

/** Stable id list for route validation. */

/**
 * Look up a subarea by id (e.g. from the `$subarea` route param).
 * @returns The matching SettingsSubarea, or `undefined` if the id is unknown.
 */
export function getSettingsSubarea(
  subareaId: string
): SettingsSubarea | undefined {
  return SETTINGS_SUBAREAS.find((subarea) => subarea.id === subareaId)
}

/**
 * Resolve the default section id for a subarea (used by index redirects when
 * no `$section` param is present).
 */
export function resolveDefaultSection(subareaId: string): string | undefined {
  return getSettingsSubarea(subareaId)?.defaultSection
}

/**
 * Validate that a (subarea, section) pair is routable. Route `beforeLoad`
 * guards call this and redirect to the default section on mismatch.
 */
export function isValidSection(subareaId: string, sectionId: string): boolean {
  const subarea = getSettingsSubarea(subareaId)
  return subarea ? subarea.sectionIds.includes(sectionId) : false
}
