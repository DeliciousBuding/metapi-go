// metapi-go/features/model-tester — test form (RHF + Zod + shadcn).
//
// Renders the left column of the tester: a local template picker (fills
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
// current catalog.

import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
import { Slider } from '@/components/ui/slider'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useChannels } from '@/features/channels'
import { useModels } from '@/features/models'
import { cn } from '@/lib/utils'

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
import { ModelPicker } from './model-picker'

type TestFormProps = {
  isRunning: boolean
  defaultModel?: string
  onSubmit: (values: TesterFormValues) => void
  onStop: () => void
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
}: TestFormProps) {
  const { t } = useTranslation()
  const advancedRef = useRef<HTMLDetailsElement>(null)
  const modelsQuery = useModels()
  const channelsQuery = useChannels()

  const form = useForm<TesterFormValues>({
    resolver: zodResolver(testerSchema),
    defaultValues: TESTER_FORM_DEFAULT_VALUES,
  })
  const appliedDeepLink = useRef<string | null>(null)

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
    (values) => {
      onSubmit(values)
    },
    (errors) => {
      if (
        advancedRef.current &&
        (errors.systemPrompt ||
          errors.temperature ||
          errors.topP ||
          errors.maxTokens)
      ) {
        advancedRef.current.open = true
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
      <form onSubmit={handleSubmit} className='flex flex-col gap-4'>
        <QueryErrorBanner
          error={modelsQuery.error as Error | null}
          messageKey='modelTester.form.modelsLoadError'
          onRetry={() => modelsQuery.refetch()}
          isRetrying={modelsQuery.isFetching}
        />
        <QueryErrorBanner
          error={channelsQuery.error as Error | null}
          messageKey='modelTester.form.channelsLoadError'
          onRetry={() => channelsQuery.refetch()}
          isRetrying={channelsQuery.isFetching}
        />
        <div className='grid items-start gap-3 sm:grid-cols-[minmax(0,1fr)_140px]'>
          <FormField
            control={form.control}
            name='model'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('modelTester.form.model')}</FormLabel>
                <FormControl>
                  <ModelPicker
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

          <FormField
            control={form.control}
            name='targetFormat'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('modelTester.form.targetFormatLabel')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={isRunning}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
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
        </div>
        {!compareChannels && (
          <FormField
            control={form.control}
            name='channelId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('modelTester.form.channel')}</FormLabel>
                <Select
                  value={field.value ? String(field.value) : 'none'}
                  onValueChange={(value) =>
                    field.onChange(value === 'none' ? undefined : Number(value))
                  }
                  disabled={isRunning}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue>
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
                      <SelectItem key={channel.id} value={String(channel.id)}>
                        {formatChannelLabel(channel)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('modelTester.form.channelHintForced')}
                </FormDescription>
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
              <FormItem>
                <FormLabel>
                  {t('modelTester.form.compareChannelsLabel')}
                </FormLabel>
                <div className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'>
                  {comparisonChannels.map((channel) => {
                    const checked = selectedChannelIds.includes(channel.id)
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
            <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
              <div className='space-y-0.5'>
                <FormLabel>{t('modelTester.form.compareChannels')}</FormLabel>
                <FormDescription>
                  {t('modelTester.form.compareChannelsHint')}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={isRunning}
                />
              </FormControl>
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='prompt'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('modelTester.form.prompt')}</FormLabel>
              <FormControl>
                <Textarea
                  placeholder={t('modelTester.form.promptPlaceholder')}
                  className='min-h-32 resize-y'
                  disabled={isRunning}
                  autoFocus
                  onKeyDown={(event) => {
                    // ⌘/Ctrl+Enter sends without leaving the textarea, the
                    // same shortcut operators expect from chat surfaces.
                    if (
                      event.key !== 'Enter' ||
                      (!event.metaKey && !event.ctrlKey) ||
                      isRunning
                    ) {
                      return
                    }
                    event.preventDefault()
                    void handleSubmit()
                  }}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t('modelTester.form.promptSubmitHint')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className={cn('mt-auto flex gap-2')}>
          {isRunning ? (
            <Button
              type='button'
              variant='destructive'
              onClick={onStop}
              className='flex-1'
            >
              {t('modelTester.form.stop')}
            </Button>
          ) : (
            <Button type='submit' className='flex-1'>
              {t('modelTester.form.submit')}
            </Button>
          )}
        </div>
        <details ref={advancedRef} className='group rounded-lg border'>
          <summary className='focus-visible:outline-ring cursor-pointer px-3 py-3 text-sm font-medium'>
            {t('modelTester.form.advanced')}
          </summary>
          <div className='flex flex-col gap-4 border-t p-3'>
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
                  <FormLabel>{t('modelTester.form.systemPrompt')}</FormLabel>
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
        </details>
      </form>
    </Form>
  )
}
