// metapi-go/features/settings/config — 5-subarea manifest + validation helpers.
// Assembles each subarea's typed section-registry adapter into a single
// string-typed list consumed by route guards + the generic SettingsPage.
//
// Semantic regroup of the settings navigation:
// basic / proxy-models / content(通知与数据) / operations.

import { basicSubarea } from '../sections/basic'
import { contentSubarea } from '../sections/content'
import { operationsSubarea } from '../sections/operations'
import { proxyModelsSubarea } from '../sections/proxy-models'
import type { SettingsSubarea } from '../types'

const SETTINGS_SUBAREAS: readonly SettingsSubarea[] = [
  basicSubarea,
  proxyModelsSubarea,
  contentSubarea,
  operationsSubarea,
]

/**
 * Settings subareas in main-sidebar order.
 * Consumed by the layout drill-in sidebar and the `/settings` entry redirect so
 * both share one metadata source (title / icon / description / default section).
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
