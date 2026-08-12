// metapi-go/features/settings/lib — dirty-field diffing tests. The submit
// path sends only fields that changed against the server baseline, so
// untouched config (including masked secrets) is never re-sent.

import { describe, expect, it } from 'vitest'

import {
  collectChangedFields,
  hasChanges,
  isDeepEqual,
} from '../collect-changed-fields'

describe('collectChangedFields', () => {
  it('returns an empty diff when nothing changed', () => {
    const baseline = { name: 'a', count: 1 }
    expect(collectChangedFields({ name: 'a', count: 1 }, baseline)).toEqual({})
  })

  it('returns only the changed top-level field', () => {
    const baseline = { name: 'a', count: 1 }
    expect(collectChangedFields({ name: 'b', count: 1 }, baseline)).toEqual({
      name: 'b',
    })
  })

  it('includes a whole nested object when any child differs', () => {
    const baseline = { weights: { a: 1, b: 2 } }
    expect(collectChangedFields({ weights: { a: 1, b: 3 } }, baseline)).toEqual(
      {
        weights: { a: 1, b: 3 },
      }
    )
  })

  it('treats equal nested objects as unchanged', () => {
    const baseline = { weights: { a: 1, b: 2 } }
    expect(collectChangedFields({ weights: { a: 1, b: 2 } }, baseline)).toEqual(
      {}
    )
  })

  it('detects array changes', () => {
    const baseline = { list: ['a', 'b'] }
    expect(collectChangedFields({ list: ['a', 'c'] }, baseline)).toEqual({
      list: ['a', 'c'],
    })
  })

  it('treats equal arrays as unchanged', () => {
    const baseline = { list: ['a', 'b'] }
    expect(collectChangedFields({ list: ['a', 'b'] }, baseline)).toEqual({})
  })

  it('returns the full values when baseline is missing', () => {
    expect(collectChangedFields({ name: 'a' }, null)).toEqual({ name: 'a' })
    expect(collectChangedFields({ name: 'a' }, undefined)).toEqual({
      name: 'a',
    })
  })

  it('detects type changes', () => {
    const baseline = { count: 1 as number | string }
    expect(collectChangedFields({ count: '1' }, baseline)).toEqual({
      count: '1',
    })
  })
})

describe('hasChanges', () => {
  it('is false for an empty diff', () => {
    expect(hasChanges({})).toBe(false)
  })

  it('is true for a non-empty diff', () => {
    expect(hasChanges({ name: 'b' })).toBe(true)
  })
})

describe('isDeepEqual', () => {
  it('handles primitives', () => {
    expect(isDeepEqual(1, 1)).toBe(true)
    expect(isDeepEqual(1, 2)).toBe(false)
    expect(isDeepEqual('a', 'a')).toBe(true)
    expect(isDeepEqual(true, false)).toBe(false)
  })

  it('treats null/undefined as distinct from objects', () => {
    expect(isDeepEqual(null, null)).toBe(true)
    expect(isDeepEqual(null, undefined)).toBe(false)
    expect(isDeepEqual({ a: 1 }, null)).toBe(false)
  })

  it('compares nested objects by shape', () => {
    expect(isDeepEqual({ a: { b: 1 } }, { a: { b: 1 } })).toBe(true)
    expect(isDeepEqual({ a: { b: 1 } }, { a: { b: 2 } })).toBe(false)
  })

  it('compares arrays by length and contents', () => {
    expect(isDeepEqual([1, 2], [1, 2])).toBe(true)
    expect(isDeepEqual([1, 2], [1, 2, 3])).toBe(false)
  })
})
