import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import type { ChannelPlugin } from '@/lib/api/channel-plugins'

function pluginStatusKey(item: ChannelPlugin) {
  if (item.loadError) return 'channels.plugins.unavailable'
  return item.enabled ? 'channels.plugins.enabled' : 'channels.plugins.disabled'
}

export function ChannelPluginRow({
  item,
  togglePending,
  onToggle,
  onEdit,
  onRemove,
}: {
  item: ChannelPlugin
  togglePending: boolean
  onToggle: (enabled: boolean) => void
  onEdit: () => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  return (
    <section className='flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3'>
      <div className='min-w-0 space-y-1'>
        <h3 className='text-sm font-medium'>
          {item.manifest?.name ?? item.key}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {item.manifest &&
            t(
              item.manifest.apiVersion === 2
                ? 'channels.plugins.providerType'
                : 'channels.plugins.wireType'
            )}{' '}
          · {item.key} ·{' '}
          {item.manifest?.version ?? t('channels.plugins.unavailable')} ·{' '}
          {t('channels.plugins.revision', { revision: item.revision })}
        </p>
        <p className='text-muted-foreground text-xs'>
          {item.manifest?.protocols
            .map((protocol) => t(`channels.capabilities.names.${protocol}`))
            .join(' · ')}
        </p>
      </div>
      <div className='flex flex-wrap items-center gap-3'>
        <Badge variant={item.enabled ? 'secondary' : 'outline'}>
          {t(pluginStatusKey(item))}
        </Badge>
        <Switch
          aria-label={t('channels.plugins.enableName', {
            name: item.manifest?.name ?? item.key,
          })}
          checked={item.enabled}
          disabled={togglePending || !!item.loadError}
          onCheckedChange={onToggle}
        />
        <Button variant='outline' size='sm' onClick={onEdit}>
          {t('channels.plugins.edit')}
        </Button>
        <Button
          variant='ghost'
          size='sm'
          disabled={item.enabled || togglePending}
          onClick={onRemove}
        >
          {t('channels.plugins.remove')}
        </Button>
      </div>
    </section>
  )
}
