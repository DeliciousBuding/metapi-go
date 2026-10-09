import { ChevronsUpDown } from 'lucide-react'
import { useState, type ComponentProps } from 'react'
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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
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
  onPreset: _onPreset,
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
                {PLATFORM_CATALOG.filter(
                  (entry) => entry.group === group && entry.selectable !== false
                ).map((entry) => (
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
                    <PlatformBadge platform={entry.id} />
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
