import type { TestChatRequestPayload } from './types'

export const testChatApi = {
  testChatSync: async (data: TestChatRequestPayload, signal?: AbortSignal) => {
    // #1034: auth rides the HttpOnly session cookie (sent automatically for
    // same-origin requests); no credential is read from or written to JS.
    const response = await fetch('/api/test/chat', {
      method: 'POST',
      signal,
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(data),
    })
    if (response.status === 401) {
      throw new Error('Session expired')
    }
    return response
  },
}
