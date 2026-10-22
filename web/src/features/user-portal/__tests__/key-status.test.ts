import { expect, it } from 'vitest'

import type { PersonalKey } from '../api'
import { personalKeyStatus } from '../key-status'

const key: PersonalKey = {
  id: 1,
  name: 'Fixture',
  enabled: true,
  keyMasked: 'fixture',
  expiresAt: null,
  supportedModels: [],
  maxCost: null,
  maxRequests: null,
  usedCost: 0,
  usedRequests: 0,
  createdAt: '',
}
it('shares exact status precedence between key list and onboarding', () => {
  expect(personalKeyStatus(key)).toBe('available')
  expect(personalKeyStatus({ ...key, enabled: false })).toBe('disabled')
  expect(personalKeyStatus({ ...key, expiresAt: 'invalid' })).toBe('expired')
  expect(personalKeyStatus({ ...key, expiresAt: '2000-01-01' })).toBe('expired')
  expect(personalKeyStatus({ ...key, maxRequests: 1, usedRequests: 1 })).toBe(
    'limitReached'
  )
  expect(personalKeyStatus({ ...key, maxCost: 0 })).toBe('limitReached')
})
