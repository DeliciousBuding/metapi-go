// metapi-go/features/settings/lib — dirty-field diffing for the unified
// settings form. `collectChangedFields` compares the current form values
// against the last server baseline and returns only the changed top-level
// fields, so PUT /api/settings/runtime never re-sends untouched config
// (including masked secrets that must not be echoed back).

/** Deep equality for JSON-able form values (objects + arrays). */
export function isDeepEqual(a: unknown, b: unknown): boolean {
  if (Object.is(a, b)) {
    return true
  }
  if (typeof a !== typeof b) {
    return false
  }
  if (a === null || b === null) {
    return false
  }
  if (Array.isArray(a)) {
    if (!Array.isArray(b) || a.length !== b.length) {
      return false
    }
    return a.every((item, index) => isDeepEqual(item, b[index]))
  }
  if (typeof a === 'object' && typeof b === 'object') {
    const aRecord = a as Record<string, unknown>
    const bRecord = b as Record<string, unknown>
    const aKeys = Object.keys(aRecord)
    const bKeys = Object.keys(bRecord)
    if (aKeys.length !== bKeys.length) {
      return false
    }
    return aKeys.every((key) => isDeepEqual(aRecord[key], bRecord[key]))
  }
  return false
}

/**
 * Return the subset of `values` whose top-level fields differ from
 * `baseline`. When there is no baseline (load failed / first render) the
 * whole object is treated as changed so the first save still works.
 */
export function collectChangedFields<T extends Record<string, unknown>>(
  values: T,
  baseline: T | null | undefined
): Partial<T> {
  if (!baseline) {
    return { ...values }
  }
  const changed: Record<string, unknown> = {}
  for (const key of Object.keys(values)) {
    if (!isDeepEqual(values[key], baseline[key])) {
      changed[key] = values[key]
    }
  }
  return changed as Partial<T>
}

/** True when a diff contains at least one field. */
export function hasChanges<T>(changed: Partial<T>): boolean {
  return Object.keys(changed).length > 0
}
