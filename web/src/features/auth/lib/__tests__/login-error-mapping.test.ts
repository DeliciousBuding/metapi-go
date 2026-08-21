// metapi-go/features/auth — login error classification tests.
// Guards the message mapping used by the sign-in form, including the
// network-unreachable case: axios errors without a response arrive with
// status 0 and must surface the dedicated "unable to connect" copy instead
// of the generic failure message.

import { describe, expect, it } from 'vitest'

import { resolveLoginErrorMessageKey } from '../../api'

describe('resolveLoginErrorMessageKey', () => {
  it('maps network errors (no response) to serverUnreachable', () => {
    expect(resolveLoginErrorMessageKey(0, '')).toBe(
      'errors.login.serverUnreachable'
    )
  })

  it('maps 401 to invalidToken', () => {
    expect(resolveLoginErrorMessageKey(401, '')).toBe(
      'errors.login.invalidToken'
    )
  })

  it('maps 403 invalid-token bodies to invalidToken', () => {
    expect(resolveLoginErrorMessageKey(403, 'Invalid token')).toBe(
      'errors.login.invalidToken'
    )
  })

  it('maps 403 IP-allowlist bodies to ipNotAllowed, not invalidToken', () => {
    expect(resolveLoginErrorMessageKey(403, 'IP not allowed')).toBe(
      'errors.login.ipNotAllowed'
    )
  })

  it('maps 5xx to serverError', () => {
    expect(resolveLoginErrorMessageKey(500, '')).toBe(
      'errors.login.serverError'
    )
    expect(resolveLoginErrorMessageKey(503, '')).toBe(
      'errors.login.serverError'
    )
  })

  it('falls back to the generic failure key for anything else', () => {
    expect(resolveLoginErrorMessageKey(403, '')).toBe('errors.login.failed')
    expect(resolveLoginErrorMessageKey(400, 'bad request')).toBe(
      'errors.login.failed'
    )
  })
})
