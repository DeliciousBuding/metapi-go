import { z } from 'zod'

const observationPointer = z
  .string()
  .max(1024)
  .refine(
    (path) =>
      path.startsWith('/') &&
      path.split('/').length <= 65 &&
      !/[\r\n\0]/.test(path) &&
      !/~(?![01])/.test(path)
  )
const observation = z
  .object({
    usagePaths: z
      .partialRecord(
        z.enum([
          'prompt_tokens',
          'completion_tokens',
          'total_tokens',
          'input_tokens',
          'output_tokens',
          'cache_read_input_tokens',
          'cache_creation_input_tokens',
          'prompt_tokens_details.cached_tokens',
          'input_tokens_details.cached_tokens',
          'completion_tokens_details.reasoning_tokens',
          'output_tokens_details.reasoning_tokens',
          'promptTokenCount',
          'candidatesTokenCount',
          'totalTokenCount',
          'thoughtsTokenCount',
        ]),
        observationPointer
      )
      .optional(),
    modelPath: observationPointer.optional(),
    errorPath: observationPointer.optional(),
    taskIdPath: observationPointer.optional(),
  })
  .strict()
  .refine(
    (value) =>
      Object.keys(value.usagePaths ?? {}).length > 0 ||
      !!value.modelPath ||
      !!value.errorPath ||
      !!value.taskIdPath
  )
export const protocolObservations = z
  .object({ response: observation.optional(), event: observation.optional() })
  .strict()
  .refine((value) => !!value.response || !!value.event)
const authenticationClaim = z.string().regex(/^[A-Za-z_][A-Za-z0-9_-]{0,63}$/)
export const authentication = z
  .object({
    type: z.enum(['api_key', 'jwt_hs256']),
    header: z
      .string()
      .min(1)
      .max(128)
      .regex(/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/)
      .refine((header) => {
        const name = header.toLowerCase()
        return (
          ![
            'sec-',
            'proxy-',
            'x-forwarded-',
            'x-codex-',
            'x-claude-code-',
          ].some((prefix) => name.startsWith(prefix)) &&
          ![
            'host',
            'cookie',
            'set-cookie',
            'connection',
            'transfer-encoding',
            'content-type',
            'content-length',
            'content-encoding',
            'accept-encoding',
            'accept',
            'te',
            'trailer',
            'upgrade',
            'forwarded',
            'x-real-ip',
            'session-id',
            'conversation_id',
            'conversation-id',
            'thread-id',
            'chatgpt-account-id',
            'anthropic-version',
            'anthropic-beta',
            'openai-beta',
            'openai-organization',
            'openai-project',
            'x-request-id',
            'x-client-request-id',
            'request-id',
            'traceparent',
            'tracestate',
            'baggage',
          ].includes(name)
        )
      }),
    prefix: z
      .string()
      .refine((value) => new TextEncoder().encode(value).length <= 256)
      .refine((value) => !/\p{Cc}/u.test(value))
      .optional(),
    jwt: z
      .object({
        keySeparator: z.enum(['.', '|']),
        issuerClaim: authenticationClaim.refine(
          (claim) => !['exp', 'iat', 'nbf'].includes(claim)
        ),
        ttlSeconds: z.number().int().min(60).max(86400),
        timeUnit: z.enum(['seconds', 'milliseconds']),
        issuedAtClaim: authenticationClaim.optional(),
        notBeforeSkewSeconds: z.number().int().min(0).max(300).optional(),
        signType: z.enum(['', 'SIGN']).optional(),
      })
      .strict()
      .refine(
        (jwt) =>
          !jwt.issuedAtClaim ||
          ![jwt.issuerClaim, 'exp', 'nbf'].includes(jwt.issuedAtClaim)
      )
      .optional(),
  })
  .strict()
  .refine((value) => (value.type === 'jwt_hs256') === !!value.jwt)
