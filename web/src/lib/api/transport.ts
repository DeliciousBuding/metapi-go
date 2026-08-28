/**
 * metapi-go API transport layer.
 *
 * Shared low-level helpers for the domain modules in this directory:
 * JSON requests through the shared axios instance (`apiClient` from
 * `@/lib/http-client`, which owns auth injection, GET dedup, business-error
 * toasts and 401 → refresh → replay) plus the streaming / raw-Response / SSE
 * endpoints that cannot pass through the axios JSON interceptors.
 */

import {
  apiClient,
  extractResponseErrorMessage,
  fetchAuthenticatedResponse,
  type ApiRequestConfig,
} from '@/lib/http-client'

export type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  body?: string
  timeoutMs?: number
  signal?: AbortSignal
  headers?: Record<string, string>
  skipErrorHandler?: boolean
  skipBusinessError?: boolean
  /** Skip the 401 -> sign-in redirect flow (login probe owns its errors). */
  skipAuthRetry?: boolean
  /** Skip GET dedup (login must never ride a cached in-flight request). */
  disableDuplicate?: boolean
}

/**
 * Core JSON request helper. Routes GET through `apiClient.get` so the
 * http-client GET-dedup applies; non-GET goes through `apiClient.request`.
 * Returns the parsed response body, matching the legacy `res.json()` contract.
 */
export async function request<T = unknown>(
  url: string,
  options: RequestOptions = {}
): Promise<T> {
  const {
    method = 'GET',
    body,
    timeoutMs = 30_000,
    signal,
    headers,
    skipErrorHandler = false,
    skipBusinessError = false,
    skipAuthRetry = false,
    disableDuplicate = false,
  } = options

  const requestHeaders: Record<string, string> | undefined = body
    ? { 'Content-Type': 'application/json', ...headers }
    : headers

  const baseConfig: ApiRequestConfig = {
    timeout: timeoutMs,
    signal,
    headers: requestHeaders,
    skipErrorHandler,
    skipBusinessError,
    skipAuthRetry,
    disableDuplicate,
  }

  const response =
    method === 'GET'
      ? await apiClient.get(url, baseConfig)
      : await apiClient.request({
          url,
          method,
          data: body,
          ...baseConfig,
        })

  return response.data as T
}

export function buildQueryString(
  params?: Record<string, string | number | boolean | null | undefined>
) {
  if (!params) return ''
  const searchParams = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    searchParams.set(key, String(value))
  }
  const serialized = searchParams.toString()
  return serialized ? `?${serialized}` : ''
}

export function parseContentDispositionFilename(
  headerValue: string | null
): string | null {
  if (!headerValue) return null
  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(headerValue)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return utf8Match[1]
    }
  }
  const quotedMatch = /filename="([^"]+)"/i.exec(headerValue)
  if (quotedMatch?.[1]) return quotedMatch[1]
  const bareMatch = /filename=([^;]+)/i.exec(headerValue)
  return bareMatch?.[1]?.trim() || null
}

type BufferLike = {
  from(data: ArrayBuffer): { toString(encoding: 'base64'): string }
}

const nodeBuffer = (globalThis as typeof globalThis & { Buffer?: BufferLike })
  .Buffer

export function arrayBufferToBase64(buffer: ArrayBuffer): string {
  if (nodeBuffer) {
    return nodeBuffer.from(buffer).toString('base64')
  }

  let binary = ''
  const bytes = new Uint8Array(buffer)
  const chunkSize = 0x8000
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize))
  }
  return btoa(binary)
}

export async function streamSse(
  url: string,
  handlers: {
    onLog?: (entry: unknown) => void
    onDone?: (payload: unknown) => void
    /**
     * Fires for every parsed SSE event (the raw event name + payload), so
     * callers with custom event vocabularies (e.g. probe-stream's
     * probe-start / probe-result / complete) do not need a second reader.
     */
    onEvent?: (event: string, payload: unknown) => void
    signal?: AbortSignal
  }
) {
  const response = await fetchAuthenticatedResponse(url, {
    method: 'GET',
    signal: handlers.signal,
    headers: {
      Accept: 'text/event-stream',
    },
    timeoutMs: 120_000,
  })

  if (!response.ok) {
    throw new Error(await extractResponseErrorMessage(response))
  }
  if (!response.body) {
    throw new Error('Stream response body is missing')
  }

  const decoder = new TextDecoder()
  const reader = response.body.getReader()
  let buffer = ''

  const flushBuffer = (final = false) => {
    const chunks = final ? [...buffer.split('\n\n'), ''] : buffer.split('\n\n')
    if (!final) buffer = chunks.pop() || ''
    else buffer = ''

    for (const chunk of chunks) {
      const lines = chunk.split('\n')
      let eventName = 'message'
      const dataLines: string[] = []

      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventName = line.slice('event:'.length).trim() || 'message'
        } else if (line.startsWith('data:')) {
          dataLines.push(line.slice('data:'.length).trim())
        }
      }

      if (dataLines.length <= 0) continue
      const raw = dataLines.join('\n')
      let payload: unknown = raw
      try {
        payload = JSON.parse(raw)
      } catch {
        // keep string payload
      }

      handlers.onEvent?.(eventName, payload)
      if (eventName === 'log') {
        handlers.onLog?.(payload)
      } else if (eventName === 'done') {
        handlers.onDone?.(payload)
      }
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    flushBuffer(false)
  }

  if (buffer.trim()) {
    flushBuffer(true)
  }
}
