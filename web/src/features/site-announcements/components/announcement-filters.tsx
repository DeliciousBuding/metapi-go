import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Site } from '@/features/sites'

import type { SiteAnnouncementFilters } from '../types'

export function AnnouncementFilters({
  filters,
  sites,
  updateFilters,
}: {
  filters: SiteAnnouncementFilters
  sites: Site[] | undefined
  updateFilters: (patch: Partial<SiteAnnouncementFilters>) => void
}) {
  const { t } = useTranslation()
  const platformOptions = useMemo(() => {
    const set = new Set<string>()
    for (const site of sites ?? []) {
      if (site.platform) set.add(site.platform)
    }
    return [...set].sort((left, right) => left.localeCompare(right))
  }, [sites])
  return (
    <div className='flex flex-wrap items-center gap-3'>
      <Select
        value={filters.siteId === null ? 'all' : String(filters.siteId)}
        onValueChange={(value) =>
          updateFilters({
            siteId: !value || value === 'all' ? null : Number(value),
          })
        }
      >
        <SelectTrigger
          aria-label={t('siteAnnouncements.filters.site')}
          className='w-44'
        >
          {/* base-ui only resolves the item label while the popup is
                mounted; map the value explicitly so the closed trigger
                never falls back to the raw "all" sentinel. */}
          <SelectValue>
            {(value: string) =>
              value === 'all'
                ? t('siteAnnouncements.filters.allSites')
                : ((sites ?? []).find((site) => String(site.id) === value)
                    ?.name ?? value)
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('siteAnnouncements.filters.allSites')}
          </SelectItem>
          {(sites ?? []).map((site) => (
            <SelectItem key={site.id} value={String(site.id)}>
              {site.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.platform || 'all'}
        onValueChange={(value) =>
          updateFilters({ platform: !value || value === 'all' ? '' : value })
        }
      >
        <SelectTrigger
          aria-label={t('siteAnnouncements.filters.platform')}
          className='w-40'
        >
          <SelectValue>
            {(value: string) =>
              value === 'all'
                ? t('siteAnnouncements.filters.allPlatforms')
                : value
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('siteAnnouncements.filters.allPlatforms')}
          </SelectItem>
          {platformOptions.map((platform) => (
            <SelectItem key={platform} value={platform}>
              {platform}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.read}
        onValueChange={(value) =>
          updateFilters({
            read: (value === 'true' || value === 'false'
              ? value
              : 'all') as SiteAnnouncementFilters['read'],
          })
        }
      >
        <SelectTrigger
          aria-label={t('siteAnnouncements.filters.read')}
          className='w-36'
        >
          <SelectValue>
            {(value: string) => {
              if (value === 'true') {
                return t('siteAnnouncements.filters.readTrue')
              }
              if (value === 'false') {
                return t('siteAnnouncements.filters.readFalse')
              }
              return t('siteAnnouncements.filters.readAll')
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('siteAnnouncements.filters.readAll')}
          </SelectItem>
          <SelectItem value='false'>
            {t('siteAnnouncements.filters.readFalse')}
          </SelectItem>
          <SelectItem value='true'>
            {t('siteAnnouncements.filters.readTrue')}
          </SelectItem>
        </SelectContent>
      </Select>

      <Select
        value={filters.status}
        onValueChange={(value) =>
          updateFilters({
            status: (value === 'active' ||
            value === 'expired' ||
            value === 'dismissed'
              ? value
              : 'all') as SiteAnnouncementFilters['status'],
          })
        }
      >
        <SelectTrigger
          aria-label={t('siteAnnouncements.filters.status')}
          className='w-36'
        >
          <SelectValue>
            {(value: string) => {
              if (value === 'active') {
                return t('siteAnnouncements.filters.statusActive')
              }
              if (value === 'expired') {
                return t('siteAnnouncements.filters.statusExpired')
              }
              if (value === 'dismissed') {
                return t('siteAnnouncements.filters.statusDismissed')
              }
              return t('siteAnnouncements.filters.statusAll')
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('siteAnnouncements.filters.statusAll')}
          </SelectItem>
          <SelectItem value='active'>
            {t('siteAnnouncements.filters.statusActive')}
          </SelectItem>
          <SelectItem value='expired'>
            {t('siteAnnouncements.filters.statusExpired')}
          </SelectItem>
          <SelectItem value='dismissed'>
            {t('siteAnnouncements.filters.statusDismissed')}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
