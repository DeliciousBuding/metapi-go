// Persist only the task reference, never a locally guessed status or result.
// Same-tab subscribers also see launches from the import flow or route form.
export const ROUTE_REBUILD_STORAGE_KEY = 'metapi-go:token-routes:rebuild-task'

export type RouteRebuildReference = { taskId: string; refreshModels: boolean }

export const routeRebuildTaskQueryKey = (taskId: string | null) =>
  ['route-rebuild-task', taskId] as const

const listeners = new Set<() => void>()
let memorySnapshot: string | null | undefined

export function readRouteRebuildSnapshot(): string | null {
  if (memorySnapshot !== undefined) return memorySnapshot
  try {
    return window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)
  } catch {
    return null
  }
}

export function parseRouteRebuildReference(
  snapshot: string | null
): RouteRebuildReference | null {
  if (!snapshot) return null
  try {
    const value: unknown = JSON.parse(snapshot)
    if (!value || typeof value !== 'object') return null
    const reference = value as Partial<RouteRebuildReference>
    return typeof reference.taskId === 'string' &&
      reference.taskId.trim() &&
      typeof reference.refreshModels === 'boolean'
      ? { taskId: reference.taskId, refreshModels: reference.refreshModels }
      : null
  } catch {
    return null
  }
}

export function rememberRouteRebuild(
  reference: RouteRebuildReference | null
): void {
  const snapshot = reference ? JSON.stringify(reference) : null
  try {
    if (snapshot === null) {
      window.localStorage.removeItem(ROUTE_REBUILD_STORAGE_KEY)
    } else window.localStorage.setItem(ROUTE_REBUILD_STORAGE_KEY, snapshot)
    memorySnapshot = undefined
  } catch {
    // Match the other local preferences: storage failure must not break the
    // current page's in-memory task tracking.
    memorySnapshot = snapshot
  }
  for (const listener of listeners) listener()
}

export function subscribeRouteRebuild(listener: () => void): () => void {
  listeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key !== null && event.key !== ROUTE_REBUILD_STORAGE_KEY) return
    memorySnapshot = undefined
    listener()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(listener)
    window.removeEventListener('storage', onStorage)
  }
}
