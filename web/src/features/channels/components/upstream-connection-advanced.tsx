import { ChevronDown, SlidersHorizontal } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'

import type { ConnectionValues } from '../lib/upstream-config'
import { ChannelRuntimeSettings } from './channel-runtime-settings'
import { UpstreamRequestConfig } from './upstream-request-config'

export function UpstreamConnectionAdvanced({
  detail,
}: {
  detail: ImportedUpstreamDetail
}) {
  const { t } = useTranslation()
  const form = useFormContext<ConnectionValues>()
  const pending = form.formState.isSubmitting
  const [advanced, setAdvanced] = useState(false)
  const advancedInvalid = [
    'streamPolicy',
    'rateLimit',
    'channelProxy',
    'customHeaders',
    'paramOverride',
    'pluginBindings',
    'providerBinding',
  ].some((key) => !!form.formState.errors[key as keyof ConnectionValues])
  const submitCount = form.formState.submitCount
  useEffect(() => {
    if (advancedInvalid) setAdvanced(true)
  }, [advancedInvalid, submitCount])
  return (
    <Collapsible
      open={advanced}
      onOpenChange={setAdvanced}
      className='rounded-xl border'
    >
      <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center gap-3 rounded-xl px-4 py-3 text-left text-sm font-medium focus-visible:outline-2'>
        <SlidersHorizontal
          className='text-muted-foreground size-4'
          aria-hidden='true'
        />
        <span className='flex-1'>{t('channels.upstream.advanced')}</span>
        <ChevronDown
          className='size-4 transition-transform group-data-[panel-open]:rotate-180'
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent keepMounted className='space-y-5 border-t p-4'>
        <ChannelRuntimeSettings disabled={pending} />
        <FormField
          control={form.control}
          name='useSystemProxy'
          render={({ field }) => (
            <FormItem className='flex items-center gap-3'>
              <FormLabel>{t('channels.upstream.systemProxy')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={pending}
                />
              </FormControl>
            </FormItem>
          )}
        />
        <UpstreamRequestConfig detail={detail} />
      </CollapsibleContent>
    </Collapsible>
  )
}
