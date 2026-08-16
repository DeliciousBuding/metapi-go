// metapi-go/lib/helpers — regression gate for the URL-entry crash class:
// router-parsed search values (null literals, duplicate-param arrays,
// numeric/boolean primitives) must never make a route's `validateSearch`
// throw on URL entry. Every schema/helper touched by the entry-crash sweep
// is exercised here so this class of crash cannot silently return.

import { describe, expect, it } from 'vitest'
import { z } from 'zod'

import {
  buildInitialCheckinLogsQuery,
  checkinSearchSchema,
} from '@/features/checkin/lib/checkin-schema'
import { proxyLogsSearchSchema } from '@/features/proxy-logs/lib/proxy-logs-schema'
import { routesSearchSchema } from '@/features/token-routes/lib/routes-schema'
import { hasValidAuthSessionSafe } from '@/lib/auth-session'
import {
  asStringParam,
  stringSearchParam,
  tableSortingItemSchema,
} from '@/lib/helpers/searchParams'
import { signInSearchSchema } from '@/routes/sign-in'

// ---------------------------------------------------------------------------
// stringSearchParam — the shared tolerant string param
// ---------------------------------------------------------------------------

describe('stringSearchParam', () => {
  it('degrades null literals and duplicate-param arrays to undefined instead of throwing', () => {
    expect(stringSearchParam.parse(null)).toBeUndefined()
    expect(stringSearchParam.parse(['a', 'b'])).toBeUndefined()
    expect(stringSearchParam.parse([])).toBeUndefined()
  })

  it('preserves router-parsed string / number / boolean primitives', () => {
    expect(stringSearchParam.parse('gpt')).toBe('gpt')
    expect(stringSearchParam.parse(123)).toBe(123)
    expect(stringSearchParam.parse(true)).toBe(true)
  })

  it('defaults an absent object key to undefined', () => {
    const schema = z.object({ q: stringSearchParam })
    expect(schema.parse({}).q).toBeUndefined()
    expect(schema.parse({ q: undefined }).q).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// checkinSearchSchema
// ---------------------------------------------------------------------------

describe('checkinSearchSchema', () => {
  it('parses adversarial router values without throwing', () => {
    const result = checkinSearchSchema.parse({
      q: null,
      status: true,
      reason: ['a', 'b'],
      site: 123,
      page: 0,
      pageSize: 9999,
    })
    expect(result.q).toBeUndefined()
    expect(result.status).toBe(true)
    expect(result.reason).toBeUndefined()
    expect(result.site).toBe(123)
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
  })

  it('coerces slipped-through primitives to strings in the loader payload', () => {
    const query = buildInitialCheckinLogsQuery(
      checkinSearchSchema.parse({
        status: true,
        reason: ['a', 'b'],
        site: 123,
      })
    )
    expect(query.status).toBe('true')
    expect(query.reason).toEqual([])
    expect(query.site).toEqual(['123'])
  })
})

// ---------------------------------------------------------------------------
// routesSearchSchema (token-routes)
// ---------------------------------------------------------------------------

describe('routesSearchSchema', () => {
  it('tolerates null / array q values without throwing', () => {
    expect(routesSearchSchema.parse({ q: null }).q).toBeUndefined()
    expect(routesSearchSchema.parse({ q: ['a', 'b'] }).q).toBeUndefined()
  })

  it('ignores the removed site key without throwing', () => {
    const result = routesSearchSchema.parse({ site: 123 })
    expect(result).not.toHaveProperty('site')
  })

  it('falls back instead of throwing for page 0 / oversized pageSize', () => {
    const result = routesSearchSchema.parse({ page: 0, pageSize: 9999 })
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(9999)
  })
})

// ---------------------------------------------------------------------------
// proxyLogsSearchSchema
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema', () => {
  it('tolerates numeric epoch / null / array client·from·to values', () => {
    const result = proxyLogsSearchSchema.parse({
      client: ['a', 'b'],
      from: 1755000000,
      to: null,
      status: true,
      page: 0,
      pageSize: 9999,
    })
    expect(result.client).toBeUndefined()
    expect(result.from).toBe(1755000000)
    expect(result.to).toBeUndefined()
    expect(result.status).toBe('all')
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(20)
  })
})

// ---------------------------------------------------------------------------
// signInSearchSchema
// ---------------------------------------------------------------------------

describe('signInSearchSchema', () => {
  it('tolerates numeric / null / array redirect values without throwing', () => {
    expect(signInSearchSchema.parse({ redirect: 123 }).redirect).toBe(123)
    expect(
      signInSearchSchema.parse({ redirect: null }).redirect
    ).toBeUndefined()
    expect(
      signInSearchSchema.parse({ redirect: ['a', 'b'] }).redirect
    ).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// asStringParam — read-site coercion
// ---------------------------------------------------------------------------

describe('asStringParam', () => {
  it('coerces primitives that slip through the tolerant union to string', () => {
    expect(asStringParam('gpt')).toBe('gpt')
    expect(asStringParam(123)).toBe('123')
    expect(asStringParam(true)).toBe('true')
    expect(asStringParam(undefined)).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// hasValidAuthSessionSafe — sign-in / root beforeLoad storage guard
// ---------------------------------------------------------------------------

describe('hasValidAuthSessionSafe', () => {
  it('treats a SecurityError-throwing storage as unauthenticated instead of crashing', () => {
    const securityError = () => {
      throw new DOMException('Storage access denied', 'SecurityError')
    }
    const throwingStorage = {
      getItem: securityError,
      setItem: securityError,
      removeItem: securityError,
    }
    expect(() => hasValidAuthSessionSafe(throwingStorage)).not.toThrow()
    expect(hasValidAuthSessionSafe(throwingStorage)).toBe(false)
  })

  it('reports a valid stored session as authenticated', () => {
    const validStorage = {
      getItem: (key: string) =>
        key === 'auth_token' ? 'token-1' : '99999999999999',
      setItem: () => {},
      removeItem: () => {},
    }
    expect(hasValidAuthSessionSafe(validStorage)).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// tableSortingItemSchema — shared sorting item shape
// ---------------------------------------------------------------------------

describe('tableSortingItemSchema', () => {
  it('requires both id and desc', () => {
    expect(tableSortingItemSchema.safeParse({ id: 'a' }).success).toBe(false)
    expect(tableSortingItemSchema.safeParse({ desc: true }).success).toBe(false)
    expect(
      tableSortingItemSchema.safeParse({ id: 'a', desc: true }).success
    ).toBe(true)
  })
})
