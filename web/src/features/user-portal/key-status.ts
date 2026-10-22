import type { PersonalKey } from './api'

export function personalKeyStatus(
  key: PersonalKey,
  now = Date.now()
): 'available' | 'disabled' | 'expired' | 'limitReached' {
  if (!key.enabled) return 'disabled'
  if (key.expiresAt) {
    const expiry = Date.parse(key.expiresAt)
    if (!Number.isFinite(expiry) || expiry <= now) return 'expired'
  }
  if (
    (key.maxRequests !== null && key.usedRequests >= key.maxRequests) ||
    (key.maxCost !== null && key.usedCost >= key.maxCost)
  )
    return 'limitReached'
  return 'available'
}
