import type { ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { ProviderSettingsProps } from './channel-provider-protocol-fields'

export function ProviderImageQueries({
  value,
  manifest,
  blocked,
  update,
  id,
  newChannel,
  invalid,
}: ProviderSettingsProps & {
  id: string
  newChannel?: boolean
  invalid?: ComponentProps<'input'>['aria-invalid']
}) {
  const { t } = useTranslation()
  return value.protocols.map((protocol) => {
    if (
      protocol !== 'imageGeneration' &&
      protocol !== 'imageEdit' &&
      protocol !== 'imageVariation' &&
      protocol !== 'modelscopeImageGeneration'
    )
      return null
    const task = manifest?.imageTasks?.[protocol]
    const query = value.imageQueries?.[protocol]
    if (!task && !query) return null
    const queryId = `${id}-${protocol}-image-query`
    const auth = query?.auth ?? task?.query.auth ?? 'bearer'
    return (
      <section key={protocol} className='space-y-2 border-t pt-3'>
        <label htmlFor={queryId} className='text-sm'>
          {t('channels.plugins.imageQueryUrl', {
            protocol: t(`channels.capabilities.names.${protocol}`),
          })}
        </label>
        <Input
          id={queryId}
          value={query?.url ?? ''}
          disabled={blocked}
          aria-invalid={invalid}
          onChange={(event) =>
            update({
              imageQueries: {
                ...value.imageQueries,
                [protocol]: { url: event.target.value, auth },
              },
            })
          }
        />
        <label
          id={`${queryId}-auth-label`}
          htmlFor={`${queryId}-auth`}
          className='text-sm'
        >
          {t('channels.plugins.imageQueryAuth')}
        </label>
        <Select
          value={auth}
          disabled={blocked}
          onValueChange={(next) => {
            if (
              next === 'bearer' ||
              next === 'x-api-key' ||
              next === 'x-goog-api-key' ||
              next === 'none'
            )
              update({
                imageQueries: {
                  ...value.imageQueries,
                  [protocol]: { url: query?.url ?? '', auth: next },
                },
              })
          }}
        >
          <SelectTrigger id={`${queryId}-auth`} aria-invalid={invalid}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent aria-labelledby={`${queryId}-auth-label`}>
            {(['bearer', 'x-api-key', 'x-goog-api-key', 'none'] as const).map(
              (method) => (
                <SelectItem key={method} value={method}>
                  {method}
                </SelectItem>
              )
            )}
          </SelectContent>
        </Select>
        <p className='text-muted-foreground text-sm'>
          {newChannel && task && !query
            ? t('channels.plugins.imageQueryDefault', {
                path: task.query.path,
              })
            : t('channels.plugins.imageQueryScope')}
        </p>
      </section>
    )
  })
}
