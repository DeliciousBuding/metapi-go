// metapi-go/features/sites — pure unit tests for the apiEndpoints editor
// helpers (normalize / validate / serialize / parse). The server-side parity
// contract lives in service/site_endpoint_service.go; these tests pin the
// TS mirror so client-side rejection matches what the backend enforces.
import { describe, expect, it } from 'vitest'

import {
  isForbiddenEndpointTargetHost,
  isHttpUrl,
  isValidEndpointUrl,
  normalizeEndpointBaseUrl,
  parseEndpointsEditorText,
  serializeEndpointsForEditor,
} from '../lib/endpoints'

describe('normalizeEndpointBaseUrl (Go parity)', () => {
  it('trims whitespace and a trailing slash', () => {
    expect(normalizeEndpointBaseUrl('  https://a.example/  ')).toBe(
      'https://a.example'
    )
    expect(normalizeEndpointBaseUrl('https://a.example/path/')).toBe(
      'https://a.example/path'
    )
  })

  it('strips the search and hash parts', () => {
    expect(normalizeEndpointBaseUrl('https://a.example/path?q=1#frag')).toBe(
      'https://a.example/path'
    )
  })

  it('falls back to the lenient trim when the URL cannot be parsed', () => {
    expect(normalizeEndpointBaseUrl('not a url/')).toBe('not a url')
    expect(normalizeEndpointBaseUrl('')).toBe('')
  })
})

describe('isHttpUrl / isValidEndpointUrl', () => {
  it('accepts only http(s) URLs', () => {
    expect(isHttpUrl('https://x.example')).toBe(true)
    expect(isHttpUrl('http://x.example')).toBe(true)
    expect(isHttpUrl('ftp://x.example')).toBe(false)
    expect(isHttpUrl('not a url')).toBe(false)
  })

  it('rejects cloud metadata / link-local targets but keeps private ranges', () => {
    expect(isValidEndpointUrl('https://api.example.com')).toBe(true)
    expect(isValidEndpointUrl('http://169.254.169.254/latest')).toBe(false)
    expect(isValidEndpointUrl('https://metadata.google.internal')).toBe(false)
    expect(isValidEndpointUrl('https://10.0.0.5')).toBe(true)
    expect(isValidEndpointUrl('https://localhost:9000')).toBe(true)
    expect(isValidEndpointUrl('')).toBe(false)
  })
})

describe('isForbiddenEndpointTargetHost (Go IsForbiddenSiteTargetURL parity)', () => {
  it('flags the well-known metadata hostnames case-insensitively', () => {
    expect(isForbiddenEndpointTargetHost('metadata.google.internal')).toBe(true)
    expect(isForbiddenEndpointTargetHost('METADATA')).toBe(true)
    expect(isForbiddenEndpointTargetHost('instance-data')).toBe(true)
  })

  it('flags link-local addresses and allows private/lab targets', () => {
    expect(isForbiddenEndpointTargetHost('169.254.169.254')).toBe(true)
    expect(isForbiddenEndpointTargetHost('10.0.0.1')).toBe(false)
    expect(isForbiddenEndpointTargetHost('localhost')).toBe(false)
    expect(isForbiddenEndpointTargetHost('example.com')).toBe(false)
  })

  it('flags IPv6 link-local and IPv4-mapped forms', () => {
    expect(isForbiddenEndpointTargetHost('fe80::1')).toBe(true)
    expect(isForbiddenEndpointTargetHost('[fe80::1]')).toBe(true)
    expect(isForbiddenEndpointTargetHost('fd7a::1')).toBe(false)
    expect(isForbiddenEndpointTargetHost('::ffff:169.254.10.20')).toBe(true)
    expect(isForbiddenEndpointTargetHost('::ffff:10.0.0.1')).toBe(false)
  })
})

describe('parseEndpointsEditorText', () => {
  it('treats blank and whitespace-only lines as ignored', () => {
    expect(parseEndpointsEditorText('')).toEqual({ endpoints: [] })
    expect(parseEndpointsEditorText('\n  \n\t\n')).toEqual({ endpoints: [] })
  })

  it('parses plain URLs with enabled true and positional sortOrder', () => {
    expect(
      parseEndpointsEditorText('https://a.example\n\nhttps://b.example')
    ).toEqual({
      endpoints: [
        { url: 'https://a.example', enabled: true, sortOrder: 0 },
        { url: 'https://b.example', enabled: true, sortOrder: 1 },
      ],
    })
  })

  it('parses JSON-object lines with explicit enabled/sortOrder', () => {
    expect(
      parseEndpointsEditorText(
        '{"url":"https://b.example","enabled":false,"sortOrder":5}'
      )
    ).toEqual({
      endpoints: [{ url: 'https://b.example', enabled: false, sortOrder: 5 }],
    })
  })

  it('accepts mixed plain and JSON lines preserving list order', () => {
    expect(
      parseEndpointsEditorText(
        'https://a.example\n{"url":"https://b.example","enabled":false,"sortOrder":9}\nhttps://c.example'
      )
    ).toEqual({
      endpoints: [
        { url: 'https://a.example', enabled: true, sortOrder: 0 },
        { url: 'https://b.example', enabled: false, sortOrder: 9 },
        { url: 'https://c.example', enabled: true, sortOrder: 2 },
      ],
    })
  })

  it('reports the first error line for invalid JSON', () => {
    expect(parseEndpointsEditorText('{not json}')).toEqual({
      error: 'invalidJson',
    })
  })

  it('reports invalid entries for malformed JSON objects', () => {
    expect(parseEndpointsEditorText('[]')).toEqual({ error: 'invalidEntry' })
    expect(parseEndpointsEditorText('{"enabled":true}')).toEqual({
      error: 'invalidEntry',
    })
    expect(parseEndpointsEditorText('{"url":12}')).toEqual({
      error: 'invalidEntry',
    })
    expect(
      parseEndpointsEditorText('{"url":"https://a.example","enabled":"yes"}')
    ).toEqual({ error: 'invalidEntry' })
    expect(
      parseEndpointsEditorText('{"url":"https://a.example","sortOrder":1.5}')
    ).toEqual({ error: 'invalidEntry' })
    expect(
      parseEndpointsEditorText('{"url":"https://a.example","sortOrder":-1}')
    ).toEqual({ error: 'invalidEntry' })
  })

  it('rejects non-http(s) and forbidden endpoint URLs', () => {
    expect(parseEndpointsEditorText('ftp://a.example')).toEqual({
      error: 'invalidUrl',
    })
    expect(parseEndpointsEditorText('not a url')).toEqual({
      error: 'invalidUrl',
    })
    expect(parseEndpointsEditorText('http://169.254.169.254/latest')).toEqual({
      error: 'invalidUrl',
    })
    expect(
      parseEndpointsEditorText('{"url":"http://metadata.google.internal"}')
    ).toEqual({ error: 'invalidUrl' })
  })

  it('rejects duplicates on the normalized URL (trailing slash variants)', () => {
    expect(
      parseEndpointsEditorText('https://dup.example\nhttps://dup.example/')
    ).toEqual({ error: 'duplicate' })
    expect(
      parseEndpointsEditorText(
        'https://a.example\nhttps://dup.example\nhttps://dup.example?x=1'
      )
    ).toEqual({ error: 'duplicate' })
  })
})

describe('serializeEndpointsForEditor', () => {
  it('serializes undefined to an empty editor value', () => {
    expect(serializeEndpointsForEditor(undefined)).toBe('')
    expect(serializeEndpointsForEditor([])).toBe('')
  })

  it('writes one compact JSON object per line with defaults', () => {
    expect(
      serializeEndpointsForEditor([
        { url: 'https://a.example' },
        { url: 'https://b.example', enabled: false, sortOrder: 5 },
      ])
    ).toBe(
      '{"url":"https://a.example","enabled":true,"sortOrder":0}\n{"url":"https://b.example","enabled":false,"sortOrder":5}'
    )
  })

  it('round-trips through parseEndpointsEditorText losslessly', () => {
    const endpoints = [
      { url: 'https://a.example', enabled: true, sortOrder: 0 },
      { url: 'https://b.example', enabled: false, sortOrder: 7 },
    ]
    const text = serializeEndpointsForEditor(endpoints)
    expect(parseEndpointsEditorText(text)).toEqual({ endpoints })
  })
})
