import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ChannelPlugin, ProviderBinding } from '@/lib/api/channel-plugins'
import type {
  ImportedCredential,
  ImportedEndpointConfig,
} from '@/lib/api/imported-upstreams'

export function ProviderSelector({
  id,
  value,
  current,
  items,
  protocols,
  credentialKind,
  blocked,
  onChange,
}: {
  id: string
  value: ProviderBinding | null
  current?: ChannelPlugin
  items: ChannelPlugin[]
  protocols: readonly (keyof ImportedEndpointConfig)[]
  credentialKind?: ImportedCredential['kind']
  blocked: boolean
  onChange: (value: ProviderBinding | null) => void
}) {
  const { t } = useTranslation()
  return (
    <>
      <label id={`${id}-label`} htmlFor={`${id}-selector`} className='text-sm'>
        {t('channels.plugins.providerSelect')}
      </label>
      <Select
        value={value?.key ?? '__native__'}
        disabled={blocked}
        onValueChange={(key) => {
          if (key === '__native__') {
            onChange(null)
            return
          }
          const item = items.find((item) => item.key === key)
          if (item?.manifest?.apiVersion !== 2 || !item.enabled) return
          onChange({
            key: item.key,
            protocols: item.manifest.protocols.filter((protocol) =>
              protocols.includes(protocol)
            ),
          })
        }}
      >
        <SelectTrigger id={`${id}-selector`} className='w-full min-w-0'>
          <SelectValue>
            {value
              ? (current?.manifest?.name ?? value.key)
              : t('channels.plugins.nativeProvider')}
          </SelectValue>
        </SelectTrigger>
        <SelectContent aria-labelledby={`${id}-label`}>
          <SelectItem value='__native__'>
            {t('channels.plugins.nativeProvider')}
          </SelectItem>
          {value && !current && (
            <SelectItem value={value.key} disabled>
              {value.key} · {t('channels.plugins.unavailable')}
            </SelectItem>
          )}
          {items.map((item) => {
            const meta = item.manifest
            if (meta?.apiVersion !== 2) return null
            const eligible =
              item.enabled &&
              !item.loadError &&
              meta.protocols.some((protocol) => protocols.includes(protocol)) &&
              (credentialKind === undefined ||
                meta.credentialKinds.some((kind) => kind === credentialKind))
            return (
              <SelectItem key={item.key} value={item.key} disabled={!eligible}>
                {meta.name}
                {!eligible && ` · ${t('channels.plugins.unavailable')}`}
              </SelectItem>
            )
          })}
        </SelectContent>
      </Select>
    </>
  )
}
