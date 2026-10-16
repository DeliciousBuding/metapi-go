import { z } from 'zod'

import { buildQueryString, request } from './transport'

export const gatewayRequestStatusSchema = z.enum([
  'completed',
  'upstream_failed',
  'output_interrupted',
  'client_cancelled',
  'rejected_no_route',
  'rejected_policy',
  'gateway_failed',
])

const gatewayRequestSchema = z.object({
  id: z.string().min(1),
  externalRequestId: z.string().nullable(),
  downstreamPath: z.string().nullable(),
  transport: z.enum(['http', 'websocket']),
  modelRequested: z.string().nullable(),
  downstreamApiKeyId: z.number().int().positive().nullable(),
  terminalStatus: gatewayRequestStatusSchema,
  reasonCode: z.string().nullable(),
  httpStatus: z.number().int().min(100).max(599).nullable(),
  outputStarted: z.boolean(),
  firstByteLatencyMs: z.number().nonnegative().nullable(),
  firstOutputLatencyMs: z.number().nonnegative().nullable(),
  latencyMs: z.number().nonnegative(),
  startedAt: z.string().min(1),
  endedAt: z.string().min(1),
  attemptCount: z.number().int().nonnegative(),
})

const gatewayRequestSummarySchema = z.object({
  totalCount: z.number().int().nonnegative(),
  completedCount: z.number().int().nonnegative(),
  failedCount: z.number().int().nonnegative(),
  cancelledCount: z.number().int().nonnegative(),
  rejectedCount: z.number().int().nonnegative(),
})

export const gatewayRequestsResponseSchema = z.object({
  items: z.array(gatewayRequestSchema),
  total: z.number().int().nonnegative(),
  page: z.number().int().positive(),
  pageSize: z.number().int().positive(),
  summary: gatewayRequestSummarySchema,
  coverageStartedAt: z.string().nullable(),
})

// Attempt rows keep their existing wire vocabulary. Validate the fields this
// view consumes; missing measurements stay unknown rather than becoming zero.
const gatewayRequestAttemptSchema = z.object({
  id: z.number().int().positive(),
  createdAt: z.string(),
  modelRequested: z.string().nullable(),
  modelActual: z.string().nullable(),
  status: z.string().nullable(),
  httpStatus: z.number().int().nullable().optional(),
  latencyMs: z.number().nonnegative().nullable(),
  retryCount: z.number().int().nonnegative().nullable(),
  totalTokens: z.number().nonnegative().nullable(),
  promptTokens: z.number().nonnegative().nullable().optional(),
  completionTokens: z.number().nonnegative().nullable().optional(),
  cacheReadTokens: z.number().nonnegative().nullable().optional(),
  cacheCreationTokens: z.number().nonnegative().nullable().optional(),
  estimatedCost: z.number().nonnegative().nullable().optional(),
  isStream: z.boolean().nullable().optional(),
  firstByteLatencyMs: z.number().nonnegative().nullable().optional(),
  firstOutputLatencyMs: z.number().nonnegative().nullable().optional(),
  siteName: z.string().nullable().optional(),
  username: z.string().nullable().optional(),
  errorMessage: z.string().nullable().optional(),
})

export const gatewayRequestDetailSchema = gatewayRequestSchema.extend({
  attempts: z.array(gatewayRequestAttemptSchema),
  partial: z.boolean(),
})

export const gatewayOverviewFieldsSchema = z
  .object({
    gatewayRequestSummary: gatewayRequestSummarySchema
      .extend({ coverageStartedAt: z.string().nullable() })
      .optional(),
    gatewayRequestPoints: z
      .array(
        z.object({
          date: z.string(),
          requests: z.number().int().nonnegative(),
          completedCount: z.number().int().nonnegative(),
          failedCount: z.number().int().nonnegative(),
          cancelledCount: z.number().int().nonnegative(),
          rejectedCount: z.number().int().nonnegative(),
        })
      )
      .optional(),
  })
  .refine(
    (value) =>
      (value.gatewayRequestSummary === undefined) ===
      (value.gatewayRequestPoints === undefined),
    { message: 'Gateway request overview requires both summary and points' }
  )

export type GatewayRequest = z.infer<typeof gatewayRequestSchema>
export type GatewayRequestDetail = z.infer<typeof gatewayRequestDetailSchema>
export type GatewayRequestStatus = z.infer<typeof gatewayRequestStatusSchema>
export type GatewayRequestsQuery = {
  limit?: number
  offset?: number
  status?: GatewayRequestStatus
  search?: string
  downstreamKeyId?: number
  // UI-local datetime or RFC3339; serialized to UTC before the HTTP request.
  from?: string
  to?: string
}

export const gatewayRequestsApi = {
  getGatewayRequests: async (params?: GatewayRequestsQuery) =>
    gatewayRequestsResponseSchema.parse(
      await request(
        `/api/stats/requests${buildQueryString({
          ...params,
          // URL filters also back datetime-local inputs. Convert in the browser
          // timezone; an existing RFC3339 offset keeps the same instant.
          from: params?.from ? new Date(params.from).toISOString() : undefined,
          to: params?.to ? new Date(params.to).toISOString() : undefined,
        })}`
      )
    ),
  getGatewayRequest: async (id: string) =>
    gatewayRequestDetailSchema.parse(
      await request(`/api/stats/requests/${encodeURIComponent(id)}`)
    ),
}
