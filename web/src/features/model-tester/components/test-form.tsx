// The parent owns the run lifecycle. Only validated values leave this form.

import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ModelPicker } from '@/components/common/model-picker'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'

import { defaultTemplateStorage, loadTesterTemplates } from '../lib/templates'
import {
  TESTER_FORM_DEFAULT_VALUES,
  testerSchema,
  type TesterFormValues,
} from '../lib/tester-schema'
import type { TestTargetFormat } from '../types'
import { TestChannelFields } from './test-channel-fields'
import { TestComposer } from './test-composer'
import { TestSamplingFields } from './test-sampling-fields'
import { TestTemplateFields } from './test-template-fields'
import { useTestCatalog, rememberModel } from './use-test-catalog'

type TestFormProps = {
  isRunning: boolean
  defaultModel?: string
  defaultTargetFormat?: TestTargetFormat
  defaultChannelId?: number
  onSubmit: (
    values: TesterFormValues
  ) => void | boolean | Promise<void | boolean>
  onStop: () => void
  children?: ReactNode
  actions?: ReactNode
  hasConversation?: boolean
  onCompareChange?: (enabled: boolean) => void
}

export function TestForm({
  isRunning,
  defaultModel,
  defaultTargetFormat,
  defaultChannelId,
  onSubmit,
  onStop,
  children,
  actions,
  hasConversation = true,
  onCompareChange,
}: TestFormProps) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const form = useForm<TesterFormValues>({
    resolver: zodResolver(testerSchema),
    defaultValues: TESTER_FORM_DEFAULT_VALUES,
  })
  useEffect(() => {
    if (!settingsOpen) return
    const invalid = (
      ['channelId', 'systemPrompt', 'temperature', 'topP', 'maxTokens'] as const
    ).find((name) => form.formState.errors[name])
    if (invalid) form.setFocus(invalid)
  }, [settingsOpen, form])
  const { modelsQuery, channelsQuery } = useTestCatalog(form, {
    defaultModel,
    defaultChannelId,
    defaultTargetFormat,
    onCompareChange,
  })
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
  const wasRunning = useRef(isRunning)
  useEffect(() => {
    if (wasRunning.current && !isRunning) promptRef.current?.focus()
    wasRunning.current = isRunning
  }, [isRunning])

  const enabledParams = form.watch('enabledParams') ?? {
    temperature: false,
    topP: false,
    maxTokens: false,
  }
  const activeParamCount =
    Number(enabledParams.temperature) +
    Number(enabledParams.topP) +
    Number(enabledParams.maxTokens)

  const handleSubmit = form.handleSubmit(
    async (values) => {
      const sent = await onSubmit(values)
      if (sent !== false && !values.compareChannels) form.setValue('prompt', '')
      promptRef.current?.focus()
    },
    (errors) => {
      if (
        errors.channelId ||
        errors.channelIds ||
        errors.systemPrompt ||
        errors.temperature ||
        errors.topP ||
        errors.maxTokens
      ) {
        setSettingsOpen(true)
      }
    }
  )

  // The template picker is not part of the validated schema: selecting a
  // template fills the prompt + sampling params, then resets to the
  // placeholder so the same template can be applied again.
  const templates = useMemo(
    () => loadTesterTemplates(defaultTemplateStorage()),
    []
  )

  const applyTemplate = (templateId: string | null) => {
    if (!templateId) return
    const template = templates.find((item) => item.id === templateId)
    if (!template) return
    form.setValue('prompt', t(template.promptKey), {
      shouldDirty: true,
      shouldValidate: true,
    })
    for (const param of ['temperature', 'topP', 'maxTokens'] as const) {
      const value = template[param]
      if (value === undefined) continue
      form.setValue(param, value, { shouldDirty: true })
      form.setValue(`enabledParams.${param}`, true, { shouldDirty: true })
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={handleSubmit} className='flex h-full min-h-0 flex-col'>
        <header className='flex shrink-0 items-center justify-between gap-3 px-4 py-3 md:px-6'>
          <h1 className='sr-only'>{t('modelTester.page.title')}</h1>
          <div className='max-w-[min(70%,24rem)] min-w-0'>
            <FormField
              control={form.control}
              name='model'
              render={({ field }) => (
                <FormItem className='min-w-0'>
                  <FormLabel className='sr-only'>
                    {t('modelTester.form.model')}
                  </FormLabel>
                  <FormControl>
                    <ModelPicker
                      className='max-w-full min-w-0 overflow-hidden border-transparent bg-transparent shadow-none dark:bg-transparent'
                      models={modelsQuery.data ?? []}
                      value={field.value}
                      onValueChange={(value) => {
                        field.onChange(value)
                        rememberModel(value)
                      }}
                      disabled={isRunning || modelsQuery.isLoading}
                      placeholder={
                        modelsQuery.isLoading
                          ? t('modelTester.form.modelLoading')
                          : t('modelTester.form.modelPlaceholder')
                      }
                      onBlur={field.onBlur}
                      ref={field.ref}
                      name={field.name}
                    />
                  </FormControl>
                  {modelsQuery.isLoading && (
                    <FormDescription>
                      <Spinner className='mr-1 inline' />
                      {t('modelTester.form.modelLoading')}
                    </FormDescription>
                  )}
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          {actions}
        </header>
        <p className='text-muted-foreground shrink-0 px-4 pb-3 text-xs leading-relaxed md:px-6'>
          {t('modelTester.page.description')}
        </p>
        <Sheet open={settingsOpen} onOpenChange={setSettingsOpen}>
          <SheetContent className='gap-0 sm:max-w-md'>
            <SheetHeader>
              <SheetTitle>{t('modelTester.form.settings')}</SheetTitle>
              <SheetDescription>
                {t('modelTester.form.settingsHint')}
              </SheetDescription>
            </SheetHeader>
            <div className='space-y-5 px-4 pb-6'>
              <QueryErrorBanner
                error={channelsQuery.error as Error | null}
                messageKey='modelTester.form.channelsLoadError'
                onRetry={() => channelsQuery.refetch()}
                isRetrying={channelsQuery.isFetching}
              />
              <TestChannelFields
                channels={channelsQuery.data ?? []}
                isRunning={isRunning}
                onCompareChange={onCompareChange}
              />
              <div className='grid gap-5 border-t pt-5'>
                <TestTemplateFields
                  templates={templates}
                  applyTemplate={applyTemplate}
                  isRunning={isRunning}
                />
                <TestSamplingFields
                  enabledParams={enabledParams}
                  isRunning={isRunning}
                  activeParamCount={activeParamCount}
                />
              </div>
            </div>
          </SheetContent>
        </Sheet>
        {modelsQuery.error && (
          <div className='px-4'>
            <QueryErrorBanner
              error={modelsQuery.error as Error | null}
              messageKey='modelTester.form.modelsLoadError'
              onRetry={() => modelsQuery.refetch()}
              isRetrying={modelsQuery.isFetching}
            />
          </div>
        )}
        <div className='min-h-0 flex-1 overflow-hidden'>
          {hasConversation ? (
            children
          ) : (
            <div className='mx-auto flex h-full max-w-3xl flex-col justify-center gap-7 px-5 pb-8'>
              <div className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t('modelTester.page.title')}
                </p>
                <h2 className='text-2xl font-semibold tracking-tight sm:text-3xl'>
                  {t('modelTester.viewer.emptyTitle')}
                </h2>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {t('modelTester.viewer.emptyHint')}
                </p>
              </div>
              <div className='grid gap-2 sm:grid-cols-3'>
                {templates.slice(0, 3).map((template) => (
                  <Button
                    key={template.id}
                    type='button'
                    variant='outline'
                    className='h-auto justify-between gap-3 rounded-xl px-4 py-3 text-left font-normal'
                    onClick={() => {
                      applyTemplate(template.id)
                      promptRef.current?.focus()
                    }}
                  >
                    {t(template.labelKey)}
                    <ChevronRight className='text-muted-foreground size-3.5 shrink-0' />
                  </Button>
                ))}
              </div>
            </div>
          )}
        </div>
        <TestComposer
          promptRef={promptRef}
          isRunning={isRunning}
          onStop={onStop}
          onOpenSettings={() => setSettingsOpen(true)}
          handleSubmit={handleSubmit}
          channels={channelsQuery.data ?? []}
          activeParamCount={activeParamCount}
        />
      </form>
    </Form>
  )
}
