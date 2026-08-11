// metapi-go/features/about — the About page.
//
// A static information page (no data-table). Renders project metadata,
// version/build info (em-dash for fields the backend does not yet supply),
// curated key dependencies, and a public GitHub repository card. All links
// point to the public DeliciousBuding/metapi-go repository — no internal
// host paths, secrets, or private documentation references.

import {
  Code as CodeIcon,
  ExternalLink as ExternalLinkIcon,
  Heart as HeartIcon,
  Info as InfoIcon,
  Package as PackageIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

import { useAboutInfo } from '../api'
import { KEY_DEPENDENCIES, type AboutDependency } from '../types'

const DEPENDENCY_CATEGORY_LABEL_KEY: Record<
  AboutDependency['category'],
  string
> = {
  framework: 'about.deps.framework',
  build: 'about.deps.build',
  data: 'about.deps.data',
  ui: 'about.deps.ui',
  form: 'about.deps.form',
  style: 'about.deps.style',
}

function InfoRow({
  label,
  value,
  fallback = '—',
  mono = false,
}: {
  label: string
  value?: string | null
  fallback?: string
  mono?: boolean
}) {
  const display = value && value.trim().length > 0 ? value : fallback
  return (
    <div className='flex items-center justify-between gap-4 py-2'>
      <span className='text-muted-foreground text-sm'>{label}</span>
      <span
        className={
          mono
            ? 'text-foreground font-mono text-sm tabular-nums'
            : 'text-foreground text-sm font-medium'
        }
      >
        {display}
      </span>
    </div>
  )
}

export function AboutPage() {
  const { t } = useTranslation()
  const aboutQuery = useAboutInfo()
  const info = aboutQuery.data

  if (!info) {
    return null
  }

  const issueUrl = `${info.repository}/issues`
  const authorUrl = `https://github.com/${info.author}`

  return (
    <div className='mx-auto w-full max-w-4xl space-y-6 p-4 sm:p-6 lg:p-8'>
      <div className='space-y-1'>
        <h1 className='text-2xl font-semibold tracking-tight'>
          {t('about.title')}
        </h1>
        <p className='text-muted-foreground text-sm'>
          {t('about.description')}
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <InfoIcon className='text-muted-foreground size-4' />
            {t('about.sections.projectInfo')}
          </CardTitle>
          <CardDescription>
            {t('about.sections.projectInfoDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent className='divide-border divide-y'>
          <InfoRow
            label={t('about.fields.projectName')}
            value={info.projectName}
          />
          <InfoRow
            label={t('about.fields.description')}
            value={info.description}
          />
          <InfoRow
            label={t('about.fields.version')}
            value={info.version}
            mono
          />
          <InfoRow label={t('about.fields.license')} value={info.license} />
          <InfoRow label={t('about.fields.author')} value={info.author} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <PackageIcon className='text-muted-foreground size-4' />
            {t('about.sections.buildInfo')}
          </CardTitle>
          <CardDescription>
            {t('about.sections.buildInfoDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent className='divide-border divide-y'>
          <InfoRow
            label={t('about.fields.buildTime')}
            value={info.buildTime}
            mono
          />
          <InfoRow label={t('about.fields.commit')} value={info.commit} mono />
          <InfoRow
            label={t('about.fields.goVersion')}
            value={info.goVersion}
            mono
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <PackageIcon className='text-muted-foreground size-4' />
            {t('about.sections.dependencies')}
          </CardTitle>
          <CardDescription>
            {t('about.sections.dependenciesDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='flex flex-wrap gap-2'>
            {KEY_DEPENDENCIES.map((dep) => (
              <div
                key={dep.name}
                className='border-border flex items-center gap-2 rounded-lg border px-3 py-1.5'
              >
                <span className='text-foreground text-sm font-medium'>
                  {dep.name}
                </span>
                <Badge
                  variant='secondary'
                  className='font-mono text-xs tabular-nums'
                >
                  {dep.version}
                </Badge>
                <span className='text-muted-foreground text-xs'>
                  {t(DEPENDENCY_CATEGORY_LABEL_KEY[dep.category])}
                </span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <CodeIcon className='text-muted-foreground size-4' />
            {t('about.sections.links')}
          </CardTitle>
          <CardDescription>
            {t('about.sections.linksDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          <a
            href={info.repository}
            target='_blank'
            rel='noopener noreferrer'
            className='border-border hover:bg-accent flex items-center justify-between gap-2 rounded-lg border px-3 py-2 transition-colors'
          >
            <span className='flex items-center gap-2'>
              <CodeIcon className='text-muted-foreground size-4' />
              <span className='text-sm font-medium'>
                {t('about.links.repository')}
              </span>
            </span>
            <ExternalLinkIcon className='text-muted-foreground size-3.5' />
          </a>
          <a
            href={info.homepage}
            target='_blank'
            rel='noopener noreferrer'
            className='border-border hover:bg-accent flex items-center justify-between gap-2 rounded-lg border px-3 py-2 transition-colors'
          >
            <span className='flex items-center gap-2'>
              <ExternalLinkIcon className='text-muted-foreground size-4' />
              <span className='text-sm font-medium'>
                {t('about.links.homepage')}
              </span>
            </span>
            <ExternalLinkIcon className='text-muted-foreground size-3.5' />
          </a>
          <a
            href={issueUrl}
            target='_blank'
            rel='noopener noreferrer'
            className='border-border hover:bg-accent flex items-center justify-between gap-2 rounded-lg border px-3 py-2 transition-colors'
          >
            <span className='flex items-center gap-2'>
              <InfoIcon className='text-muted-foreground size-4' />
              <span className='text-sm font-medium'>
                {t('about.links.issues')}
              </span>
            </span>
            <ExternalLinkIcon className='text-muted-foreground size-3.5' />
          </a>
          <a
            href={authorUrl}
            target='_blank'
            rel='noopener noreferrer'
            className='border-border hover:bg-accent flex items-center justify-between gap-2 rounded-lg border px-3 py-2 transition-colors'
          >
            <span className='flex items-center gap-2'>
              <HeartIcon className='text-muted-foreground size-4' />
              <span className='text-sm font-medium'>
                {t('about.links.author')}
              </span>
            </span>
            <ExternalLinkIcon className='text-muted-foreground size-3.5' />
          </a>
        </CardContent>
      </Card>

      <Separator />
      <p className='text-muted-foreground text-center text-xs'>
        {t('about.footer.copyright', { year: new Date().getFullYear() })}
      </p>
    </div>
  )
}
