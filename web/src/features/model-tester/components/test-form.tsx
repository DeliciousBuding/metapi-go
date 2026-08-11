// metapi-go/features/model-tester — test form (RHF + Zod + shadcn).
//
// Renders the left column of the tester: model picker (populated from the
// models marketplace via `useModels`), target protocol format, system +
// user prompts, and sampling parameters (temperature / top_p / max_tokens /
// stream). The parent owns the run/stop lifecycle; this form only emits
// validated values on submit. When `defaultModel` is provided (deep link
// from the marketplace `/models?...` → `/model-tester?model=…`) the model
// field is pre-selected as soon as the marketplace list loads.

import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
import { useModels } from '@/features/models'
import { cn } from '@/lib/utils'

import {
  TESTER_FORM_DEFAULT_VALUES,
  testerSchema,
  type TesterFormValues,
} from '../lib/tester-schema'

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

export function TestForm({
  isRunning,
  defaultModel,
  onSubmit,
  onStop,
}: TestFormProps) {
  const { t } = useTranslation()
  const modelsQuery = useModels()

  const form = useForm<TesterFormValues>({
    resolver: zodResolver(testerSchema),
    defaultValues: TESTER_FORM_DEFAULT_VALUES,
  })

  // Pre-select the model from a deep link once the marketplace list lands.
  useEffect(() => {
    if (!defaultModel) return
    const models = modelsQuery.data ?? []
    if (models.length === 0) return
    const exists = models.some((model) => model.name === defaultModel)
    if (exists) {
      form.setValue('model', defaultModel, { shouldDirty: true })
    }
  }, [defaultModel, modelsQuery.data, form])

  const handleSubmit = form.handleSubmit((values) => {
    onSubmit(values)
  })

  return (
    <Form {...form}>
      <form onSubmit={handleSubmit} className='flex h-full flex-col gap-4'>
        <FormField
          control={form.control}
          name='model'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('modelTester.form.model')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={isRunning}
              >
                <FormControl>
                  <SelectTrigger>
                    <SelectValue
                      placeholder={
                        modelsQuery.isLoading
                          ? t('modelTester.form.modelLoading')
                          : t('modelTester.form.modelPlaceholder')
                      }
                    />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {(modelsQuery.data ?? []).map((model) => (
                    <SelectItem key={model.name} value={model.name}>
                      {model.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
                  <SelectTrigger>
                    <SelectValue />
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

        <FormField
          control={form.control}
          name='systemPrompt'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('modelTester.form.systemPrompt')}</FormLabel>
              <FormControl>
                <Textarea
                  placeholder={t('modelTester.form.systemPromptPlaceholder')}
                  className='min-h-[80px] resize-y'
                  disabled={isRunning}
                  {...field}
                />
              </FormControl>
              <FormMessage />
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
                  className='min-h-[140px] resize-y'
                  disabled={isRunning}
                  autoFocus
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

        <div className='grid gap-4 sm:grid-cols-2'>
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
          <FormField
            control={form.control}
            name='stream'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                <div className='space-y-0.5'>
                  <FormLabel>{t('modelTester.form.stream')}</FormLabel>
                  <FormDescription>
                    {t('modelTester.form.streamHint')}
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
        </div>

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
      </form>
    </Form>
  )
}
