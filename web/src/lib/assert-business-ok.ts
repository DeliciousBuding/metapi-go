// metapi-go/lib — shared mutation-envelope guard.
//
// Backend write endpoints fail in one of two shapes:
//   • legacy 200 `{"success": false, "message": "..."}` — the http-client
//     response interceptor already toasts it; re-throwing here flips the
//     useMutation into its error state and makes mutateAsync reject;
//   • modern 4xx `{"error": "..."}` — axios already rejects those in the
//     http-client error interceptor, so they never reach this helper. The
//     `error` branch below is defensive coverage for error-shaped bodies
//     that arrive with a 2xx status.
// Success payloads pass through untouched (envelope or plain data).

import i18n from '@/i18n/config'

type BusinessEnvelope = {
  success?: unknown
  message?: unknown
  error?: unknown
  data?: unknown
}

function resolveErrorText(error: unknown): string | null {
  if (typeof error === 'string' && error) return error
  if (error && typeof error === 'object') {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message) return message
  }
  return null
}

export function assertBusinessOk<T>(
  result: unknown,
  fallbackI18nKey: string
): T {
  const envelope = result as BusinessEnvelope

  if (envelope && typeof envelope.success === 'boolean' && !envelope.success) {
    throw new Error(
      typeof envelope.message === 'string'
        ? envelope.message
        : i18n.t(fallbackI18nKey)
    )
  }

  if (envelope && envelope.success !== true) {
    const errorText = resolveErrorText(envelope.error)
    if (errorText) throw new Error(errorText)
  }

  return (result as T) ?? (envelope?.data as T)
}
