// metapi-go/features/model-tester — tester page (dual-column layout).
//
// Left column: `TestForm` (model picker + prompt + params). Right column:
// `TestResponseViewer` (conversation turns + live streaming content /
// reasoning / raw SSE). The page owns the conversation history
// (`messages`) and the streaming accumulation state (`content` /
// `reasoningContent`) plus the `AbortController`, and wires them to the
// `useTestModel` mutation: each parsed delta appends to the live strings
// (functional setState so rapid chunks don't clobber each other); the
// resolved `TestResponse` summary is committed to the viewer's stats bar
// when the stream closes.
//
// Session semantics: submitting appends the user prompt to `messages`
// immediately and sends the prior turns as request context; when the run
// finishes with non-empty content the assistant turn is appended. Stopped
// or failed runs are never committed (their partial output stays visible
// until the next submit). The Clear button (with confirmation) wipes the
// conversation and the live run state.
//
// A deep link from the marketplace (`/model-tester?model=…`) pre-selects
// the model in the form. The Stop button aborts the in-flight fetch; the
// resulting `AbortError` is detected and surfaced as a "stopped" state
// rather than a hard error.

import { useSearch } from '@tanstack/react-router'
import { Trash2 as TrashIcon } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { toast } from '@/lib/toast'

import { useTestModel } from '../api'
import type { TesterFormValues } from '../lib/tester-schema'
import type { ChatMessage, TestResponse, TestStreamDelta } from '../types'
import { TestForm } from './test-form'
import { TestResponseViewer } from './test-response-viewer'

let messageIdCounter = 0

function nextMessageId(): string {
  messageIdCounter += 1
  return `message-${messageIdCounter}`
}

function isAbortError(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  return (
    error.name === 'AbortError' ||
    error.message === 'This operation was aborted' ||
    error.message === 'The user aborted a request.'
  )
}

export function ModelTesterPage() {
  const { t } = useTranslation()
  // The `/model-tester` route validates `?model=` via validateSearch, so the
  // deep-link param comes from `useSearch()` (typed) rather than a raw
  // `window.location.search` read.
  const { model } = useSearch({ from: '/_authenticated/model-tester' })
  const defaultModel = model?.trim() ? model : undefined

  const testModel = useTestModel()

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [content, setContent] = useState('')
  const [reasoningContent, setReasoningContent] = useState('')
  const [response, setResponse] = useState<TestResponse | null>(null)
  const [error, setError] = useState<string | undefined>(undefined)
  const [clearDialogOpen, setClearDialogOpen] = useState(false)

  const abortControllerRef = useRef<AbortController | null>(null)

  const handleDelta = useCallback((delta: TestStreamDelta) => {
    if (delta.contentDelta) {
      setContent((prev) => prev + delta.contentDelta)
    }
    if (delta.reasoningDelta) {
      setReasoningContent((prev) => prev + delta.reasoningDelta)
    }
  }, [])

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort()
  }, [])

  const handleSubmit = useCallback(
    async (values: TesterFormValues) => {
      // The current prompt joins the conversation immediately; the prior
      // turns (before this prompt) are sent as request context.
      const history = messages
      setMessages((prev) => [
        ...prev,
        { id: nextMessageId(), role: 'user', content: values.prompt },
      ])
      // Reset previous run state.
      setContent('')
      setReasoningContent('')
      setResponse(null)
      setError(undefined)

      const controller = new AbortController()
      abortControllerRef.current = controller

      try {
        const result = await testModel.mutateAsync({
          payload: values,
          history,
          onDelta: handleDelta,
          signal: controller.signal,
        })
        setResponse(result)
        if (result.error) {
          setError(result.error)
        } else if (result.empty) {
          toast.warning(t('modelTester.toast.empty'))
        } else {
          if (result.content) {
            setMessages((prev) => [
              ...prev,
              {
                id: nextMessageId(),
                role: 'assistant',
                content: result.content,
              },
            ])
          }
          toast.success(t('modelTester.toast.succeeded'))
        }
      } catch (err) {
        if (isAbortError(err)) {
          setError(t('modelTester.error.stopped'))
          toast.info(t('modelTester.toast.stopped'))
        } else {
          const rawMessage =
            err instanceof Error ? err.message : 'modelTester.error.unknown'
          // Some thrown errors carry an i18n key (e.g.
          // `modelTester.error.notAvailable` / `sessionExpired`); translate
          // key-shaped messages so the viewer shows user-facing text.
          const message = rawMessage.startsWith('modelTester.')
            ? t(rawMessage)
            : rawMessage
          setError(message)
          toast.error(t('modelTester.toast.failed'))
        }
      } finally {
        abortControllerRef.current = null
      }
    },
    [handleDelta, testModel, t, messages]
  )

  const handleClearSession = useCallback(() => {
    setMessages([])
    setContent('')
    setReasoningContent('')
    setResponse(null)
    setError(undefined)
    setClearDialogOpen(false)
    toast.success(t('modelTester.toast.cleared'))
  }, [t])

  const isRunning = testModel.isPending

  return (
    <div className='flex h-full flex-col gap-4 p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal'>{t('modelTester.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('modelTester.page.description')}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => setClearDialogOpen(true)}
          disabled={isRunning}
        >
          <TrashIcon className='mr-1 size-3.5' />
          {t('modelTester.clear.button')}
        </Button>
      </div>

      <div className='grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-2 lg:overflow-hidden'>
        <Card className='flex h-full min-h-0 flex-col'>
          <CardContent className='flex min-h-0 flex-1 flex-col p-4'>
            <TestForm
              isRunning={isRunning}
              defaultModel={defaultModel}
              onSubmit={handleSubmit}
              onStop={handleStop}
            />
          </CardContent>
        </Card>

        <Card className='flex h-full min-h-0 flex-col'>
          <CardContent className='flex min-h-0 flex-1 flex-col p-0'>
            <TestResponseViewer
              messages={messages}
              content={content}
              reasoningContent={reasoningContent}
              isRunning={isRunning}
              response={response}
              error={error}
            />
          </CardContent>
        </Card>
      </div>

      <ConfirmDialog
        open={clearDialogOpen}
        title={t('modelTester.clear.title')}
        description={t('modelTester.clear.description')}
        confirmLabel={t('modelTester.clear.confirm')}
        cancelLabel={t('modelTester.clear.cancel')}
        destructive
        onConfirm={handleClearSession}
        onCancel={() => setClearDialogOpen(false)}
      />
    </div>
  )
}
