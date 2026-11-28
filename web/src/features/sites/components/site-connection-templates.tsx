import { ChevronDown } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  PLATFORM_CONNECTION_TEMPLATES,
  getConnectionPresetNameKey,
  type ConnectionTemplate,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

import { useSiteInitializationPresets } from '../api'
import {
  TEMPLATE_CATEGORIES,
  type TemplateCategory,
  templatesForCategory,
  templateFromPreset,
} from './site-connection-template-catalog'
import { SiteConnectionTemplateList } from './site-connection-template-list'
import { SiteConnectionTemplatePreview } from './site-connection-template-preview'

type Props = {
  selected: ConnectionTemplate | null
  feedback: string
  url: string
  platform: string
  onSelect: (template: ConnectionTemplate) => boolean
}

export function SiteConnectionTemplates(props: Props) {
  const { t } = useTranslation()
  const presets = useSiteInitializationPresets()
  const allTemplates = [
    ...(presets.data ?? []).map((preset) => {
      const nameKey = getConnectionPresetNameKey(preset.id)
      return templateFromPreset(
        preset,
        nameKey ? t(nameKey) : preset.providerLabel
      )
    }),
    ...PLATFORM_CONNECTION_TEMPLATES,
  ]
  const [expanded, setExpanded] = useState(true)
  const [category, setCategory] = useState<TemplateCategory>('common')
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
        <SiteConnectionTemplatePreview
          selected={props.selected}
          platform={props.platform}
          url={props.url}
        />
      )}
      {expanded && (
        <Tabs
          value={category}
          onValueChange={(value) => {
            setCategory(value as TemplateCategory)
            setSearch('')
          }}
          className='gap-3'
        >
          <TabsList
            className='w-full justify-start overflow-x-auto'
            aria-label={t('sites.templates.categories')}
          >
            {TEMPLATE_CATEGORIES.map((group) => (
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
          {TEMPLATE_CATEGORIES.map((group) => {
            const templates = templatesForCategory(allTemplates, group, query)
            const usesApiPresets =
              group === 'common' ||
              group === 'api' ||
              group === 'coding' ||
              Boolean(query)
            return (
              <TabsContent key={group} value={group} className='space-y-3'>
                {usesApiPresets && presets.isPending && (
                  <div
                    className='flex justify-center py-3'
                    aria-label={t('sites.templates.loading')}
                  >
                    <Spinner />
                  </div>
                )}
                {usesApiPresets && presets.isError && (
                  <div
                    role='alert'
                    className='flex items-center justify-between gap-3 text-sm'
                  >
                    <span>{t('sites.templates.loadFailed')}</span>
                    <Button
                      type='button'
                      variant='ghost'
                      size='xs'
                      onClick={() => void presets.refetch()}
                    >
                      {t('common.retry')}
                    </Button>
                  </div>
                )}
                <SiteConnectionTemplateList
                  templates={templates}
                  selectedId={props.selected?.id}
                  onSelect={select}
                />
                {templates.length === 0 &&
                  !(
                    usesApiPresets &&
                    (presets.isPending || presets.isError)
                  ) && (
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
