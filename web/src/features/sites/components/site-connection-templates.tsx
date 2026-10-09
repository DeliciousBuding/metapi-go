import { ArrowUpRight, ChevronDown, Check } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { PlatformBadge } from '@/components/common/platform-badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { SiteInitializationPreset } from '@/lib/api/sites'
import {
  PLATFORM_CONNECTION_TEMPLATES,
  getConnectionPresetIcon,
  getConnectionPresetNameKey,
  type ConnectionTemplate,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

import { useSiteInitializationPresets } from '../api'

type Props = {
  selected: ConnectionTemplate | null
  feedback: string
  url: string
  platform: string
  onSelect: (template: ConnectionTemplate) => boolean
}

const TEMPLATE_CATEGORIES = [
  'common',
  'gateway',
  'api',
  'coding',
  'oauth',
] as const
type TemplateCategory = (typeof TEMPLATE_CATEGORIES)[number]

const COMMON_TEMPLATE_IDS = [
  'new-api-connection',
  'bailian',
  'codingplan-openai',
  'siliconflow',
  'deepseek-openai',
  'moonshot-openai',
  'minimax-openai',
  'zhipu-coding-plan-openai',
  'zai-coding-plan-openai',
  'kimi-coding-openai',
  'doubao-coding-openai',
  'xiaomi-token-plan-claude',
]
const DOMESTIC_PROVIDERS = [
  'bailian',
  'siliconflow',
  'deepseek',
  'moonshot',
  'minimax',
  'modelscope',
  'zhipu',
  'zai',
  'doubao',
  'xiaomi',
  'stepfun',
  'mimo',
  'xiaomimimo',
  'ppio',
  'qiniu',
]

function isCodingPlan(template: ConnectionTemplate) {
  return (
    template.group === 'api' &&
    (/coding|token-plan/.test(template.id) ||
      template.id.startsWith('minimax-'))
  )
}

function providerFamily(template: ConnectionTemplate) {
  const family = template.id.split('-')[0]
  if (family === 'codingplan') return 'bailian'
  if (family === 'kimi') return 'moonshot'
  if (family === 'volcengine') return 'doubao'
  return family
}

function protocolOrder(template: ConnectionTemplate) {
  return ['openai', 'claude', 'gemini'].indexOf(template.platform)
}

function compareWithinBrand(a: ConnectionTemplate, b: ConnectionTemplate) {
  return (
    providerFamily(a).localeCompare(providerFamily(b), 'en') ||
    protocolOrder(a) - protocolOrder(b) ||
    a.id.localeCompare(b.id, 'en')
  )
}

const CODING_PROVIDERS = [
  'bailian',
  'zhipu',
  'moonshot',
  'doubao',
  'minimax',
  'xiaomi',
  'zai',
]

function codingOrder(template: ConnectionTemplate) {
  const index = CODING_PROVIDERS.indexOf(providerFamily(template))
  return index === -1 ? CODING_PROVIDERS.length : index
}

function serviceOrder(template: ConnectionTemplate) {
  const domestic = DOMESTIC_PROVIDERS.indexOf(providerFamily(template))
  if (domestic !== -1) return domestic
  const official = ['openai-api', 'anthropic-api', 'gemini-api'].indexOf(
    template.id
  )
  if (official !== -1) return 100 + official
  return 50
}

function templatesForCategory(
  templates: ConnectionTemplate[],
  category: TemplateCategory,
  query: string
) {
  if (query) {
    return templates
      .filter((template) =>
        `${template.name} ${template.providerLabel ?? ''} ${template.label ?? ''} ${template.platform} ${template.url}`
          .toLowerCase()
          .includes(query)
      )
      .sort(
        (a, b) => serviceOrder(a) - serviceOrder(b) || compareWithinBrand(a, b)
      )
  }
  if (category === 'common') {
    return COMMON_TEMPLATE_IDS.flatMap((id) =>
      templates.filter((template) => template.id === id)
    )
  }
  if (category === 'coding') {
    return templates
      .filter(isCodingPlan)
      .sort(
        (a, b) => codingOrder(a) - codingOrder(b) || compareWithinBrand(a, b)
      )
  }
  if (category === 'api') {
    return templates
      .filter(
        (template) =>
          template.group === 'api' &&
          (!isCodingPlan(template) || template.id.startsWith('minimax-'))
      )
      .sort(
        (a, b) => serviceOrder(a) - serviceOrder(b) || compareWithinBrand(a, b)
      )
  }
  return templates.filter((template) => template.group === category)
}

function templateFromPreset(
  preset: SiteInitializationPreset,
  name: string
): ConnectionTemplate {
  let protocols: ConnectionTemplate['protocols'] = ['chat']
  if (preset.id === 'openai-api') protocols = ['chat', 'responses']
  else if (preset.platform === 'claude') protocols = ['messages']
  else if (preset.platform === 'gemini') protocols = ['gemini']
  return {
    id: preset.id,
    name,
    providerLabel: preset.providerLabel,
    label: preset.label,
    platform: preset.platform,
    icon: getConnectionPresetIcon(preset.id),
    url: preset.defaultUrl,
    group: 'api',
    protocols,
    available: true,
  }
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
  const selectedNameKey =
    props.selected && getConnectionPresetNameKey(props.selected.id)
  const selectedName = selectedNameKey
    ? t(selectedNameKey)
    : props.selected?.name

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
              <span className='font-medium'>{selectedName}</span>
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
                    if (template.group === 'oauth' && template.available) {
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
                            ? t('sites.templates.use', {
                                name: template.label ?? template.name,
                              })
                            : t('sites.templates.unavailable', {
                                name: template.label ?? template.name,
                              })
                        }
                        onClick={() => select(template)}
                      >
                        {contents}
                      </Button>
                    )
                  })}
                </div>
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
