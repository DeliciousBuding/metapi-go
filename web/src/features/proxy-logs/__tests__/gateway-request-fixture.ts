import type { GatewayRequestDetail } from '@/lib/api'

export function gatewayRequestFixture(
  overrides: Partial<GatewayRequestDetail> = {}
): GatewayRequestDetail {
  return {
    id: 'synthetic-request-1',
    externalRequestId: null,
    downstreamPath: '/v1/responses',
    transport: 'http',
    modelRequested: 'synthetic-model',
    downstreamApiKeyId: 7,
    terminalStatus: 'completed',
    reasonCode: null,
    httpStatus: 200,
    outputStarted: true,
    firstByteLatencyMs: 0,
    firstOutputLatencyMs: null,
    latencyMs: 120,
    startedAt: '2026-10-16T01:00:00Z',
    endedAt: '2026-10-16T01:00:00.120Z',
    attemptCount: 1,
    partial: false,
    attempts: [
      {
        id: 17,
        createdAt: '2026-10-16T01:00:00Z',
        modelRequested: 'synthetic-model',
        modelActual: 'synthetic-upstream-model',
        status: 'success',
        httpStatus: 200,
        latencyMs: 115,
        retryCount: 0,
        totalTokens: null,
        promptTokens: 0,
        completionTokens: null,
        estimatedCost: null,
      },
    ],
    ...overrides,
  }
}
