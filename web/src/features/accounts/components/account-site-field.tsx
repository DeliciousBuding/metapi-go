import { ChevronsUpDown } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { PlatformBadge } from '@/components/common/platform-badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  getPlatformDefinition,
  getPlatformDisplayName,
} from '@/lib/platform-catalog'

import type { AccountFormValues } from '../lib/accounts-schema'
import type { Site } from '../types'

function getSiteLabel(site: Site): string {
  return site.name || site.url || `#${site.id}`
}

function getSiteSearchValue(site: Site): string {
  return [
    site.name,
    site.url,
    site.platform,
    getPlatformDisplayName(site.platform),
    String(site.id),
  ]
    .filter(Boolean)
    .join(' ')
}

export function AccountSiteField({
  form,
  sites,
  open,
  inferCredentialMode,
}: {
  form: UseFormReturn<AccountFormValues>
  sites: Site[]
  open: boolean
  inferCredentialMode: boolean
}) {
  const { t } = useTranslation()
  const [siteSelectorOpen, setSiteSelectorOpen] = useState(false)
  useEffect(() => {
    if (!open) setSiteSelectorOpen(false)
  }, [open])
  const siteSearchInputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (!siteSelectorOpen) return

    const animationFrameId = window.requestAnimationFrame(() => {
      siteSearchInputRef.current?.focus()
    })
    return () => window.cancelAnimationFrame(animationFrameId)
  }, [siteSelectorOpen])
  return (
    <FormField
      control={form.control}
      name='siteId'
      render={({ field }) => {
        const selectedSite = sites.find((site) => site.id === field.value)
        const hasSelection = field.value > 0
        let selectedLabel = t('accounts.form.sitePlaceholder')
        if (selectedSite) {
          selectedLabel = getSiteLabel(selectedSite)
        } else if (hasSelection) {
          selectedLabel = `#${field.value}`
        }

        return (
          <FormItem>
            <FormLabel>{t('accounts.form.site')}</FormLabel>
            <Popover
              open={siteSelectorOpen}
              onOpenChange={(nextOpen) => {
                setSiteSelectorOpen(nextOpen)
                if (!nextOpen) field.onBlur()
              }}
            >
              <FormControl>
                <PopoverTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      role='combobox'
                      aria-expanded={siteSelectorOpen}
                      aria-haspopup='listbox'
                      className='w-full justify-between font-normal'
                    />
                  }
                >
                  <span
                    className={
                      hasSelection
                        ? 'flex min-w-0 flex-1 items-center gap-2 text-left'
                        : 'text-muted-foreground min-w-0 flex-1 truncate text-left'
                    }
                  >
                    <span className='truncate'>{selectedLabel}</span>
                    {selectedSite?.platform && (
                      <PlatformBadge
                        platform={selectedSite.platform}
                        className='text-muted-foreground shrink-0 text-xs'
                      />
                    )}
                  </span>
                  <ChevronsUpDown
                    aria-hidden='true'
                    className='text-muted-foreground size-4 opacity-50'
                  />
                </PopoverTrigger>
              </FormControl>
              <PopoverContent
                align='start'
                initialFocus={siteSearchInputRef}
                className='w-(--anchor-width) max-w-[calc(100vw-2rem)] p-0'
              >
                <Command>
                  <CommandInput
                    ref={siteSearchInputRef}
                    placeholder={t('sites.toolbar.searchPlaceholder')}
                  />
                  <CommandList className='max-h-[min(20rem,var(--available-height))]'>
                    <CommandEmpty>
                      {sites.length === 0
                        ? t('accounts.form.siteEmpty')
                        : t('No results found.')}
                    </CommandEmpty>
                    <CommandGroup>
                      {sites.map((site) => {
                        const label = getSiteLabel(site)
                        const details = site.name ? site.url : ''

                        return (
                          <CommandItem
                            key={site.id}
                            value={getSiteSearchValue(site)}
                            data-checked={field.value === site.id}
                            onSelect={() => {
                              if (
                                inferCredentialMode &&
                                !form.getFieldState('credentialMode').isDirty &&
                                !form.getValues('accessToken') &&
                                !form.getValues('apiToken') &&
                                !form.getValues('password')
                              ) {
                                form.setValue(
                                  'credentialMode',
                                  getPlatformDefinition(site.platform)
                                    ?.group === 'api'
                                    ? 'apikey'
                                    : 'session'
                                )
                              }
                              field.onChange(site.id)
                              field.onBlur()
                              setSiteSelectorOpen(false)
                            }}
                          >
                            <span className='min-w-0 flex-1'>
                              <span className='block truncate'>{label}</span>
                              {(site.platform || details) && (
                                <span className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs'>
                                  {site.platform && (
                                    <PlatformBadge
                                      platform={site.platform}
                                      className='shrink-0'
                                    />
                                  )}
                                  {details && (
                                    <span className='truncate'>{details}</span>
                                  )}
                                </span>
                              )}
                            </span>
                          </CommandItem>
                        )
                      })}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )
}
