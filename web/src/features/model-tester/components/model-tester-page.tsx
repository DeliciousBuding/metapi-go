// metapi-go/features/model-tester — tester page (dual-column layout).
//
// Left column: `TestForm` (model picker + prompt + params). Right column:
// `TestResponseViewer` (streaming content / reasoning / raw SSE). The page
// owns the streaming accumulation state (`content` / `reasoningContent`)
// and the `AbortController`, and wires them to the `useTestModel` mutation:
// each parsed delta appends to the live strings (functional setState so
// rapid chunks don't clobber each other); the resolved `TestResponse`
// summary is committed to the viewer's stats bar when the stream closes.
//
// A deep link from the marketplace (`/model-tester?model=…`) pre-selects
// the model in the form. The Stop button aborts the in-flight fetch; the
// resulting `AbortError` is detected and surfaced as a "stopped" state
// rather than a hard error.

import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Card, CardContent } from '@/components/ui/card'

import { useTestModel } from '../api'
import type { TesterFormValues } from '../lib/tester-schema'
import type { TestResponse, TestStreamDelta } from '../types'
import { TestForm } from './test-form'
import { TestResponseViewer } from './test-response-viewer'

function isAbortError(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  return (
    error.name === 'AbortError' ||
    error.message === 'This operation was aborted' ||
    error.message === 'The user aborted a request.'
  )
}

function readDefaultModelFromUrl(): string | undefined {
  if (typeof window === 'undefined') return undefined
  const params = new URLSearchParams(window.location.search)
  const model = params.get('model')
  return model && model.trim().length > 0 ? model : undefined
}

export function ModelTesterPage() {
  const { t } = useTranslation()
  // The `/model-tester` route file does not land its own validateSearch yet,
  // so the page reads the `model` deep-link param directly from the URL (the
  // same pattern the sites page uses for its search state).
  const defaultModel = readDefaultModelFromUrl()

  const testModel = useTestModel()

  const [content, setContent] = useState('')
  const [reasoningContent, setReasoningContent] = useState('')
  const [response, setResponse] = useState<TestResponse | null>(null)
  const [error, setError] = useState<string | undefined>(undefined)

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
          onDelta: handleDelta,
          signal: controller.signal,
        })
        setResponse(result)
        if (result.error) {
          setError(result.error)
        } else if (result.empty) {
          toast.warning(t('modelTester.toast.empty'))
        } else {
          toast.success(t('modelTester.toast.succeeded'))
        }
      } catch (err) {
        if (isAbortError(err)) {
          setError(t('modelTester.error.stopped'))
          toast.info(t('modelTester.toast.stopped'))
        } else {
          const message =
            err instanceof Error ? err.message : t('modelTester.error.unknown')
          setError(message)
          toast.error(t('modelTester.toast.failed'))
        }
      } finally {
        abortControllerRef.current = null
      }
    },
    [handleDelta, testModel, t]
  )

  const isRunning = testModel.isPending

  return (
    <div className='grid h-full grid-cols-1 gap-4 lg:grid-cols-2 lg:overflow-hidden'>
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
            content={content}
            reasoningContent={reasoningContent}
            isRunning={isRunning}
            response={response}
            error={error}
          />
        </CardContent>
      </Card>
    </div>
  )
}
