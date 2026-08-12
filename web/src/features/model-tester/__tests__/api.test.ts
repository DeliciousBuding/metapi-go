// metapi-go/features/model-tester — error-mapping unit tests.
//
// `resolveTestResponseError` turns non-ok test responses into user-facing
// messages: backend 501s are known limitations (no fake SSE stream; the sync
// harness requires a forced channel) and map to a friendly localized key
// instead of the raw "not implemented" text.

import { describe, expect, it } from 'vitest'

import { resolveTestResponseError } from '../api'

describe('resolveTestResponseError', () => {
  it('maps the sync 501 (forced-channel harness required) to notAvailable', async () => {
    const response = new Response(
      JSON.stringify({
        success: false,
        message:
          'Chat test requires channelId, siteId, or forcedChannelId for the forced-channel harness',
      }),
      { status: 501 }
    )
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.notAvailable'
    )
  })

  it('maps the stream 501 (SSE not implemented) to notAvailable', async () => {
    const response = new Response(
      JSON.stringify({
        success: false,
        message: 'Chat stream test is not implemented in Go',
      }),
      { status: 501 }
    )
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.notAvailable'
    )
  })

  it('maps 401 to the session-expired key', async () => {
    const response = new Response(JSON.stringify({ message: 'unauthorized' }), {
      status: 401,
    })
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.sessionExpired'
    )
  })

  it('maps 403 to the session-expired key', async () => {
    const response = new Response(JSON.stringify({ error: 'forbidden' }), {
      status: 403,
    })
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.sessionExpired'
    )
  })

  it('falls back to the parsed backend error text for other statuses', async () => {
    const response = new Response(
      JSON.stringify({ success: false, message: 'boom' }),
      { status: 400 }
    )
    expect(await resolveTestResponseError(response)).toBe('boom')
  })

  it('falls back to HTTP status when the body is not JSON', async () => {
    const response = new Response('plain text body', { status: 500 })
    expect(await resolveTestResponseError(response)).toBe('plain text body')
  })
})
