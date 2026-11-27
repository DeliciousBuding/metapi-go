import type { SoftTone } from '@/components/common/soft-tone-badge'

import type { PersonalKey } from './api'

export type PersonalKeyStatus =
  | 'available'
  | 'disabled'
  | 'expired'
  | 'limitReached'

export function personalKeyStatus(
  key: PersonalKey,
  now = Date.now()
): PersonalKeyStatus {
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

export function personalKeyStatusTone(status: PersonalKeyStatus): SoftTone {
  switch (status) {
    case 'available':
      return 'success'
    case 'expired':
    case 'limitReached':
      return 'warning'
    case 'disabled':
      return 'neutral'
  }
}
