import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { ChannelRow } from '@/features/channels'

import {
  filterEnabledComparisonChannels,
  formatChannelLabel,
} from '../lib/comparison-channels'
import type { TesterFormValues } from '../lib/tester-schema'
import { TARGET_FORMAT_OPTIONS } from './test-target-formats'

export function TestChannelFields({
  channels,
  isRunning,
  onCompareChange,
}: {
  channels: ChannelRow[]
  isRunning: boolean
  onCompareChange?: (enabled: boolean) => void
}) {
  const { t } = useTranslation()
  const form = useFormContext<TesterFormValues>()
  const compareChannels = form.watch('compareChannels')
  const selectedChannelIds = form.watch('channelIds') ?? []
  const comparisonChannels = filterEnabledComparisonChannels(channels)
  return (
    <>
      <FormField
        control={form.control}
        name='targetFormat'
        render={({ field }) => (
          <FormItem className='min-w-0'>
            <FormLabel>{t('modelTester.form.targetFormatLabel')}</FormLabel>
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
                  field.onChange(value === 'none' ? undefined : Number(value))
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
                        const channel = channels.find(
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
                  {channels.map((channel) => (
                    <SelectItem key={channel.id} value={String(channel.id)}>
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
          <FormItem className='flex flex-row items-center justify-between gap-3 self-end py-2 md:pb-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('modelTester.form.compareChannels')}</FormLabel>
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
    </>
  )
}
