// Checks model access and one non-streaming request in the protocol the user
// explicitly selects. The request consumes upstream quota; the result links to
// this key's request logs and never auto-retries through another protocol.

import { Link } from '@tanstack/react-router'
import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

type Protocol = 'chat' | 'responses' | 'messages'

type StepState<TOk, TErr> =
  | { phase: 'idle' }
  | { phase: 'loading' }
  | ({ phase: 'ok' } & TOk)
  | ({ phase: 'fail' } & TErr)

type ModelsState = StepState<{ models: string[] }, { message: string }>
type VerificationState = StepState<
  { latencyMs: number; protocol: Protocol },
  { status: number | null; message: string }
>

const VERIFY_TIMEOUT_MS = 30_000
const VERIFY_MAX_OUTPUT_TOKENS = 32
const ANTHROPIC_VERSION = '2023-06-01'
const modelsResponseSchema = z.object({
  data: z.array(z.object({ id: z.string().min(1) })),
  error: z.never().optional(),
})
const chatResponseSchema = z.object({
  choices: z
    .array(
      z.object({
        message: z.object({
          role: z.literal('assistant'),
          content: z.string().trim().min(1),
        }),
      })
    )
    .min(1),
})
const responsesResponseSchema = z.object({
  object: z.literal('response'),
  status: z.literal('completed'),
  output: z.array(z.unknown()).min(1),
})
const messagesResponseSchema = z.object({
  type: z.literal('message'),
  role: z.literal('assistant'),
  content: z.array(z.unknown()).min(1),
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

function verificationRequest(
  protocol: Protocol,
  apiBase: string,
  model: string,
  apiKey: string
): { url: string; headers: Record<string, string>; body: unknown } {
  if (protocol === 'responses') {
    return {
      url: `${apiBase}/responses`,
      headers: {
        Authorization: `Bearer ${apiKey}`,
        'Content-Type': 'application/json',
      },
      body: {
        model,
        input: 'ping',
        max_output_tokens: VERIFY_MAX_OUTPUT_TOKENS,
        stream: false,
      },
    }
  }
  if (protocol === 'messages') {
    return {
      url: `${apiBase}/messages`,
      headers: {
        'x-api-key': apiKey,
        'anthropic-version': ANTHROPIC_VERSION,
        'Content-Type': 'application/json',
      },
      body: {
        model,
        max_tokens: VERIFY_MAX_OUTPUT_TOKENS,
        messages: [{ role: 'user', content: 'ping' }],
        stream: false,
      },
    }
  }
  return {
    url: `${apiBase}/chat/completions`,
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
    },
    body: {
      model,
      messages: [{ role: 'user', content: 'ping' }],
      max_tokens: VERIFY_MAX_OUTPUT_TOKENS,
      stream: false,
    },
  }
}

function validVerificationResponse(protocol: Protocol, body: unknown): boolean {
  if (!body || typeof body !== 'object') return false
  const error = (body as Record<string, unknown>).error
  if (error !== undefined && error !== null) return false

  if (protocol === 'responses') {
    const parsed = responsesResponseSchema.safeParse(body)
    if (!parsed.success) return false
    return parsed.data.output.some(
      (item) =>
        item !== null &&
        typeof item === 'object' &&
        (item as Record<string, unknown>).type === 'message' &&
        (item as Record<string, unknown>).role === 'assistant' &&
        Array.isArray((item as Record<string, unknown>).content) &&
        ((item as Record<string, unknown>).content as unknown[]).some(
          (part) =>
            part !== null &&
            typeof part === 'object' &&
            (part as Record<string, unknown>).type === 'output_text' &&
            typeof (part as Record<string, unknown>).text === 'string' &&
            ((part as Record<string, unknown>).text as string).trim() !== ''
        )
    )
  }
  if (protocol === 'messages') {
    const parsed = messagesResponseSchema.safeParse(body)
    if (!parsed.success) return false
    return parsed.data.content.some(
      (block) =>
        block !== null &&
        typeof block === 'object' &&
        (block as Record<string, unknown>).type === 'text' &&
        typeof (block as Record<string, unknown>).text === 'string' &&
        ((block as Record<string, unknown>).text as string).trim() !== ''
    )
  }
  return chatResponseSchema.safeParse(body).success
}

function invalidResponseKey(protocol: Protocol): string {
  if (protocol === 'responses') return 'connect.verify.invalidResponsesResponse'
  if (protocol === 'messages') return 'connect.verify.invalidMessagesResponse'
  return 'connect.verify.invalidChatResponse'
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
  const [verificationState, setVerificationState] = useState<VerificationState>(
    { phase: 'idle' }
  )
  const [protocol, setProtocol] = useState<Protocol>('chat')
  const [selectedModel, setSelectedModel] = useState('')
  const request = useRef<AbortController | null>(null)
  const protocolLabelId = useId()
  const busy =
    modelsState.phase === 'loading' || verificationState.phase === 'loading'

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
            <div className='space-y-2'>
              <div className='space-y-1'>
                <p
                  id={protocolLabelId}
                  className='text-muted-foreground text-xs font-medium'
                >
                  {t('connect.verify.protocol.label')}
                </p>
                <RadioGroup
                  className='flex flex-wrap gap-x-4 gap-y-2'
                  value={protocol}
                  disabled={busy}
                  aria-labelledby={protocolLabelId}
                  onValueChange={(value) => {
                    if (!value || value === protocol) return
                    setProtocol(value as Protocol)
                    setVerificationState({ phase: 'idle' })
                  }}
                >
                  <div className='flex items-center gap-1.5'>
                    <RadioGroupItem
                      id={`${protocolLabelId}-chat`}
                      value='chat'
                    />
                    <Label htmlFor={`${protocolLabelId}-chat`}>
                      {t('connect.verify.protocol.chat')}
                    </Label>
                  </div>
                  <div className='flex items-center gap-1.5'>
                    <RadioGroupItem
                      id={`${protocolLabelId}-responses`}
                      value='responses'
                    />
                    <Label htmlFor={`${protocolLabelId}-responses`}>
                      {t('connect.verify.protocol.responses')}
                    </Label>
                  </div>
                  <div className='flex items-center gap-1.5'>
                    <RadioGroupItem
                      id={`${protocolLabelId}-messages`}
                      value='messages'
                    />
                    <Label htmlFor={`${protocolLabelId}-messages`}>
                      {t('connect.verify.protocol.messages')}
                    </Label>
                  </div>
                </RadioGroup>
              </div>
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
                    setVerificationState({ phase: 'idle' })
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
                  onClick={() => void runVerificationCheck()}
                >
                  {verificationState.phase === 'loading' ? (
                    <Loader2 className='size-3.5 animate-spin' />
                  ) : null}
                  {t('connect.verify.testCta')}
                </Button>
              </div>
            </div>
          )}

          {verificationState.phase === 'ok' ? (
            <div className='flex flex-wrap items-center gap-2'>
              <span className='text-success-soft-fg flex items-center gap-1 text-xs'>
                <CheckCircle2 className='size-3.5' />
                {t('connect.verify.testOk', {
                  protocol: t(
                    `connect.verify.protocol.${verificationState.protocol}`
                  ),
                  ms: verificationState.latencyMs,
                })}
              </span>
              {logsLink}
            </div>
          ) : null}
          {verificationState.phase === 'fail' ? (
            <div className='space-y-2'>
              <p className='text-destructive-soft-fg flex items-start gap-1 text-xs break-all'>
                <XCircle className='mt-0.5 size-3.5 shrink-0' />
                <span>
                  {verificationState.status !== null
                    ? `HTTP ${verificationState.status}: ${verificationState.message}`
                    : verificationState.message}
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
