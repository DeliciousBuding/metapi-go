import { ChevronsUpDown } from 'lucide-react'
import { useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  CONNECTION_PRESETS,
  PLATFORM_CATALOG,
  getPlatformDefinition,
  type ConnectionPreset,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

type SitePlatformPickerProps = Omit<
  ComponentProps<'button'>,
  'value' | 'onChange'
> & {
  value: string
  onValueChange: (value: string) => void
  onPreset?: (preset: ConnectionPreset) => void
}

export function SitePlatformPicker({
  value,
  onValueChange,
  onPreset,
  className,
  ...restProps
}: SitePlatformPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const custom = value.trim() && !getPlatformDefinition(value)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            {...restProps}
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            aria-haspopup='listbox'
            className={cn(
              'h-9 w-full justify-between px-2.5 font-normal',
              className
            )}
          >
            {value ? (
              <PlatformBadge platform={value} />
            ) : (
              <span className='text-muted-foreground truncate'>
                {t('sites.form.platformSelectPlaceholder')}
              </span>
            )}
            <ChevronsUpDown
              className='text-muted-foreground size-4 shrink-0'
              aria-hidden='true'
            />
          </Button>
        }
      />
      <PopoverContent
        aria-label={t('sites.form.platformSelectPlaceholder')}
        align='start'
        className='w-96 max-w-[calc(100vw-2rem)] p-0'
      >
        <Command label={t('platforms.search')}>
          <CommandInput
            placeholder={t('platforms.search')}
            aria-label={t('platforms.search')}
          />
          <CommandList className='max-h-80'>
            <CommandEmpty>{t('platforms.noResults')}</CommandEmpty>
            {custom && (
              <CommandGroup heading={t('platforms.custom')}>
                <CommandItem
                  value={value}
                  onSelect={() => {
                    onValueChange(value)
                    setOpen(false)
                  }}
                >
                  <PlatformBadge platform={value} />
                </CommandItem>
              </CommandGroup>
            )}
            {(['api', 'gateway', 'oauth'] as const).map((group) => (
              <CommandGroup
                key={group}
                heading={t(`platforms.groups.${group}`)}
              >
                {PLATFORM_CATALOG.filter((entry) => entry.group === group).map(
                  (entry) => (
                    <CommandItem
                      key={entry.id}
                      value={entry.id}
                      keywords={[entry.name, t(entry.descriptionKey)]}
                      aria-label={entry.name}
                      data-checked={value === entry.id}
                      onSelect={() => {
                        onValueChange(entry.id)
                        setOpen(false)
                      }}
                    >
                      <div className='min-w-0 space-y-1'>
                        <PlatformBadge platform={entry.id} />
                        <p className='text-muted-foreground text-xs'>
                          {t(entry.descriptionKey)}
                        </p>
                      </div>
                    </CommandItem>
                  )
                )}
              </CommandGroup>
            ))}
            {onPreset && (
              <CommandGroup heading={t('platforms.groups.presets')}>
                {CONNECTION_PRESETS.map((preset) => (
                  <CommandItem
                    key={preset.id}
                    value={`preset-${preset.id}`}
                    keywords={[preset.name, preset.url]}
                    disabled={
                      Boolean(value.trim()) && value !== preset.platform
                    }
                    aria-label={t('platforms.usePreset', { name: preset.name })}
                    onSelect={() => {
                      onPreset(preset)
                      setOpen(false)
                    }}
                  >
                    <span aria-hidden='true'>
                      <BrandGlyph
                        icon={preset.icon}
                        size={18}
                        fallbackText={preset.name}
                      />
                    </span>
                    <div className='min-w-0'>
                      <p className='font-medium'>{preset.name}</p>
                      <p className='text-muted-foreground truncate text-xs'>
                        {preset.url}
                      </p>
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
          {onPreset && (
            <p className='text-muted-foreground border-t px-3 py-2 text-xs'>
              {t('platforms.presetHint')}
            </p>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  )
}
