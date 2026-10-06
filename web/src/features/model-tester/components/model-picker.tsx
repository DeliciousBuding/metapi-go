import { ChevronsUpDown } from 'lucide-react'
import { useMemo, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { getBrand } from '@/assets/brand-icons/BrandIcon'
import { ModelPill } from '@/components/common/model-pill'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

type ModelPickerProps = Omit<ComponentProps<'button'>, 'value' | 'onChange'> & {
  models: Array<{ name: string }>
  value: string
  onValueChange: (model: string) => void
  placeholder?: string
}

/** Search and provider grouping use the existing model identity registry. */
export function ModelPicker({
  models,
  value,
  onValueChange,
  placeholder,
  className,
  ...restProps
}: ModelPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const groups = useMemo(() => {
    const grouped = new Map<string, Array<{ name: string }>>()
    for (const model of models) {
      const provider = getBrand(model.name)?.name ?? t('modelPicker.other')
      grouped.set(provider, [...(grouped.get(provider) ?? []), model])
    }
    return [...grouped].sort(([a], [b]) => a.localeCompare(b))
  }, [models, t])
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            {...restProps}
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            className={cn(
              'w-full justify-between gap-3 font-normal',
              className
            )}
          >
            {value ? (
              <ModelPill
                model={value}
                variant='inline'
                className='min-w-0 text-sm'
              />
            ) : (
              <span className='text-muted-foreground truncate'>
                {placeholder ?? t('modelPicker.placeholder')}
              </span>
            )}
            <ChevronsUpDown
              className='text-muted-foreground size-4 shrink-0'
              aria-hidden='true'
            />
          </Button>
        }
      />
      <DialogContent className='gap-3 sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('modelPicker.title')}</DialogTitle>
          <DialogDescription>
            {t('modelPicker.description', { count: models.length })}
          </DialogDescription>
        </DialogHeader>
        <Command label={t('modelPicker.search')}>
          <CommandInput
            aria-label={t('modelPicker.search')}
            placeholder={t('modelPicker.search')}
          />
          <CommandList className='max-h-80'>
            <CommandEmpty>{t('modelPicker.empty')}</CommandEmpty>
            {groups.map(([provider, items]) => (
              <CommandGroup
                key={provider}
                heading={`${provider} · ${items.length}`}
              >
                {items.map((model) => (
                  <CommandItem
                    key={model.name}
                    value={model.name}
                    keywords={[provider]}
                    data-checked={value === model.name}
                    onSelect={() => {
                      onValueChange(model.name)
                      setOpen(false)
                    }}
                    className='min-h-10'
                  >
                    <ModelPill
                      model={model.name}
                      variant='inline'
                      className='min-w-0 text-sm'
                    />
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  )
}
