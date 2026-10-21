import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { isReauthRequired } from '@/lib/http-client'

export const userAdminKeys = {
  all: ['user-administration'] as const,
  users: ['user-administration', 'users'] as const,
  groups: ['user-administration', 'groups'] as const,
  budget: (id: number) => ['user-administration', 'budget', id] as const,
  pricing: (id: number) => ['user-administration', 'pricing', id] as const,
  grants: (id: number) => ['user-administration', 'grants', id] as const,
  routes: ['user-administration', 'public-routes'] as const,
}

/** Normal writes only. One-shot credentials deliberately bypass mutation caches. */
export function useAdminWrite<T>(
  operation: (input: T, token?: string) => Promise<unknown>,
  onSuccess?: () => void
) {
  const client = useQueryClient()
  const { t } = useTranslation()
  const reauth = useReauthReplay()
  const [replayPending, setReplayPending] = useState(false)
  const [replayError, setReplayError] = useState<Error | null>(null)
  const finish = async () => {
    await client.invalidateQueries({ queryKey: userAdminKeys.all })
    onSuccess?.()
  }
  const mutation = useMutation({
    mutationFn: (input: T) => operation(input),
    onSuccess: finish,
    onError: (error, input) => {
      reauth(error, t('userAdmin.reauth'), async (token) => {
        // Never put the master token in mutation variables/cache, even on a replay.
        setReplayPending(true)
        setReplayError(null)
        try {
          await operation(input, token)
          await finish()
        } catch (replayFailure) {
          if (!isReauthRequired(replayFailure))
            setReplayError(new Error('write failed'))
          throw replayFailure
        } finally {
          setReplayPending(false)
        }
      })
    },
  })
  return {
    ...mutation,
    isPending: mutation.isPending || replayPending,
    error:
      replayError ?? (isReauthRequired(mutation.error) ? null : mutation.error),
    submit: (input: T) => {
      setReplayError(null)
      mutation.mutate(input)
    },
  }
}
