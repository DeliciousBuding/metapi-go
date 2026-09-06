import { describe, expect, it } from 'vitest'

import {
  getAccountTokenFormDefaultValues,
  getAccountTokenFormSchema,
  transformTokenFormToPayload,
} from '../lib/tokens-schema'

const values = () => ({
  ...getAccountTokenFormDefaultValues(7),
  name: 'relay token',
})

describe('account token form wire contract', () => {
  it('requires a quota before issuing a finite upstream token', () => {
    const result = getAccountTokenFormSchema().safeParse({
      ...values(),
      unlimited: false,
    })
    expect(result.success).toBe(false)
    if (!result.success) expect(result.error.issues[0].path).toEqual(['quota'])
  })

  it('accepts an empty value for upstream creation and keep-on-edit', () => {
    expect(getAccountTokenFormSchema().safeParse(values()).success).toBe(true)
    expect(transformTokenFormToPayload(values())).not.toHaveProperty('token')
  })

  it('uses the API field names and preserves an explicit finite quota', () => {
    const payload = transformTokenFormToPayload({
      ...values(),
      tokenGroup: 'limited',
      unlimited: false,
      quota: 25000,
      expiresAt: '2030-01-01T00:00:00Z',
      allowedIps: '127.0.0.1, ::1  192.0.2.10',
    })
    expect(payload).toEqual({
      accountId: 7,
      name: 'relay token',
      group: 'limited',
      unlimitedQuota: false,
      remainQuota: 25000,
      expiredTime: 1893456000,
      allowIps: '127.0.0.1,::1,192.0.2.10',
    })
    for (const ignored of ['tokenGroup', 'quota', 'allowedIps']) {
      expect(payload).not.toHaveProperty(ignored)
    }
  })

  it('does not pretend a local import configures upstream restrictions', () => {
    expect(
      transformTokenFormToPayload({
        ...values(),
        token: '  test-relay-value  ',
        unlimited: false,
        quota: 10,
        allowedIps: '127.0.0.1',
      })
    ).toEqual({
      accountId: 7,
      name: 'relay token',
      token: 'test-relay-value',
      group: 'default',
    })
  })
})
