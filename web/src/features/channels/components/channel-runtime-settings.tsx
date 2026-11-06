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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { ConnectionValues } from '../lib/upstream-config'

const limitFields = [
  'rpm',
  'tpm',
  'maxConcurrent',
  'queueSize',
  'queueTimeoutMs',
] as const

export function ChannelRuntimeSettings({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation()
  const { control } = useFormContext<ConnectionValues>()
  return (
    <section
      className='space-y-4'
      aria-label={t('channels.upstream.runtimeSettings')}
    >
      <div className='space-y-1'>
        <h3 className='text-sm font-semibold'>
          {t('channels.upstream.runtimeSettings')}
        </h3>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t('channels.upstream.runtimeScope')}
        </p>
      </div>
      <FormField
        control={control}
        name='streamPolicy'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('channels.upstream.streamPolicy')}</FormLabel>
            <Select
              value={field.value}
              onValueChange={field.onChange}
              disabled={disabled}
            >
              <FormControl>
                <SelectTrigger
                  className='w-full'
                  ref={field.ref}
                  onBlur={field.onBlur}
                >
                  <SelectValue>
                    {t(`channels.upstream.streamOptions.${field.value}`)}
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent aria-label={t('channels.upstream.streamPolicy')}>
                {(['unlimited', 'require', 'forbid'] as const).map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(`channels.upstream.streamOptions.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>
              {t(`channels.upstream.streamHints.${field.value}`)}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className='grid gap-4 sm:grid-cols-2'>
        {limitFields.map((name) => (
          <FormField
            key={name}
            control={control}
            name={`rateLimit.${name}`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t(`channels.upstream.limits.${name}`)}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type='number'
                    min={0}
                    step={1}
                    max={
                      name === 'queueTimeoutMs'
                        ? 9_223_372_036_854
                        : Number.MAX_SAFE_INTEGER
                    }
                    disabled={disabled}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? 0
                          : event.target.valueAsNumber
                      )
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(`channels.upstream.limitHints.${name}`)}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
      </div>
    </section>
  )
}
