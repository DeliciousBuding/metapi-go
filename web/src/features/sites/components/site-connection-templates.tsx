import { ArrowUpRight, ChevronDown, Check } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { PlatformBadge } from '@/components/common/platform-badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  CONNECTION_TEMPLATES,
  type ConnectionTemplate,
  type PlatformDefinition,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

type Props = {
  selected: ConnectionTemplate | null
  feedback: string
  url: string
  platform: string
  onSelect: (template: ConnectionTemplate) => boolean
}

export function SiteConnectionTemplates(props: Props) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(true)
  const [category, setCategory] = useState<PlatformDefinition['group']>('api')
  const [search, setSearch] = useState('')
  const toggleRef = useRef<HTMLButtonElement>(null)
  const query = search.trim().toLowerCase()

  function select(template: ConnectionTemplate) {
    if (!props.onSelect(template)) return
    setExpanded(false)
    setSearch('')
    toggleRef.current?.focus()
  }

  return (
    <section aria-label={t('sites.templates.title')} className='space-y-3'>
      <div className='flex items-center justify-between gap-3'>
        <h3 className='text-sm font-semibold'>{t('sites.templates.title')}</h3>
        <Button
          ref={toggleRef}
          type='button'
          variant='ghost'
          size='xs'
          onClick={() => setExpanded(!expanded)}
          aria-expanded={expanded}
        >
          {expanded ? t('sites.templates.manual') : t('sites.templates.change')}
          <ChevronDown
            className={cn(
              'size-3.5 transition-transform',
              expanded && 'rotate-180'
            )}
            aria-hidden='true'
          />
        </Button>
      </div>
      {!expanded && props.selected && (
        <div className='bg-muted/20 flex items-start gap-3 rounded-lg border p-3'>
          <span aria-hidden='true' className='pt-0.5'>
            <BrandGlyph
              icon={props.selected.icon}
              size={22}
              fallbackText={props.selected.name}
            />
          </span>
          <div className='min-w-0 flex-1 space-y-1'>
            <div className='flex flex-wrap items-center gap-2'>
              <span className='font-medium'>{props.selected.name}</span>
              <PlatformBadge platform={props.platform} />
            </div>
            <p className='text-muted-foreground text-xs break-all'>
              {props.url || t('sites.templates.enterAddress')}
            </p>
          </div>
          <Check className='text-primary size-4 shrink-0' aria-hidden='true' />
        </div>
      )}
      {expanded && (
        <Tabs
          value={category}
          onValueChange={(value) => {
            setCategory(value as PlatformDefinition['group'])
            setSearch('')
          }}
          className='gap-3'
        >
          <TabsList
            className='w-full'
            aria-label={t('sites.templates.categories')}
          >
            {(['api', 'gateway', 'oauth'] as const).map((group) => (
              <TabsTrigger key={group} value={group}>
                {t(`sites.templates.categoriesLabel.${group}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('sites.templates.search')}
            aria-label={t('sites.templates.search')}
          />
          {(['api', 'gateway', 'oauth'] as const).map((group) => {
            const templates = CONNECTION_TEMPLATES.filter(
              (template) =>
                template.group === group &&
                `${template.name} ${template.platform} ${template.url}`
                  .toLowerCase()
                  .includes(query)
            )
            return (
              <TabsContent key={group} value={group} className='space-y-3'>
                <div className='grid max-h-64 grid-cols-2 gap-2 overflow-y-auto pr-1 max-[420px]:grid-cols-1'>
                  {templates.map((template) => {
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
                            {group === 'api'
                              ? `${template.protocols?.map((protocol) => t(`sites.templates.protocols.${protocol}`)).join(' / ')} · API Key`
                              : t(
                                  template.available
                                    ? `sites.templates.auth.${group}`
                                    : 'sites.templates.notAvailable'
                                )}
                          </span>
                        </span>
                        {group === 'oauth' && (
                          <ArrowUpRight
                            className='size-3.5 shrink-0'
                            aria-hidden='true'
                          />
                        )}
                      </>
                    )
                    const className = cn(
                      'h-auto min-h-18 justify-start gap-2.5 whitespace-normal rounded-lg px-3 py-2.5',
                      props.selected?.id === template.id &&
                        'border-primary bg-primary/5'
                    )
                    if (group === 'oauth' && template.available) {
                      return (
                        <a
                          key={template.id}
                          className={cn(
                            buttonVariants({ variant: 'outline' }),
                            className
                          )}
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
                            ? t('sites.templates.use', { name: template.name })
                            : t('sites.templates.unavailable', {
                                name: template.name,
                              })
                        }
                        onClick={() => select(template)}
                      >
                        {contents}
                      </Button>
                    )
                  })}
                </div>
                {templates.length === 0 && (
                  <p className='text-muted-foreground py-3 text-center text-sm'>
                    {t('platforms.noResults')}
                  </p>
                )}
              </TabsContent>
            )
          })}
        </Tabs>
      )}
      {props.feedback && (
        <p role='status' className='text-muted-foreground text-xs'>
          {props.feedback}
        </p>
      )}
    </section>
  )
}
