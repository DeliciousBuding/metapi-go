import { ArrowDown, MessageSquare, Copy, Check } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatInt, formatLatency } from '@/lib/format'
import { toast } from '@/lib/toast'

import type { ChatMessage } from '../types'
import { Message, MessageContent, MessageResponse } from './message'

type TestResponseViewerProps = {
  messages: ChatMessage[]
  isRunning: boolean
}

function prettyPrintRawEvents(rawEvents: string[]): string {
  return rawEvents
    .map((raw) => {
      try {
        return JSON.stringify(JSON.parse(raw) as unknown, null, 2)
      } catch {
        return raw
      }
    })
    .join('\n\n')
}

function ConversationArea({ children }: { children: ReactNode }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [following, setFollowing] = useState(true)
  const { t } = useTranslation()

  useEffect(() => {
    if (!following) return
    const node = scrollRef.current
    if (node) node.scrollTop = node.scrollHeight
  }, [children, following])

  return (
    <div className='relative h-full min-h-0'>
      <div
        ref={scrollRef}
        role='log'
        aria-label={t('modelTester.viewer.title')}
        onScroll={(event) => {
          const node = event.currentTarget
          setFollowing(
            node.scrollHeight - node.scrollTop - node.clientHeight < 48
          )
        }}
        className='h-full overflow-y-auto overscroll-contain'
      >
        <div className='mx-auto flex min-h-full w-full max-w-3xl flex-col gap-8 px-4 pt-6 pb-10 sm:px-6'>
          {children}
        </div>
      </div>
      {!following && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='absolute right-4 bottom-3 shadow-sm'
          onClick={() => {
            const node = scrollRef.current
            if (node) node.scrollTop = node.scrollHeight
            setFollowing(true)
          }}
        >
          <ArrowDown className='size-4' />
          {t('modelTester.viewer.latest')}
        </Button>
      )}
    </div>
  )
}

function TurnBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const response = message.response
  const hasUsage =
    response &&
    (response.promptTokens !== undefined ||
      response.completionTokens !== undefined ||
      response.totalTokens !== undefined)

  return (
    <Message from={message.role}>
      <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs'>
        <span className='font-medium'>
          {message.role === 'user'
            ? t('modelTester.viewer.roleYou')
            : t('modelTester.viewer.roleAssistant')}
        </span>
        {message.model && <span className='truncate'>{message.model}</span>}
      </div>
      <MessageContent>
        {message.role === 'user' ? (
          <div className='whitespace-pre-wrap'>{message.content}</div>
        ) : (
          <>
            {response?.reasoningContent && (
              <details className='text-muted-foreground border-l-2 py-1 pl-3'>
                <summary className='cursor-pointer text-xs font-medium'>
                  {t('modelTester.viewer.tabReasoning')}
                </summary>
                <div className='pt-3 whitespace-pre-wrap'>
                  {response.reasoningContent}
                </div>
              </details>
            )}
            {message.content ? (
              <MessageResponse>{message.content}</MessageResponse>
            ) : (
              !message.error && (
                <span className='text-muted-foreground'>
                  {t('modelTester.viewer.noContent')}
                </span>
              )
            )}
            {message.error && (
              <p role='alert' className='text-destructive break-words'>
                {message.error}
              </p>
            )}
          </>
        )}
      </MessageContent>
      {message.role === 'assistant' && (
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs tabular-nums'>
          {message.content && (
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t(
                copied ? 'modelTester.viewer.copied' : 'modelTester.viewer.copy'
              )}
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(message.content)
                  setCopied(true)
                } catch {
                  toast.error(t('modelTester.viewer.copyFailed'))
                }
              }}
            >
              {copied ? (
                <Check className='size-3.5' />
              ) : (
                <Copy className='size-3.5' />
              )}
            </Button>
          )}
          {response && (
            <>
              <span>
                {t('modelTester.viewer.latency', {
                  value: formatLatency(response.latencyMs),
                })}
              </span>
              {hasUsage && (
                <span>
                  {t('modelTester.viewer.usage', {
                    total: formatInt(response.totalTokens ?? null),
                    prompt: formatInt(response.promptTokens ?? null),
                    completion: formatInt(response.completionTokens ?? null),
                  })}
                </span>
              )}
              <details className='max-w-full'>
                <summary className='w-fit cursor-pointer py-1'>
                  {t('modelTester.viewer.details')}
                </summary>
                <div className='mt-2 space-y-2 border-l-2 pl-3'>
                  <p>
                    HTTP {response.statusCode}
                    {!response.doneReceived && !response.error
                      ? ` · ${t('modelTester.viewer.noDone')}`
                      : ''}
                  </p>
                  {response.bodyTruncated && (
                    <p className='text-warning-foreground'>
                      {t('modelTester.viewer.truncated')}
                    </p>
                  )}
                  <pre className='bg-muted max-h-64 overflow-auto rounded p-3 font-mono text-xs'>
                    {prettyPrintRawEvents(response.rawEvents) ||
                      t('modelTester.viewer.noRaw')}
                  </pre>
                </div>
              </details>
            </>
          )}
        </div>
      )}
    </Message>
  )
}

export function TestResponseViewer(props: TestResponseViewerProps) {
  const { t } = useTranslation()
  const lastMessage = props.messages.at(-1)
  const hasLiveRound = props.isRunning && lastMessage?.role !== 'assistant'
  const isEmpty = props.messages.length === 0 && !props.isRunning

  return (
    <ConversationArea>
      {isEmpty ? (
        <div className='text-muted-foreground my-auto flex flex-col items-center gap-3 px-2 py-8 text-center'>
          <MessageSquare className='text-primary size-8' aria-hidden='true' />
          <h2 className='text-foreground text-base font-medium'>
            {t('modelTester.viewer.emptyTitle')}
          </h2>
          <p className='max-w-sm text-sm leading-relaxed'>
            {t('modelTester.viewer.emptyHint')}
          </p>
        </div>
      ) : (
        <>
          {props.messages.map((message) => (
            <TurnBlock key={message.id} message={message} />
          ))}
          {hasLiveRound && (
            <div
              className='text-muted-foreground flex items-center gap-2 text-sm'
              role='status'
            >
              <Spinner className='size-4' />
              {t('modelTester.viewer.awaitingContent')}
            </div>
          )}
        </>
      )}
    </ConversationArea>
  )
}
