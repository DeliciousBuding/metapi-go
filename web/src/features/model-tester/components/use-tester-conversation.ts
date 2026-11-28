import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toast } from '@/lib/toast'

import { useTestModel, isAbortError } from '../api'
import type { TesterFormValues } from '../lib/tester-schema'
import type { ChatMessage } from '../types'

let messageIdCounter = 0

function nextMessageId(): string {
  messageIdCounter += 1
  return `message-${messageIdCounter}`
}

export function useTesterConversation() {
  const { t } = useTranslation()
  const testModel = useTestModel()
  const [messages, setMessages] = useState<ChatMessage[]>([])

  async function submit(values: TesterFormValues, controller: AbortController) {
    const history = messages
    // Single runs join the conversation immediately; comparisons are
    // measurements and do not create an unmatched user-only turn.
    setMessages((prev) => [
      ...prev,
      { id: nextMessageId(), role: 'user', content: values.prompt },
    ])

    try {
      const result = await testModel.mutateAsync({
        payload: values,
        history,
        signal: controller.signal,
      })
      if (controller.signal.aborted)
        throw new DOMException('This operation was aborted', 'AbortError')
      setMessages((prev) => [
        ...prev,
        {
          id: nextMessageId(),
          role: 'assistant',
          content: result.content,
          response: result,
          error: result.error,
          model: values.model,
        },
      ])
      if (!result.error && result.empty) {
        toast.warning(t('modelTester.toast.empty'))
      }
      return !result.error && !result.empty
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        setMessages((prev) => [
          ...prev,
          {
            id: nextMessageId(),
            role: 'assistant',
            content: '',
            error: t('modelTester.error.stopped'),
          },
        ])
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
        setMessages((prev) => [
          ...prev,
          {
            id: nextMessageId(),
            role: 'assistant',
            content: '',
            error: message,
          },
        ])
        toast.error(t('modelTester.toast.failed'))
      }
      return false
    }
  }

  return {
    messages,
    submit,
    isPending: testModel.isPending,
    clear: () => setMessages([]),
  }
}
