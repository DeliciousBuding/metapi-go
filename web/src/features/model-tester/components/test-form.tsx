// metapi-go/features/model-tester — test form (RHF + Zod + shadcn).
//
// Owns the playground configuration and bottom composer. A template picker fills
// the prompt + sampling parameters from the localStorage template library),
// model picker (populated from the models marketplace via `useModels`), a
// channel picker (populated from the channels list via `useChannels`, targeting
// the forced-channel sync harness), target protocol format, system + user
// prompts, and sampling parameters (temperature / top_p / max_tokens). The
// parent owns the run/stop lifecycle; this form only emits validated values on
// submit. When `defaultModel` is provided (deep link from the marketplace
// `/models?...` → `/model-tester?model=…`) the model field is pre-selected
// as soon as the marketplace list loads. Without a deep link, restore the last
// valid model chosen in this browser; never invent a model absent from the
// current catalog. Message content stays in page memory, not browser storage.

import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowUp, Square, SlidersHorizontal, ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ModelPicker } from '@/components/common/model-picker'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { Slider } from '@/components/ui/slider'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useChannels } from '@/features/channels'
import { useModels } from '@/features/models'

import {
  filterEnabledComparisonChannels,
  formatChannelLabel,
} from '../lib/comparison-channels'
import { defaultTemplateStorage, loadTesterTemplates } from '../lib/templates'
import {
  TESTER_FORM_DEFAULT_VALUES,
  testerSchema,
  type TesterFormValues,
} from '../lib/tester-schema'

type TestFormProps = {
  isRunning: boolean
  defaultModel?: string
  onSubmit: (
    values: TesterFormValues
  ) => void | boolean | Promise<void | boolean>
  onStop: () => void
  children?: ReactNode
  actions?: ReactNode
  hasConversation?: boolean
  onCompareChange?: (enabled: boolean) => void
}

const TARGET_FORMAT_OPTIONS: Array<{
  value: TesterFormValues['targetFormat']
  labelKey: string
}> = [
  { value: 'openai', labelKey: 'modelTester.form.targetFormat.openai' },
  { value: 'claude', labelKey: 'modelTester.form.targetFormat.claude' },
  { value: 'responses', labelKey: 'modelTester.form.targetFormat.responses' },
  { value: 'gemini', labelKey: 'modelTester.form.targetFormat.gemini' },
]

const LAST_MODEL_KEY = 'metapi-model-tester-last-model'

function readLastModel(): string {
  try {
    return window.localStorage.getItem(LAST_MODEL_KEY)?.trim() ?? ''
  } catch {
    return '' // Storage may be disabled by the browser.
  }
}

function rememberModel(model: string): void {
  try {
    window.localStorage.setItem(LAST_MODEL_KEY, model)
  } catch {
    // The live selection still works when local persistence is unavailable.
  }
}

export function TestForm({
  isRunning,
  defaultModel,
  onSubmit,
  onStop,
  children,
  actions,
  hasConversation = true,
  onCompareChange,
}: TestFormProps) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const modelsQuery = useModels()
  const channelsQuery = useChannels()

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
  const appliedDeepLink = useRef<string | null>(null)
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
  const wasRunning = useRef(isRunning)
  useEffect(() => {
    if (wasRunning.current && !isRunning) promptRef.current?.focus()
    wasRunning.current = isRunning
  }, [isRunning])

  const compareChannels = form.watch('compareChannels')
  const selectedChannelIds = form.watch('channelIds') ?? []
  const comparisonChannels = useMemo(
    () => filterEnabledComparisonChannels(channelsQuery.data ?? []),
    [channelsQuery.data]
  )

  // A deep link wins over local preference; once applied, a background model
  // refetch must not reset a model the user chose afterward. The saved model
  // is only restored while the field is empty and remains in the live catalog.
  useEffect(() => {
    const models = modelsQuery.data ?? []
    if (models.length === 0) return
    if (defaultModel) {
      if (
        appliedDeepLink.current !== defaultModel &&
        models.some((model) => model.name === defaultModel)
      ) {
        form.setValue('model', defaultModel, { shouldDirty: true })
        appliedDeepLink.current = defaultModel
      }
      return
    }
    if (form.getValues('model')) return
    const saved = readLastModel()
    if (saved && models.some((model) => model.name === saved)) {
      form.setValue('model', saved)
    }
  }, [defaultModel, modelsQuery.data, form])

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
  const [selectedTemplateId, setSelectedTemplateId] = useState('')

  const applyTemplate = (templateId: string | null) => {
    if (!templateId) return
    const template = templates.find((item) => item.id === templateId)
    if (!template) return
    form.setValue('prompt', t(template.promptKey), {
      shouldDirty: true,
      shouldValidate: true,
    })
    if (template.temperature !== undefined) {
      form.setValue('temperature', template.temperature, {
        shouldDirty: true,
      })
    }
    if (template.topP !== undefined) {
      form.setValue('topP', template.topP, { shouldDirty: true })
    }
    if (template.maxTokens !== undefined) {
      form.setValue('maxTokens', template.maxTokens, { shouldDirty: true })
    }
    setSelectedTemplateId('')
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
              <FormField
                control={form.control}
                name='targetFormat'
                render={({ field }) => (
                  <FormItem className='min-w-0'>
                    <FormLabel>
                      {t('modelTester.form.targetFormatLabel')}
                    </FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      disabled={isRunning}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full max-w-full min-w-0 overflow-hidden'>
                          <SelectValue>
                            {(selected) => {
                              const option = TARGET_FORMAT_OPTIONS.find(
                                (item) => item.value === selected
                              )
                              return option
                                ? t(option.labelKey)
                                : String(selected ?? '')
                            }}
                          </SelectValue>
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {TARGET_FORMAT_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {!compareChannels && (
                <FormField
                  control={form.control}
                  name='channelId'
                  render={({ field }) => (
                    <FormItem className='min-w-0'>
                      <FormLabel>{t('modelTester.form.channel')}</FormLabel>
                      <Select
                        value={field.value ? String(field.value) : 'none'}
                        onValueChange={(value) =>
                          field.onChange(
                            value === 'none' ? undefined : Number(value)
                          )
                        }
                        disabled={isRunning}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full max-w-full min-w-0 overflow-hidden'>
                            <SelectValue className='min-w-0 overflow-hidden'>
                              {(selected) => {
                                if (!selected || selected === 'none') {
                                  return t('modelTester.form.channelNoneForced')
                                }
                                const channel = (channelsQuery.data ?? []).find(
                                  (item) => String(item.id) === selected
                                )
                                return channel
                                  ? formatChannelLabel(channel)
                                  : String(selected)
                              }}
                            </SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value='none'>
                            {t('modelTester.form.channelNoneForced')}
                          </SelectItem>
                          {(channelsQuery.data ?? []).map((channel) => (
                            <SelectItem
                              key={channel.id}
                              value={String(channel.id)}
                            >
                              {formatChannelLabel(channel)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {compareChannels && (
                <FormField
                  control={form.control}
                  name='channelIds'
                  render={() => (
                    <FormItem className='min-w-0'>
                      <FormLabel>
                        {t('modelTester.form.compareChannelsLabel')}
                      </FormLabel>
                      <div className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'>
                        {comparisonChannels.map((channel) => {
                          const checked = selectedChannelIds.includes(
                            channel.id
                          )
                          return (
                            <label
                              key={channel.id}
                              className='hover:bg-muted flex items-center gap-2 rounded px-2 py-1'
                            >
                              <Checkbox
                                checked={checked}
                                onCheckedChange={(value) => {
                                  const next = value
                                    ? [...selectedChannelIds, channel.id]
                                    : selectedChannelIds.filter(
                                        (id) => id !== channel.id
                                      )
                                  form.setValue('channelIds', next, {
                                    shouldValidate: true,
                                  })
                                }}
                                disabled={isRunning}
                              />
                              <span className='truncate text-sm'>
                                {formatChannelLabel(channel)}
                              </span>
                            </label>
                          )
                        })}
                      </div>
                      <FormDescription>
                        {t('modelTester.form.compareChannelsCount', {
                          count: selectedChannelIds.length,
                        })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='compareChannels'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between gap-3 self-end py-2 md:pb-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>
                        {t('modelTester.form.compareChannels')}
                      </FormLabel>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={(checked) => {
                          field.onChange(checked)
                          onCompareChange?.(checked)
                        }}
                        disabled={isRunning}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <div className='grid gap-5 border-t pt-5'>
                <FormItem>
                  <FormLabel>{t('modelTester.template.label')}</FormLabel>
                  <Select
                    value={selectedTemplateId}
                    onValueChange={applyTemplate}
                    disabled={isRunning}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue
                          placeholder={t('modelTester.template.placeholder')}
                        />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {templates.map((template) => (
                        <SelectItem key={template.id} value={template.id}>
                          {t(template.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('modelTester.template.hint')}
                  </FormDescription>
                </FormItem>

                <FormField
                  control={form.control}
                  name='systemPrompt'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('modelTester.form.systemPrompt')}
                      </FormLabel>
                      <FormControl>
                        <Textarea
                          placeholder={t(
                            'modelTester.form.systemPromptPlaceholder'
                          )}
                          className='min-h-20 resize-y'
                          disabled={isRunning}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='grid gap-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='temperature'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('modelTester.form.temperature')}
                          <span className='text-muted-foreground ml-1 tabular-nums'>
                            {field.value.toFixed(2)}
                          </span>
                        </FormLabel>
                        <FormControl>
                          <Slider
                            aria-label={t('modelTester.form.temperature')}
                            value={[field.value]}
                            min={0}
                            max={2}
                            step={0.05}
                            disabled={isRunning}
                            onValueChange={(values) => {
                              const next = Array.isArray(values)
                                ? (values[0] ?? 0)
                                : values
                              field.onChange(next)
                            }}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='topP'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('modelTester.form.topP')}
                          <span className='text-muted-foreground ml-1 tabular-nums'>
                            {field.value.toFixed(2)}
                          </span>
                        </FormLabel>
                        <FormControl>
                          <Slider
                            aria-label={t('modelTester.form.topP')}
                            value={[field.value]}
                            min={0}
                            max={1}
                            step={0.05}
                            disabled={isRunning}
                            onValueChange={(values) => {
                              const next = Array.isArray(values)
                                ? (values[0] ?? 0)
                                : values
                              field.onChange(next)
                            }}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='maxTokens'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('modelTester.form.maxTokens')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          step={1}
                          value={field.value}
                          onChange={(event) =>
                            field.onChange(
                              Number.isNaN(event.target.valueAsNumber)
                                ? 0
                                : event.target.valueAsNumber
                            )
                          }
                          onBlur={field.onBlur}
                          name={field.name}
                          ref={field.ref}
                          disabled={isRunning}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('modelTester.form.maxTokensHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
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
                  className='text-muted-foreground max-w-48 min-w-0'
                  aria-label={t('modelTester.form.settings')}
                  onClick={() => setSettingsOpen(true)}
                >
                  <SlidersHorizontal className='size-4 shrink-0' />
                  <span className='truncate'>
                    {compareChannels
                      ? t('modelTester.form.compareChannels')
                      : (channelsQuery.data ?? []).find(
                          (channel) => channel.id === form.watch('channelId')
                        )?.name || t('modelTester.form.channelNoneForced')}
                  </span>
                </Button>
                <span className='text-muted-foreground truncate text-xs'>
                  {compareChannels
                    ? t('modelTester.form.compareChannelsCount', {
                        count: selectedChannelIds.length,
                      })
                    : t(
                        TARGET_FORMAT_OPTIONS.find(
                          (option) =>
                            option.value === form.watch('targetFormat')
                        )!.labelKey
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
                  <span className='sr-only'>
                    {t('modelTester.form.submit')}
                  </span>
                </Button>
              )}
            </div>
          </div>
          <p className='text-muted-foreground mt-2 hidden text-center text-xs sm:block'>
            {t('modelTester.form.promptSubmitHint')}
          </p>
        </div>
      </form>
    </Form>
  )
}
