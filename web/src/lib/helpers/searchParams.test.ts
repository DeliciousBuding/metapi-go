// metapi-go/helpers — tests for URL search-param decoding. Covers the three
// wire shapes each param can arrive in: comma string, router-serialized JSON
// array, and the literal `[]` empty marker (plus undefined / garbage input).

import { describe, expect, it } from 'vitest'

import { parseSortingParam, parseStringListParam } from './searchParams'

describe('parseSortingParam', () => {
  it('returns [] for undefined / empty input', () => {
    expect(parseSortingParam(undefined)).toEqual([])
    expect(parseSortingParam('')).toEqual([])
    expect(parseSortingParam('   ')).toEqual([])
  })

  it('returns [] for the literal empty-array marker', () => {
    expect(parseSortingParam('[]')).toEqual([])
  })

  it('parses the comma-separated string form', () => {
    expect(parseSortingParam('name:desc,url:asc')).toEqual([
      { id: 'name', desc: true },
      { id: 'url', desc: false },
    ])
  })

  it('defaults a missing direction to ascending', () => {
    expect(parseSortingParam('name')).toEqual([{ id: 'name', desc: false }])
  })

  it('parses the router-serialized JSON array form', () => {
    const json = '[{"id":"name","desc":true},{"id":"url","desc":false}]'
    expect(parseSortingParam(json)).toEqual([
      { id: 'name', desc: true },
      { id: 'url', desc: false },
    ])
  })

  it('passes an already-parsed array through', () => {
    expect(parseSortingParam([{ id: 'name', desc: true }])).toEqual([
      { id: 'name', desc: true },
    ])
  })

  it('drops malformed items from a JSON array', () => {
    const json = '[{"id":"name","desc":true},{"id":7}]'
    expect(parseSortingParam(json)).toEqual([{ id: 'name', desc: true }])
  })

  it('falls back to the comma form for invalid JSON', () => {
    expect(parseSortingParam('[broken')).toEqual([
      { id: '[broken', desc: false },
    ])
  })
})

describe('parseStringListParam', () => {
  it('returns [] for undefined / empty input', () => {
    expect(parseStringListParam(undefined)).toEqual([])
    expect(parseStringListParam('')).toEqual([])
    expect(parseStringListParam('   ')).toEqual([])
  })

  it('returns [] for the literal empty-array marker', () => {
    expect(parseStringListParam('[]')).toEqual([])
  })

  it('parses the comma-separated string form', () => {
    expect(parseStringListParam('openai,anthropic')).toEqual([
      'openai',
      'anthropic',
    ])
  })

  it('trims and drops empty segments', () => {
    expect(parseStringListParam(' openai , , anthropic ')).toEqual([
      'openai',
      'anthropic',
    ])
  })

  it('parses the router-serialized JSON array form', () => {
    expect(parseStringListParam('["openai","anthropic"]')).toEqual([
      'openai',
      'anthropic',
    ])
  })

  it('passes an already-parsed array through', () => {
    expect(parseStringListParam(['openai', 'anthropic'])).toEqual([
      'openai',
      'anthropic',
    ])
  })
})
