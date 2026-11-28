import { ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { Button, buttonVariants } from '@/components/ui/button'
import type { ConnectionTemplate } from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

export function SiteConnectionTemplateList({
  templates,
  selectedId,
  onSelect,
}: {
  templates: ConnectionTemplate[]
  selectedId?: string
  onSelect: (template: ConnectionTemplate) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='grid max-h-64 grid-cols-2 gap-2 overflow-y-auto pr-1 max-[420px]:grid-cols-1'>
      {templates.map((template) => {
        let detail = t('sites.templates.notAvailable')
        if (template.available) {
          detail =
            template.group === 'api'
              ? `${template.protocols?.map((protocol) => t(`sites.templates.protocols.${protocol}`)).join(' / ')} · API Key`
              : t(`sites.templates.auth.${template.group}`)
        }
        const contents = (
          <>
            <span
              className='bg-muted/50 flex size-8 shrink-0 items-center justify-center rounded-md'
              aria-hidden='true'
            >
              <BrandGlyph
                icon={template.icon}
                size={20}
                fallbackText={template.name}
              />
            </span>
            <span className='min-w-0 flex-1 space-y-1 text-left'>
              <span className='block truncate text-sm font-medium'>
                {template.name}
              </span>
              <span className='text-muted-foreground block text-xs font-normal'>
                {detail}
              </span>
            </span>
            {template.group === 'oauth' && (
              <ArrowUpRight className='size-3.5 shrink-0' aria-hidden='true' />
            )}
          </>
        )
        const className = cn(
          'h-auto min-h-18 justify-start gap-2.5 whitespace-normal rounded-lg px-3 py-2.5',
          selectedId === template.id && 'border-primary bg-primary/5'
        )
        if (template.group === 'oauth' && template.available) {
          return (
            <a
              key={template.id}
              className={cn(buttonVariants({ variant: 'outline' }), className)}
              href='/oauth'
              target='_blank'
              rel='noopener noreferrer'
              aria-label={t('sites.templates.openOAuth', {
                name: template.name,
              })}
            >
              {contents}
            </a>
          )
        }
        return (
          <Button
            key={template.id}
            type='button'
            variant='outline'
            className={className}
            disabled={!template.available}
            aria-label={
              template.available
                ? t('sites.templates.use', {
                    name: template.label ?? template.name,
                  })
                : t('sites.templates.unavailable', {
                    name: template.label ?? template.name,
                  })
            }
            onClick={() => onSelect(template)}
          >
            {contents}
          </Button>
        )
      })}
    </div>
  )
}
