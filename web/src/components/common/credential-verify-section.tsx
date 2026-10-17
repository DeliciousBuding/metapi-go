// Checks model access and a non-streaming Chat request with the unlocked key.
// Chat consumes upstream quota; results link to this key's request logs.

import { Link } from '@tanstack/react-router'
import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

type StepState<TOk, TErr> =
  | { phase: 'idle' }
  | { phase: 'loading' }
  | ({ phase: 'ok' } & TOk)
  | ({ phase: 'fail' } & TErr)

type ModelsState = StepState<{ models: string[] }, { message: string }>
type ChatState = StepState<
  { latencyMs: number },
  { status: number | null; message: string }
>

const VERIFY_TIMEOUT_MS = 30_000
const modelsResponseSchema = z.object({
  data: z.array(z.object({ id: z.string().min(1) })),
  error: z.never().optional(),
})
const chatResponseSchema = z.object({
  choices: z
    .array(z.object({ message: z.object({ role: z.literal('assistant') }) }))
    .min(1),
  error: z.never().optional(),
})

/** Parsed upstream/gateway error body, truncated — never the key. */
async function readErrorMessage(res: Response): Promise<string> {
  const text = (await res.text()).slice(0, 500)
  try {
    const parsed = JSON.parse(text) as {
      error?: { message?: unknown }
      message?: unknown
    }
    const message = parsed.error?.message ?? parsed.message
    if (typeof message === 'string' && message.trim() !== '') return message
  } catch {
    // not JSON — fall through to the raw text
  }
  return text
}

function classifyFetchError(
  error: unknown,
  timeoutCopy: string
): { status: number | null; message: string } {
  if (error instanceof DOMException && error.name === 'AbortError') {
    return { status: null, message: timeoutCopy }
  }
  return {
    status: null,
    message: error instanceof Error ? error.message : String(error),
  }
}

export function CredentialVerifySection({
  keyId,
  apiBase,
  apiKey,
}: {
  keyId: number
  /** Gateway base including the /v1 suffix (same value the profiles show). */
  apiBase: string
  /** Plain key, available after the master-token unlock. */
  apiKey: string
}) {
  const { t } = useTranslation()
  const [modelsState, setModelsState] = useState<ModelsState>({ phase: 'idle' })
  const [chatState, setChatState] = useState<ChatState>({ phase: 'idle' })
  const [selectedModel, setSelectedModel] = useState('')
  const request = useRef<AbortController | null>(null)
  const busy = modelsState.phase === 'loading' || chatState.phase === 'loading'

  useEffect(
    () => () => {
      const controller = request.current
      request.current = null
      controller?.abort()
    },
    []
  )

  async function runModelsCheck() {
    if (request.current) return
    setModelsState({ phase: 'loading' })
    setChatState({ phase: 'idle' })
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
        return
      }
      const models = body.data.data.map((entry) => entry.id)
      setModelsState({ phase: 'ok', models })
      setSelectedModel((current) =>
        current && models.includes(current) ? current : (models[0] ?? '')
      )
    } catch (error) {
      if (request.current !== controller) return
      setModelsState({
        phase: 'fail',
        message: classifyFetchError(error, t('connect.verify.timeout')).message,
      })
    } finally {
      clearTimeout(timer)
      if (request.current === controller) request.current = null
    }
  }

  async function runChatCheck() {
    if (!selectedModel || request.current) return
    setChatState({ phase: 'loading' })
    const controller = new AbortController()
    request.current = controller
    const timer = setTimeout(() => controller.abort(), VERIFY_TIMEOUT_MS)
    const started = performance.now()
    try {
      const res = await fetch(`${apiBase}/chat/completions`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${apiKey}`,
          'Content-Type': 'application/json',
        },
        // Keep the connectivity check short.
        body: JSON.stringify({
          model: selectedModel,
          messages: [{ role: 'user', content: 'ping' }],
          max_tokens: 1,
          stream: false,
        }),
        signal: controller.signal,
      })
      const latencyMs = Math.round(performance.now() - started)
      if (!res.ok) {
        const message = await readErrorMessage(res)
        if (request.current === controller)
          setChatState({
            phase: 'fail',
            status: res.status,
            message,
          })
        return
      }
      const body = chatResponseSchema.safeParse(
        await res.json().catch(() => null)
      )
      if (request.current !== controller) return
      if (!body.success) {
        setChatState({
          phase: 'fail',
          status: res.status,
          message: t('connect.verify.invalidChatResponse'),
        })
        return
      }
      setChatState({ phase: 'ok', latencyMs })
    } catch (error) {
      if (request.current !== controller) return
      setChatState({
        phase: 'fail',
        ...classifyFetchError(error, t('connect.verify.timeout')),
      })
    } finally {
      clearTimeout(timer)
      if (request.current === controller) request.current = null
    }
  }

  const logsLink = (
    <Button
      variant='outline'
      size='sm'
      render={<Link to='/proxy-logs' search={{ downstreamKeyId: keyId }} />}
    >
      {t('connect.verify.viewLogs')}
    </Button>
  )

  return (
    <div className='space-y-2 rounded-lg border p-3'>
      <p className='text-muted-foreground text-xs font-medium'>
        {t('connect.verify.title')}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t('connect.verify.description')}
      </p>

      <div className='flex flex-wrap items-center gap-2'>
        <Button
          size='sm'
          variant='outline'
          disabled={!apiKey || busy}
          onClick={() => void runModelsCheck()}
        >
          {modelsState.phase === 'loading' ? (
            <Loader2 className='size-3.5 animate-spin' />
          ) : null}
          {t('connect.verify.modelsCta')}
        </Button>
        {modelsState.phase === 'ok' ? (
          <span className='text-muted-foreground flex items-center gap-1 text-xs'>
            <CheckCircle2 className='text-success-soft-fg size-3.5' />
            {t('connect.verify.modelsOk', { count: modelsState.models.length })}
          </span>
        ) : null}
      </div>

      {modelsState.phase === 'ok' ? (
        <div className='space-y-2 border-t pt-2'>
          {modelsState.models.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('connect.verify.noModels')}
            </p>
          ) : (
            <div className='flex flex-wrap items-center gap-2'>
              <Select
                items={modelsState.models.map((model) => ({
                  value: model,
                  label: model,
                }))}
                value={selectedModel}
                disabled={busy}
                onValueChange={(value) => {
                  setSelectedModel(value ?? '')
                  setChatState({ phase: 'idle' })
                }}
              >
                <SelectTrigger
                  size='sm'
                  aria-label={t('connect.verify.modelAria')}
                  className='w-56'
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modelsState.models.map((model) => (
                    <SelectItem key={model} value={model}>
                      {model}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                size='sm'
                disabled={!selectedModel || busy}
                onClick={() => void runChatCheck()}
              >
                {chatState.phase === 'loading' ? (
                  <Loader2 className='size-3.5 animate-spin' />
                ) : null}
                {t('connect.verify.chatCta')}
              </Button>
            </div>
          )}

          {chatState.phase === 'ok' ? (
            <div className='flex flex-wrap items-center gap-2'>
              <span className='text-success-soft-fg flex items-center gap-1 text-xs'>
                <CheckCircle2 className='size-3.5' />
                {t('connect.verify.chatOk', { ms: chatState.latencyMs })}
              </span>
              {logsLink}
            </div>
          ) : null}
          {chatState.phase === 'fail' ? (
            <div className='space-y-2'>
              <p className='text-destructive-soft-fg flex items-start gap-1 text-xs break-all'>
                <XCircle className='mt-0.5 size-3.5 shrink-0' />
                <span>
                  {chatState.status !== null
                    ? `HTTP ${chatState.status}: ${chatState.message}`
                    : chatState.message}
                </span>
              </p>
              {logsLink}
            </div>
          ) : null}
        </div>
      ) : null}

      {modelsState.phase === 'fail' ? (
        <div className='space-y-2 border-t pt-2'>
          <p className='text-destructive-soft-fg flex items-start gap-1 text-xs break-all'>
            <XCircle className='mt-0.5 size-3.5 shrink-0' />
            <span>{modelsState.message}</span>
          </p>
          {logsLink}
        </div>
      ) : null}
    </div>
  )
}
