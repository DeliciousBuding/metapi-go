import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'

import type { TesterFormValues } from '../lib/tester-schema'

type SamplingProps = {
  isRunning: boolean
  enabledParams: NonNullable<TesterFormValues['enabledParams']>
}
function SamplingSlider({
  name,
  isRunning,
  enabledParams,
}: SamplingProps & { name: 'temperature' | 'topP' }) {
  const { t } = useTranslation()
  const form = useFormContext<TesterFormValues>()
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <div className='flex items-center justify-between gap-2'>
            <FormLabel>
              {t(`modelTester.form.${name}`)}
              <span className='text-muted-foreground ml-1 tabular-nums'>
                {enabledParams[name] ? field.value.toFixed(2) : '—'}
              </span>
            </FormLabel>
            <FormField
              control={form.control}
              name={`enabledParams.${name}`}
              render={({ field: toggleField }) => (
                <FormItem className='space-y-0'>
                  <FormControl>
                    <Switch
                      aria-label={t('modelTester.form.paramEnabled', {
                        param: t(`modelTester.form.${name}`),
                      })}
                      checked={Boolean(toggleField.value)}
                      disabled={isRunning}
                      onCheckedChange={toggleField.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>
          <FormControl>
            <Slider
              aria-label={t(`modelTester.form.${name}`)}
              value={[field.value]}
              min={0}
              max={name === 'temperature' ? 2 : 1}
              step={0.05}
              disabled={isRunning || !enabledParams[name]}
              onValueChange={(values) => {
                const next = Array.isArray(values) ? (values[0] ?? 0) : values
                field.onChange(next)
              }}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
export function TestSamplingFields({
  isRunning,
  enabledParams,
  activeParamCount,
}: SamplingProps & { activeParamCount: number }) {
  const { t } = useTranslation()
  const form = useFormContext<TesterFormValues>()
  return (
    <>
      <div className='grid gap-4 sm:grid-cols-2'>
        <SamplingSlider
          name='temperature'
          isRunning={isRunning}
          enabledParams={enabledParams}
        />
        <SamplingSlider
          name='topP'
          isRunning={isRunning}
          enabledParams={enabledParams}
        />
      </div>

      <FormField
        control={form.control}
        name='maxTokens'
        render={({ field }) => (
          <FormItem>
            <div className='flex items-center justify-between gap-2'>
              <FormLabel>{t('modelTester.form.maxTokens')}</FormLabel>
              <FormField
                control={form.control}
                name='enabledParams.maxTokens'
                render={({ field: toggleField }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('modelTester.form.paramEnabled', {
                          param: t('modelTester.form.maxTokens'),
                        })}
                        checked={Boolean(toggleField.value)}
                        disabled={isRunning}
                        onCheckedChange={toggleField.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
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
                disabled={isRunning || !enabledParams.maxTokens}
              />
            </FormControl>
            <FormDescription>
              {enabledParams.maxTokens
                ? t('modelTester.form.maxTokensHint')
                : t('modelTester.form.paramDisabledHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {activeParamCount < 3 && (
        <p className='text-muted-foreground text-xs'>
          {t('modelTester.form.paramDisabledHint')}
        </p>
      )}
    </>
  )
}
