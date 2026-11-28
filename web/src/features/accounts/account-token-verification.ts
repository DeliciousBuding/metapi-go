import { useMutation } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'

import type { VerifyTokenResult } from './api'

// ---------------------------------------------------------------------------
// useVerifyAccountToken — POST /api/accounts/verify-token (inline credential
// check before save). Passes skipErrorHandler so the shared axios layer does
// not toast a global error: the form renders the verified/failed state inline
// instead. The backend returns non-2xx {"error"} on failure, so assertBusinessOk
// only ever sees success envelopes here.
// ---------------------------------------------------------------------------

export function useVerifyAccountToken() {
  return useMutation({
    mutationFn: async (payload: {
      siteId: number
      accessToken: string
      platformUserId?: number
      proxyUrl?: string
      credentialMode: 'session' | 'apikey'
    }) => {
      const result = await api.verifyToken(payload, { skipErrorHandler: true })
      return assertBusinessOk<VerifyTokenResult>(
        result,
        'accounts.verify.failed'
      )
    },
  })
}
