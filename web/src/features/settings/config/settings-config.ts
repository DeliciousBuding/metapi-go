// metapi-go/features/settings/config — 5-subarea manifest + validation helpers.
// Assembles each subarea's typed section-registry adapter into a single
// string-typed list consumed by route guards + the generic SettingsPage.
//
// Wave 9 lane B semantic regroup (settings-ia-plan.md §3.2 方案 A):
// basic / proxy-models / downstream / content(通知与数据) / operations.

import { basicSubarea } from '../sections/basic'
import { contentSubarea } from '../sections/content'
import { downstreamSubarea } from '../sections/downstream'
import { operationsSubarea } from '../sections/operations'
import { proxyModelsSubarea } from '../sections/proxy-models'
import type { SettingsSubarea } from '../types'

const SETTINGS_SUBAREAS: readonly SettingsSubarea[] = [
  basicSubarea,
  proxyModelsSubarea,
  downstreamSubarea,
  contentSubarea,
  operationsSubarea,
]

/** Stable id list for route validation. */

/**
 * All 5 settings subareas in main-sidebar order
 * (matches components/layout/config/system-settings.config.ts).
 */

/** Stable id list for route validation. */

/**
 * All 5 settings subareas in main-sidebar order.
 * Consumed by the settings overview landing and the layout drill-in sidebar so
 * both surfaces share one metadata source (title / icon / description).
 */
export function getSettingsSubareas(): readonly SettingsSubarea[] {
  return SETTINGS_SUBAREAS
}

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
