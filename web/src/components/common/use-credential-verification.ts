import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  type CredentialVerifyProtocol as Protocol,
  readErrorMessage,
  classifyFetchError,
  verificationRequest,
  validVerificationResponse,
  invalidResponseKey,
} from './credential-verify-protocol'

type StepState<TOk, TErr> =
  | { phase: 'idle' }
  | { phase: 'loading' }
  | ({ phase: 'ok' } & TOk)
  | ({ phase: 'fail' } & TErr)

type ModelsState = StepState<{ models: string[] }, { message: string }>
export type VerificationState = StepState<
  { latencyMs: number; protocol: Protocol },
  { status: number | null; message: string }
>

const VERIFY_TIMEOUT_MS = 30_000
const modelsResponseSchema = z.object({
  data: z.array(z.object({ id: z.string().min(1) })),
  error: z.never().optional(),
})

export interface CredentialVerificationInput {
  keyId: number
  /** Gateway base including the /v1 suffix (same value the profiles show). */
  apiBase: string
  /** Plain key, available after the master-token unlock. */
  apiKey: string
  /** Client identity also invalidates results when two clients use one protocol. */
  contextId: string
  model: string
  protocol: Protocol
  onModelChange: (model: string) => void
}

export function useCredentialVerification({
  keyId,
  apiBase,
  apiKey,
  contextId,
  model: selectedModel,
  protocol,
  onModelChange,
}: CredentialVerificationInput) {
  const { t } = useTranslation()
  const [modelsState, setModelsState] = useState<ModelsState>({ phase: 'idle' })
  const [verificationState, setVerificationState] = useState<VerificationState>(
    { phase: 'idle' }
  )
  const request = useRef<AbortController | null>(null)
  const busy =
    modelsState.phase === 'loading' || verificationState.phase === 'loading'

  useEffect(() => {
    setVerificationState({ phase: 'idle' })
    setModelsState((current) =>
      current.phase === 'loading' ? { phase: 'idle' } : current
    )
    const controller = request.current
    request.current = null
    controller?.abort()
    return () => {
      const controller = request.current
      request.current = null
      controller?.abort()
    }
  }, [apiBase, apiKey, contextId, keyId, protocol, selectedModel])

  async function runModelsCheck() {
    if (request.current) return
    setModelsState({ phase: 'loading' })
    setVerificationState({ phase: 'idle' })
    const controller = new AbortController()
    request.current = controller
    const timer = setTimeout(() => controller.abort(), VERIFY_TIMEOUT_MS)
    try {
      const res = await fetch(`${apiBase}/models`, {
        headers: { Authorization: `Bearer ${apiKey}` },
        signal: controller.signal,
      })
      if (!res.ok) {
        const message = `HTTP ${res.status}: ${await readErrorMessage(res)}`
        if (request.current === controller)
          setModelsState({
            phase: 'fail',
            message,
          })
        if (request.current === controller) onModelChange('')
        return
      }
      const body = modelsResponseSchema.safeParse(
        await res.json().catch(() => null)
      )
      if (request.current !== controller) return
      if (!body.success) {
        setModelsState({
          phase: 'fail',
          message: `HTTP ${res.status}: ${t('connect.verify.invalidModelsResponse')}`,
        })
        onModelChange('')
        return
      }
      const models = body.data.data.map((entry) => entry.id)
      setModelsState({ phase: 'ok', models })
      onModelChange(
        selectedModel && models.includes(selectedModel)
          ? selectedModel
          : (models[0] ?? '')
      )
    } catch (error) {
      if (request.current !== controller) return
      setModelsState({
        phase: 'fail',
        message: classifyFetchError(error, t('connect.verify.timeout')).message,
      })
      onModelChange('')
    } finally {
      clearTimeout(timer)
      if (request.current === controller) {
        request.current = null
      }
    }
  }

  async function runVerificationCheck() {
    if (!selectedModel || request.current) return
    setVerificationState({ phase: 'loading' })
    const controller = new AbortController()
    request.current = controller
    const timer = setTimeout(() => controller.abort(), VERIFY_TIMEOUT_MS)
    const started = performance.now()
    const selectedProtocol = protocol
    const upstreamRequest = verificationRequest(
      selectedProtocol,
      apiBase,
      selectedModel,
      apiKey
    )
    try {
      const res = await fetch(upstreamRequest.url, {
        method: 'POST',
        headers: upstreamRequest.headers,
        body: JSON.stringify(upstreamRequest.body),
        signal: controller.signal,
      })
      const latencyMs = Math.round(performance.now() - started)
      if (!res.ok) {
        const message = await readErrorMessage(res)
        if (request.current === controller)
          setVerificationState({
            phase: 'fail',
            status: res.status,
            message,
          })
        return
      }
      const parsedBody = await res.json().catch(() => null)
      if (request.current !== controller) return
      if (!validVerificationResponse(selectedProtocol, parsedBody)) {
        setVerificationState({
          phase: 'fail',
          status: res.status,
          message: t(invalidResponseKey(selectedProtocol)),
        })
        return
      }
      setVerificationState({
        phase: 'ok',
        latencyMs,
        protocol: selectedProtocol,
      })
    } catch (error) {
      if (request.current !== controller) return
      setVerificationState({
        phase: 'fail',
        ...classifyFetchError(error, t('connect.verify.timeout')),
      })
    } finally {
      clearTimeout(timer)
      if (request.current === controller) {
        request.current = null
      }
    }
  }
  return {
    modelsState,
    verificationState,
    busy,
    runModelsCheck,
    runVerificationCheck,
    resetVerification: () => setVerificationState({ phase: 'idle' }),
  }
}
