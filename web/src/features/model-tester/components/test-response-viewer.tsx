// metapi-go/features/model-tester — response viewer (right column).
//
// Renders the tester conversation: every committed turn (user prompt +
// assistant response) in order, with the live round streaming at the
// bottom while a run is in flight. The reasoning and raw/JSON tabs keep
// showing the current (or last) run's artifacts, and the stats bar reports
// the last run's latency / chunk count / done sentinel / error. The parent
// owns the streaming accumulation state and passes the current strings down
// each render so the viewer re-renders on every delta.

import { Brain as BrainIcon, Code as CodeIcon } from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ChatMessage, TestResponse } from '../types'

type TestResponseViewerProps = {
  messages: ChatMessage[]
  content: string
  reasoningContent: string
  isRunning: boolean
  response: TestResponse | null
  error?: string
}

function prettyPrintRawEvents(rawEvents: string[]): string {
  const pretty: string[] = []
  for (const raw of rawEvents) {
    try {
      const parsed = JSON.parse(raw) as unknown
      pretty.push(JSON.stringify(parsed, null, 2))
    } catch {
      pretty.push(raw)
    }
  }
  return pretty.join('\n\n')
}

function ContentArea(props: {
  text: string
  placeholder: string
  autoScroll: boolean
  className?: string
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!props.autoScroll) return
    const node = scrollRef.current
    if (node) {
      node.scrollTop = node.scrollHeight
    }
  }, [props.text, props.autoScroll])

  return (
    <div
      ref={scrollRef}
      className={cn(
        'h-full overflow-y-auto whitespace-pre-wrap break-words p-4 text-sm leading-relaxed',
        props.className
      )}
    >
      {props.text ? (
        props.text
      ) : (
        <span className='text-muted-foreground'>{props.placeholder}</span>
      )}
    </div>
  )
}

function ConversationArea(props: { autoScroll: boolean; children: ReactNode }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!props.autoScroll) return
    const node = scrollRef.current
    if (node) {
      node.scrollTop = node.scrollHeight
    }
  })

  return (
    <div
      ref={scrollRef}
      className='flex h-full flex-col gap-4 overflow-y-auto p-4'
    >
      {props.children}
    </div>
  )
}

function TurnBlock(props: {
  role: ChatMessage['role']
  text: string
  isStreaming?: boolean
  error?: string
  placeholder?: string
}) {
  const { t } = useTranslation()
  const roleLabel =
    props.role === 'user'
      ? t('modelTester.viewer.roleYou')
      : t('modelTester.viewer.roleAssistant')

  let body: ReactNode
  if (props.text.length > 0) {
    body = (
      <div className='text-sm leading-relaxed break-words whitespace-pre-wrap'>
        {props.text}
      </div>
    )
  } else if (props.error) {
    body = (
      <div className='text-destructive text-sm break-words'>{props.error}</div>
    )
  } else {
    body = (
      <div className='text-muted-foreground text-sm'>
        {props.placeholder ?? ''}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-1.5'>
      <div className='flex items-center gap-2'>
        <Badge variant={props.role === 'user' ? 'secondary' : 'default'}>
          {roleLabel}
        </Badge>
        {props.isStreaming ? <Spinner className='size-3.5' /> : null}
      </div>
      {body}
    </div>
  )
}

export function TestResponseViewer(props: TestResponseViewerProps) {
  const { t } = useTranslation()

  const hasContent = props.content.length > 0
  const hasReasoning = props.reasoningContent.length > 0
  const lastMessage = props.messages.at(-1)
  const hasLiveRound = props.isRunning || lastMessage?.role === 'user'
  const isEmpty =
    !props.isRunning &&
    !hasContent &&
    !hasReasoning &&
    !props.response &&
    !props.error &&
    props.messages.length === 0

  let body: ReactNode
  if (isEmpty) {
    body = (
      <div className='flex flex-1 items-center justify-center p-8 text-center'>
        <div className='text-muted-foreground max-w-sm text-sm'>
          {t('modelTester.viewer.emptyHint')}
        </div>
      </div>
    )
  } else if (
    props.error &&
    !props.isRunning &&
    !hasContent &&
    props.messages.length === 0
  ) {
    body = (
      <div className='flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center'>
        <Badge variant='destructive'>{t('modelTester.viewer.error')}</Badge>
        <div className='text-destructive max-w-sm text-sm break-words'>
          {props.error}
        </div>
      </div>
    )
  } else {
    body = (
      <Tabs defaultValue='content' className='flex min-h-0 flex-1 flex-col'>
        <TabsList className='mx-3 mt-3 w-fit'>
          <TabsTrigger value='content'>
            {t('modelTester.viewer.tabContent')}
          </TabsTrigger>
          <TabsTrigger value='reasoning'>
            <BrainIcon className='mr-1 size-3.5' />
            {t('modelTester.viewer.tabReasoning')}
            {hasReasoning && (
              <span className='text-muted-foreground ml-1 text-xs'>●</span>
            )}
          </TabsTrigger>
          <TabsTrigger value='raw'>
            <CodeIcon className='mr-1 size-3.5' />
            {t('modelTester.viewer.tabRaw')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='content' className='min-h-0 flex-1'>
          <ScrollArea className='h-full'>
            <ConversationArea autoScroll={props.isRunning}>
              {props.messages.map((message) => (
                <TurnBlock
                  key={message.id}
                  role={message.role}
                  text={message.content}
                />
              ))}
              {hasLiveRound ? (
                <TurnBlock
                  role='assistant'
                  text={props.content}
                  isStreaming={props.isRunning}
                  error={props.isRunning ? undefined : props.error}
                  placeholder={
                    props.isRunning
                      ? t('modelTester.viewer.awaitingContent')
                      : t('modelTester.viewer.noContent')
                  }
                />
              ) : null}
            </ConversationArea>
          </ScrollArea>
        </TabsContent>

        <TabsContent value='reasoning' className='min-h-0 flex-1'>
          <ScrollArea className='h-full'>
            <ContentArea
              text={props.reasoningContent}
              placeholder={t('modelTester.viewer.noReasoning')}
              autoScroll={props.isRunning}
              className='text-muted-foreground'
            />
          </ScrollArea>
        </TabsContent>

        <TabsContent value='raw' className='min-h-0 flex-1'>
          <ScrollArea className='h-full'>
            <ContentArea
              text={prettyPrintRawEvents(props.response?.rawEvents ?? [])}
              placeholder={t('modelTester.viewer.noRaw')}
              autoScroll={false}
              className='font-mono text-xs'
            />
          </ScrollArea>
        </TabsContent>
      </Tabs>
    )
  }

  return (
    <div className='flex h-full flex-col'>
      <div className='flex flex-wrap items-center gap-2 border-b p-3'>
        <span className='text-sm font-medium'>
          {t('modelTester.viewer.title')}
        </span>
        {props.isRunning && (
          <Badge variant='secondary'>
            <Spinner className='mr-1' />
            {t('modelTester.viewer.streaming')}
          </Badge>
        )}
        {props.response && !props.isRunning && (
          <>
            <Badge variant={props.response.empty ? 'secondary' : 'default'}>
              {props.response.empty
                ? t('modelTester.viewer.empty')
                : t('modelTester.viewer.done')}
            </Badge>
            <span className='text-muted-foreground text-xs'>
              {t('modelTester.viewer.latency', {
                value: formatLatency(props.response.latencyMs),
              })}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('modelTester.viewer.chunks', { value: props.response.chunks })}
            </span>
            {!props.response.doneReceived && (
              <span className='text-muted-foreground text-xs'>
                {t('modelTester.viewer.noDone')}
              </span>
            )}
          </>
        )}
        {props.error && !props.isRunning && (
          <Badge variant='destructive'>{t('modelTester.viewer.error')}</Badge>
        )}
      </div>

      {body}

      <Separator />
      <div className='text-muted-foreground flex items-center justify-between p-2 text-xs'>
        <span>{t('modelTester.viewer.hint')}</span>
        {hasContent && (
          <span className='tabular-nums'>
            {t('modelTester.viewer.contentChars', {
              value: props.content.length,
            })}
          </span>
        )}
      </div>
    </div>
  )
}
