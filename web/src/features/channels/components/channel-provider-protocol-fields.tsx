import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import type {
  ProviderBinding,
  ChannelPluginManifest,
} from '@/lib/api/channel-plugins'

import { PluginOptionField } from './channel-plugin-bindings'

export type ProviderSettingsProps = {
  value: ProviderBinding
  manifest?: Extract<ChannelPluginManifest, { apiVersion: 2 }>
  blocked: boolean
  update: (patch: Partial<ProviderBinding>) => void
}

export function ProviderProtocolFields({
  value,
  manifest,
  protocols,
  blocked,
  update,
}: ProviderSettingsProps & {
  protocols: readonly ProviderBinding['protocols'][number][]
}) {
  const { t } = useTranslation()
  const available =
    manifest?.protocols.filter((protocol) => protocols.includes(protocol)) ?? []
  return (
    <>
      {manifest?.authentication && (
        <div className='space-y-1 text-sm'>
          <p>
            {t('channels.plugins.hostAuthentication', {
              type: manifest.authentication.type,
              header: manifest.authentication.header,
            })}
          </p>
          {manifest.authentication.jwt && (
            <p className='text-muted-foreground text-xs'>
              {t('channels.plugins.hostJWT', {
                separator: manifest.authentication.jwt.keySeparator,
                seconds: manifest.authentication.jwt.ttlSeconds,
              })}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('channels.plugins.hostAuthenticationHint')}
          </p>
        </div>
      )}
      <div className='flex flex-wrap gap-3'>
        {[...new Set([...available, ...value.protocols])].map((protocol) => (
          <label key={protocol} className='flex items-center gap-2 text-sm'>
            <Checkbox
              checked={value.protocols.includes(protocol)}
              disabled={
                blocked ||
                (!value.protocols.includes(protocol) &&
                  !available.includes(protocol)) ||
                (value.protocols.length === 1 &&
                  value.protocols.includes(protocol))
              }
              onCheckedChange={(checked) =>
                update({
                  protocols: checked
                    ? [...value.protocols, protocol]
                    : value.protocols.filter((item) => item !== protocol),
                })
              }
            />
            {t(`channels.capabilities.names.${protocol}`)}
            {!protocols.includes(protocol) && (
              <span className='text-destructive'>
                {t('channels.plugins.endpointMissing')}
              </span>
            )}
          </label>
        ))}
      </div>
      {manifest?.options?.map((option) => (
        <PluginOptionField
          key={option.key}
          option={option}
          value={value.options?.[option.key] ?? option.default}
          disabled={blocked}
          onChange={(next) =>
            update({ options: { ...value.options, [option.key]: next } })
          }
        />
      ))}
    </>
  )
}
