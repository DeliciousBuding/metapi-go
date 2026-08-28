import {
  fetchAuthenticatedResponse,
  extractResponseErrorMessage,
} from '@/lib/http-client'

import {
  request,
  parseContentDispositionFilename,
  arrayBufferToBase64,
  type RequestOptions,
} from './transport'
import type { ProxyTestRequestEnvelope, TestChatRequestPayload } from './types'

const DEFAULT_PROXY_TEST_TIMEOUT_MS = 30_000
const LONG_RUNNING_PROXY_TEST_TIMEOUT_MS = 150_000

function resolveProxyTestTimeoutMs(data: ProxyTestRequestEnvelope) {
  if (data.jobMode) return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  if (data.path === '/v1/images/generations') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  if (data.path === '/v1/images/edits') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  if (data.path === '/v1/videos' && data.method === 'POST') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  return DEFAULT_PROXY_TEST_TIMEOUT_MS
}

export const testChatApi = {
  // Simple chat test from admin panel
  startTestChatJob: (data: TestChatRequestPayload) =>
    request('/api/test/chat/jobs', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getTestChatJob: (jobId: string) =>
    request(`/api/test/chat/jobs/${encodeURIComponent(jobId)}`),
  deleteTestChatJob: (jobId: string) =>
    request(`/api/test/chat/jobs/${encodeURIComponent(jobId)}`, {
      method: 'DELETE',
    }),
  startProxyTestJob: (data: ProxyTestRequestEnvelope) =>
    request('/api/test/proxy/jobs', {
      method: 'POST',
      body: JSON.stringify(data),
      timeoutMs: resolveProxyTestTimeoutMs(data),
    }),
  getProxyTestJob: (jobId: string) =>
    request(`/api/test/proxy/jobs/${encodeURIComponent(jobId)}`),
  deleteProxyTestJob: (jobId: string) =>
    request(`/api/test/proxy/jobs/${encodeURIComponent(jobId)}`, {
      method: 'DELETE',
    }),
  getProxyFileContentDataUrl: async (
    fileId: string,
    options: Pick<RequestOptions, 'signal' | 'timeoutMs'> = {}
  ) => {
    const response = await fetchAuthenticatedResponse(
      `/v1/files/${encodeURIComponent(fileId)}/content`,
      {
        method: 'GET',
        ...options,
      }
    )
    if (!response.ok) {
      throw new Error(await extractResponseErrorMessage(response))
    }

    const mimeType =
      (response.headers.get('content-type') || 'application/octet-stream')
        .split(';')[0]
        .trim() || 'application/octet-stream'
    const filename = parseContentDispositionFilename(
      response.headers.get('content-disposition')
    )
    const base64 = arrayBufferToBase64(await response.arrayBuffer())
    return {
      filename,
      mimeType,
      data: `data:${mimeType};base64,${base64}`,
    }
  },
  testChat: (data: TestChatRequestPayload) =>
    request('/api/test/chat', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
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
