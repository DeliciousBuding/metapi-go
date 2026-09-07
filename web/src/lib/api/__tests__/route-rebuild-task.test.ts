import { beforeEach, describe, expect, it, vi } from 'vitest'

import { eventsApi } from '../events'
import { tokenRoutesApi } from '../token-routes'

const client = vi.hoisted(() => ({ get: vi.fn(), request: vi.fn() }))
vi.mock('@/lib/http-client', () => ({
  apiClient: client,
  fetchAuthenticatedResponse: vi.fn(),
  extractResponseErrorMessage: vi.fn(),
}))

beforeEach(() => {
  client.get.mockReset().mockResolvedValue({ data: {} })
  client.request.mockReset()
})

describe('route rebuild background transport', () => {
  it('explicitly sends wait:false and returns the accepted task without a five-minute request', async () => {
    const queued = {
      success: true,
      queued: true,
      reused: false,
      jobId: 'rebuild-1',
      taskId: 'rebuild-1',
      status: 'pending',
    }
    client.request.mockResolvedValue({ status: 202, data: queued })

    expect(await tokenRoutesApi.rebuildRoutes()).toEqual(queued)

    const config = client.request.mock.calls[0][0]
    expect(config).toMatchObject({
      url: '/api/routes/rebuild',
      method: 'POST',
      timeout: 30_000,
    })
    expect(JSON.parse(config.data)).toEqual({
      refreshModels: true,
      wait: false,
    })
  })

  it('preserves an explicitly requested synchronous rebuild', async () => {
    client.request.mockResolvedValue({
      status: 200,
      data: { success: true, queued: false },
    })

    await tokenRoutesApi.rebuildRoutes(false, true)

    const config = client.request.mock.calls[0][0]
    expect(JSON.parse(config.data)).toEqual({
      refreshModels: false,
      wait: true,
    })
    expect(config.timeout).toBe(300_000)
  })

  it('uses the existing task endpoint with cancellation and caller-owned query errors', async () => {
    const controller = new AbortController()

    await eventsApi.getTask('rebuild/1', {
      signal: controller.signal,
      skipErrorHandler: true,
    })

    expect(client.get).toHaveBeenCalledWith(
      '/api/tasks/rebuild%2F1',
      expect.objectContaining({
        signal: controller.signal,
        skipErrorHandler: true,
      })
    )
  })
})
