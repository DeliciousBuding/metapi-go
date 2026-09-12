// Behavior test for the downstream-key `expiresAt` wire format.
//
// Regression lock for a silent no-op control: `expires_at` is only enforced
// when the server can parse it. handler/admin/downstream_keys_normalize.go
// stores an unparseable value verbatim, and auth/downstream.go's
// parseISO8601 SKIPS the expiry check on a parse error (TS parity, locked by
// auth/downstream_test.go TestExpiration_InvalidDateFormat). A bare
// datetime-local value ("2020-01-01T00:00") parses nowhere in that chain —
// verified against a live server, where a key that expired in 2020 still
// served /v1/models with HTTP 200 while the RFC3339 form of the same instant
// returned 403 key_expired.
//
// So the mapper must emit RFC3339 (seconds + offset), which both the write
// normalizer and the auth-time parser accept.

import { describe, expect, it } from 'vitest'

import { localDatetimeInputToIso } from '../key-form-shared'

describe('localDatetimeInputToIso', () => {
  it('upgrades a minute-precision datetime-local value to RFC3339 UTC', () => {
    const iso = localDatetimeInputToIso('2026-09-02T15:04')

    // The shape the Go parsers require: seconds present, explicit offset.
    expect(iso).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/)
    // Same instant, so the operator's chosen local wall-clock is preserved.
    expect(new Date(iso).getTime()).toBe(new Date('2026-09-02T15:04').getTime())
  })

  it('keeps an explicit UTC value on the same instant', () => {
    expect(localDatetimeInputToIso('2026-09-02T15:04:05Z')).toBe(
      '2026-09-02T15:04:05.000Z'
    )
  })

  it('passes empty through so "never expires" still clears the column', () => {
    // Create stores ""/NULL and update normalizes "" to NULL; the update path
    // additionally relies on the key being PRESENT in the body to clear an
    // existing expiry, so empty must not be dropped or rewritten.
    expect(localDatetimeInputToIso('')).toBe('')
  })

  it('passes an unparseable value through instead of dropping it', () => {
    // Never silently turn a value we do not understand into "no expiry".
    expect(localDatetimeInputToIso('not-a-date')).toBe('not-a-date')
  })
})
