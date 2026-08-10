// metapi-go/features/model-tester — response viewer (right column).
//
// Renders the streaming assistant output as it arrives: a live content
// panel (auto-scrolls while streaming), a reasoning panel for
// chain-of-thought text, a raw/JSON panel of the last N SSE data events,
// and a stats bar (latency / chunk count / done sentinel / error). The
// parent owns the streaming accumulation state and passes the current
// strings down each render so the viewer re-renders on every delta.

import { Brain as BrainIcon, Code as CodeIcon } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import type { TestResponse } from '../types'

type TestResponseViewerProps = {
  content: string
  reasoningContent: string
  isRunning: boolean
  response: TestResponse | null
  error?: string
}

function formatLatency(latencyMs: number | undefined): string {
  if (latencyMs === undefined || latencyMs === null) return '—'
  return `${latencyMs}ms`
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

function ContentArea({
  text,
  placeholder,
  autoScroll,
  className,
}: {
  text: string
  placeholder: string
  autoScroll: boolean
  className?: string
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!autoScroll) return
    const node = scrollRef.current
    if (node) {
      node.scrollTop = node.scrollHeight
    }
  }, [text, autoScroll])

  return (
    <div
      ref={scrollRef}
      className={cn(
        'h-full overflow-y-auto whitespace-pre-wrap break-words p-4 text-sm leading-relaxed',
        className,
      )}
    >
      {text ? text : <span className='text-muted-foreground'>{placeholder}</span>}
    </div>
  )
}

export function TestResponseViewer({
  content,
  reasoningContent,
  isRunning,
  response,
  error,
}: TestResponseViewerProps) {
  const { t } = useTranslation()

  const hasContent = content.length > 0
  const hasReasoning = reasoningContent.length > 0
  const isEmpty =
    !isRunning && !hasContent && !hasReasoning && !response && !error

  return (
    <div className='flex h-full flex-col'>
      <div className='flex flex-wrap items-center gap-2 border-b p-3'>
        <span className='text-sm font-medium'>
          {t('modelTester.viewer.title')}
        </span>
        {isRunning && (
          <Badge variant='secondary'>
            <Spinner className='mr-1' />
            {t('modelTester.viewer.streaming')}
          </Badge>
        )}
        {response && !isRunning && (
          <>
            <Badge variant={response.empty ? 'secondary' : 'default'}>
              {response.empty
                ? t('modelTester.viewer.empty')
                : t('modelTester.viewer.done')}
            </Badge>
            <span className='text-muted-foreground text-xs'>
              {t('modelTester.viewer.latency', {
                value: formatLatency(response.latencyMs),
              })}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('modelTester.viewer.chunks', { value: response.chunks })}
            </span>
            {!response.doneReceived && (
              <span className='text-muted-foreground text-xs'>
                {t('modelTester.viewer.noDone')}
              </span>
            )}
          </>
        )}
        {error && !isRunning && (
          <Badge variant='destructive'>{t('modelTester.viewer.error')}</Badge>
        )}
      </div>

      {isEmpty ? (
        <div className='flex flex-1 items-center justify-center p-8 text-center'>
          <div className='text-muted-foreground max-w-sm text-sm'>
            {t('modelTester.viewer.emptyHint')}
          </div>
        </div>
      ) : error && !isRunning && !hasContent ? (
        <div className='flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center'>
          <Badge variant='destructive'>{t('modelTester.viewer.error')}</Badge>
          <div className='text-destructive max-w-sm text-sm break-words'>
            {error}
          </div>
        </div>
      ) : (
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
              <ContentArea
                text={content}
                placeholder={
                  isRunning
                    ? t('modelTester.viewer.awaitingContent')
                    : t('modelTester.viewer.noContent')
                }
                autoScroll={isRunning}
              />
            </ScrollArea>
          </TabsContent>

          <TabsContent value='reasoning' className='min-h-0 flex-1'>
            <ScrollArea className='h-full'>
              <ContentArea
                text={reasoningContent}
                placeholder={t('modelTester.viewer.noReasoning')}
                autoScroll={isRunning}
                className='text-muted-foreground'
              />
            </ScrollArea>
          </TabsContent>

          <TabsContent value='raw' className='min-h-0 flex-1'>
            <ScrollArea className='h-full'>
              <ContentArea
                text={prettyPrintRawEvents(response?.rawEvents ?? [])}
                placeholder={t('modelTester.viewer.noRaw')}
                autoScroll={false}
                className='font-mono text-xs'
              />
            </ScrollArea>
          </TabsContent>
        </Tabs>
      )}

      <Separator />
      <div className='flex items-center justify-between p-2 text-xs text-muted-foreground'>
        <span>{t('modelTester.viewer.hint')}</span>
        {hasContent && (
          <span className='tabular-nums'>
            {t('modelTester.viewer.contentChars', { value: content.length })}
          </span>
        )}
      </div>
    </div>
  )
}
