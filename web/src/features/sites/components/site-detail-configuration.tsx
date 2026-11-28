import { ExternalLink as ExternalLinkIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { ExpandableText } from '@/components/common/expandable-text'
import { PlatformBadge } from '@/components/common/platform-badge'

import { isHttpUrl, isValidEndpointUrl } from '../lib/endpoints'
import type { Site } from '../types'

export function SiteDetailConfiguration({ site }: { site: Site }) {
  const { t } = useTranslation()
  const siteURL = site.url.trim()
  const hasSafeSiteURL = isHttpUrl(siteURL)
  return (
    <section>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('sites.detail.url')} full title={site.url}>
          <ExpandableText text={site.url} className='max-w-full'>
            {hasSafeSiteURL ? (
              <a
                href={siteURL}
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary inline-flex max-w-full min-w-0 items-start gap-1 hover:underline'
              >
                <span className='min-w-0 break-all'>{site.url}</span>
                <ExternalLinkIcon className='size-4 shrink-0' />
              </a>
            ) : (
              <span className='block max-w-full break-all'>{site.url}</span>
            )}
          </ExpandableText>
        </DetailField>
        {site.externalCheckinUrl ? (
          <DetailField
            label={t('sites.detail.externalCheckinUrl')}
            full
            title={site.externalCheckinUrl}
          >
            <ExpandableText
              text={site.externalCheckinUrl}
              className='max-w-full'
            >
              {/^https?:\/\//i.test(site.externalCheckinUrl) &&
              isValidEndpointUrl(site.externalCheckinUrl) ? (
                <a
                  href={site.externalCheckinUrl}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary inline-flex max-w-full min-w-0 items-start gap-1 hover:underline'
                >
                  <span className='min-w-0 break-all'>
                    {site.externalCheckinUrl}
                  </span>
                  <ExternalLinkIcon className='size-4 shrink-0' />
                </a>
              ) : (
                <span className='text-muted-foreground break-all'>
                  {site.externalCheckinUrl}
                </span>
              )}
            </ExpandableText>
          </DetailField>
        ) : null}
        {site.platform ? (
          <DetailField label={t('sites.detail.platform')}>
            <PlatformBadge platform={site.platform} />
          </DetailField>
        ) : null}
        <DetailField label={t('sites.detail.globalWeight')}>
          {site.globalWeight ?? 1}
        </DetailField>
        <DetailField label={t('sites.detail.maxConcurrency')}>
          {site.maxConcurrency
            ? site.maxConcurrency
            : t('sites.detail.unlimited')}
        </DetailField>
        <DetailField label={t('sites.detail.useSystemProxy')}>
          {site.useSystemProxy ? t('sites.detail.yes') : t('sites.detail.no')}
        </DetailField>
        {site.proxyUrl ? (
          <DetailField label={t('sites.detail.proxyUrl')} title={site.proxyUrl}>
            {site.proxyUrl}
          </DetailField>
        ) : null}
      </dl>
    </section>
  )
}
