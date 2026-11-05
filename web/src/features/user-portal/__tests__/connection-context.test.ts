// @vitest-environment node
import { describe, expect, it } from 'vitest'

import { modelContextHref, readModelContext } from '../connection-context'

describe('personal connection navigation context', () => {
  it('keeps a public model and its selected protocol without adding credentials', () => {
    const context = readModelContext(
      '?view=keys&model=public%2Fmodel&protocol=responses'
    )
    expect(context).toEqual({ model: 'public/model', protocol: 'responses' })
    const href = modelContextHref('?view=models&page=2&q=public', context)
    expect(
      new URL(href, 'https://gateway.example').searchParams.get('model')
    ).toBe('public/model')
    expect(
      readModelContext(new URL(href, 'https://gateway.example').search)
    ).toEqual(context)
    expect(href).toContain('page=2')
    expect(href).toContain('q=public')
  })

  it.each([
    '?model=one&model=two&protocol=responses',
    '?model=model&protocol=responses&protocol=messages',
    '?model=model&protocol=unknown',
    '?protocol=responses',
    '?model=',
    '?model=model%0Aother',
    '?model=' + 'm'.repeat(201),
  ])('rejects ambiguous or invalid values: %s', (search) => {
    expect(readModelContext(search)).toEqual({ invalid: true })
  })

  it('distinguishes no selection from a valid model with no protocol metadata', () => {
    expect(readModelContext('?view=keys')).toEqual({})
    expect(readModelContext('?model=model')).toEqual({
      model: 'model',
      protocol: undefined,
    })
    expect(
      modelContextHref('?view=models&model=old&protocol=responses', {})
    ).toBe('/user?view=models')
  })
})
