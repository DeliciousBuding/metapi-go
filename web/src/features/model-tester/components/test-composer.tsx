import { ArrowUp, Square, SlidersHorizontal } from 'lucide-react'
import type { RefObject } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'
import type { ChannelRow } from '@/features/channels'

import type { TesterFormValues } from '../lib/tester-schema'
import { TARGET_FORMAT_OPTIONS } from './test-target-formats'

export function TestComposer({
  promptRef,
  isRunning,
  onStop,
  onOpenSettings,
  handleSubmit,
  channels,
  activeParamCount,
}: {
  promptRef: RefObject<HTMLTextAreaElement | null>
  isRunning: boolean
  onStop: () => void
  onOpenSettings: () => void
  handleSubmit: () => Promise<void>
  channels: ChannelRow[]
  activeParamCount: number
}) {
  const { t } = useTranslation()
  const form = useFormContext<TesterFormValues>()
  const compareChannels = form.watch('compareChannels')
  const selectedChannelIds = form.watch('channelIds') ?? []
  return (
    <div className='bg-background shrink-0 px-3 pb-3 md:px-6 md:pb-5'>
      <div className='bg-muted/40 focus-within:ring-ring/20 border-border/70 mx-auto flex w-full max-w-3xl flex-col gap-2 rounded-2xl border p-3 shadow-sm focus-within:ring-2 sm:p-4'>
        <FormField
          control={form.control}
          name='prompt'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='sr-only'>
                {t('modelTester.form.prompt')}
              </FormLabel>
              <FormControl>
                <Textarea
                  placeholder={t('modelTester.form.promptPlaceholder')}
                  className='max-h-40 min-h-14 resize-none border-0 bg-transparent p-0 shadow-none focus-visible:ring-0 dark:bg-transparent'
                  disabled={isRunning}
                  onKeyDown={(event) => {
                    // ⌘/Ctrl+Enter sends without leaving the textarea, the
                    // same shortcut operators expect from chat surfaces.
                    if (
                      event.key !== 'Enter' ||
                      (!event.metaKey && !event.ctrlKey) ||
                      event.nativeEvent.isComposing ||
                      isRunning
                    ) {
                      return
                    }
                    event.preventDefault()
                    void handleSubmit()
                  }}
                  {...field}
                  ref={(node) => {
                    field.ref(node)
                    promptRef.current = node
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className='flex items-center justify-between gap-2'>
          <div className='flex min-w-0 items-center gap-2'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='text-muted-foreground max-w-56 min-w-0'
              aria-label={t('modelTester.form.settings')}
              onClick={onOpenSettings}
            >
              <SlidersHorizontal className='size-4 shrink-0' />
              <span className='truncate'>
                {compareChannels
                  ? t('modelTester.form.compareChannels')
                  : channels.find(
                      (channel) => channel.id === form.watch('channelId')
                    )?.name || t('modelTester.form.channelNoneForced')}
              </span>
              {activeParamCount > 0 && (
                <Badge
                  variant='secondary'
                  data-testid='active-params-badge'
                  title={t('modelTester.form.activeParamsBadge', {
                    count: activeParamCount,
                  })}
                  className='text-2xs h-4 px-1 tabular-nums'
                >
                  {activeParamCount}
                </Badge>
              )}
            </Button>
            <span className='text-muted-foreground truncate text-xs'>
              {compareChannels
                ? t('modelTester.form.compareChannelsCount', {
                    count: selectedChannelIds.length,
                  })
                : t(
                    TARGET_FORMAT_OPTIONS.find(
                      (option) => option.value === form.watch('targetFormat')
                    )?.labelKey ?? 'modelTester.form.targetFormat.openai'
                  )}
            </span>
          </div>
          {isRunning ? (
            <Button
              type='button'
              variant='outline'
              onClick={onStop}
              size='icon'
              className='rounded-full'
              aria-label={t('modelTester.form.stop')}
            >
              <Square className='size-3.5' />
              <span className='sr-only'>{t('modelTester.form.stop')}</span>
            </Button>
          ) : (
            <Button
              type='submit'
              size='icon'
              className='rounded-full'
              aria-label={t('modelTester.form.submit')}
            >
              <ArrowUp className='size-4' />
              <span className='sr-only'>{t('modelTester.form.submit')}</span>
            </Button>
          )}
        </div>
      </div>
      <p className='text-muted-foreground mt-2 hidden text-center text-xs sm:block'>
        {t('modelTester.form.promptSubmitHint')}
      </p>
    </div>
  )
}
