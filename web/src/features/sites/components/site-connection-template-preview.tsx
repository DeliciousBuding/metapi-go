import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { PlatformBadge } from '@/components/common/platform-badge'
import {
  getConnectionPresetNameKey,
  type ConnectionTemplate,
} from '@/lib/platform-catalog'

export function SiteConnectionTemplatePreview({
  selected,
  platform,
  url,
}: {
  selected: ConnectionTemplate
  platform: string
  url: string
}) {
  const { t } = useTranslation()
  const nameKey = getConnectionPresetNameKey(selected.id)
  const selectedName = nameKey ? t(nameKey) : selected.name
  return (
    <div className='bg-muted/20 flex items-start gap-3 rounded-lg border p-3'>
      <span aria-hidden='true' className='pt-0.5'>
        <BrandGlyph
          icon={selected.icon}
          size={22}
          fallbackText={selected.name}
        />
      </span>
      <div className='min-w-0 flex-1 space-y-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='font-medium'>{selectedName}</span>
          <PlatformBadge platform={platform} />
        </div>
        <p className='text-muted-foreground text-xs break-all'>
          {url || t('sites.templates.enterAddress')}
        </p>
      </div>
      <Check className='text-primary size-4 shrink-0' aria-hidden='true' />
    </div>
  )
}
