import { describe, expect, it, vi } from 'vitest'

import {
  gatewayOverviewFieldsSchema,
  gatewayRequestDetailSchema,
  gatewayRequestsApi,
  gatewayRequestsResponseSchema,
} from '@/lib/api/gateway-requests'
import { request } from '@/lib/api/transport'

import { gatewayRequestFixture } from './gateway-request-fixture'

vi.mock('@/lib/api/transport', async (original) => ({
  ...(await original<typeof import('@/lib/api/transport')>()),
  request: vi.fn(),
}))

describe('gateway request wire validation', () => {
  it('keeps a zero-dispatch rejection, unknown timings and the committed HTTP status', () => {
    const detail = gatewayRequestDetailSchema.parse(
      gatewayRequestFixture({
        terminalStatus: 'rejected_no_route',
        httpStatus: 503,
        outputStarted: false,
        attemptCount: 0,
        attempts: [],
        firstByteLatencyMs: null,
      })
    )
    expect(detail.attempts).toEqual([])
    expect(detail.firstByteLatencyMs).toBeNull()
    expect(detail.httpStatus).toBe(503)
  })
  it('does not turn malformed lists or missing terminal metadata into an empty success', () => {
    expect(() =>
      gatewayRequestsResponseSchema.parse({ items: 'invalid', total: 0 })
    ).toThrow()
    expect(() =>
      gatewayRequestDetailSchema.parse({
        ...gatewayRequestFixture(),
        terminalStatus: 'success',
      })
    ).toThrow()
    expect(() =>
      gatewayRequestDetailSchema.parse({
        ...gatewayRequestFixture(),
        attempts: [{}],
      })
    ).toThrow()
    expect(() =>
      gatewayRequestDetailSchema.parse({
        ...gatewayRequestFixture(),
        httpStatus: undefined,
      })
    ).toThrow()
  })
  it('uses the request endpoint and validates the response at the network boundary', async () => {
    vi.mocked(request).mockResolvedValueOnce(
      gatewayRequestFixture({
        terminalStatus: 'output_interrupted',
        httpStatus: 200,
      })
    )
    const result = await gatewayRequestsApi.getGatewayRequest('synthetic/id')
    expect(request).toHaveBeenLastCalledWith(
      '/api/stats/requests/synthetic%2Fid'
    )
    expect(result.terminalStatus).toBe('output_interrupted')
    vi.mocked(request).mockResolvedValueOnce({ items: [] })
    await expect(gatewayRequestsApi.getGatewayRequests()).rejects.toThrow()
  })
  it.each([
    ['manual local input', '2026-10-16T00:00', '2026-10-16T23:59'],
    ['preset local range', '2026-10-15T12:34', '2026-10-16T12:34'],
    [
      'overview offset range',
      '2026-10-16T00:00:00+08:00',
      '2026-10-16T01:00:00+08:00',
    ],
    ['overview UTC range', '2026-10-15T16:00:00Z', '2026-10-16T16:00:00Z'],
  ])(
    'serializes %s as RFC3339 instants at the API boundary',
    async (_, from, to) => {
      vi.mocked(request).mockResolvedValueOnce({
        items: [],
        total: 0,
        page: 1,
        pageSize: 50,
        summary: {
          totalCount: 0,
          completedCount: 0,
          failedCount: 0,
          cancelledCount: 0,
          rejectedCount: 0,
        },
        coverageStartedAt: null,
      })
      await gatewayRequestsApi.getGatewayRequests({ from, to, limit: 50 })
      const [url] = vi.mocked(request).mock.calls.at(-1)!
      const query = new URL(url, 'http://localhost').searchParams
      expect(query.get('from')).toBe(new Date(from).toISOString())
      expect(query.get('to')).toBe(new Date(to).toISOString())
      expect(query.get('from')).toMatch(/T\d{2}:\d{2}:\d{2}\.\d{3}Z$/)
      expect(query.get('limit')).toBe('50')
    }
  )
  it('does not send an invalid date or silently remove its filter', async () => {
    vi.mocked(request).mockClear()
    await expect(
      gatewayRequestsApi.getGatewayRequests({ from: 'not-a-date' })
    ).rejects.toThrow()
    expect(request).not.toHaveBeenCalled()
  })
  it('allows an old overview but refuses half-present or malformed new outcomes', () => {
    expect(gatewayOverviewFieldsSchema.parse({ requestSummary: {} })).toEqual(
      {}
    )
    expect(() =>
      gatewayOverviewFieldsSchema.parse({ gatewayRequestPoints: [] })
    ).toThrow()
    expect(() =>
      gatewayOverviewFieldsSchema.parse({
        gatewayRequestSummary: { totalCount: '0' },
        gatewayRequestPoints: [],
      })
    ).toThrow()
  })
})
