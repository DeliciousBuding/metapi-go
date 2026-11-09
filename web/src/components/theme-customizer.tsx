import { Radio as RadioPrimitive } from '@base-ui/react/radio'
import { Check, Monitor, Moon, Sun } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RadioGroup } from '@/components/ui/radio-group'
import { useTheme, type Theme } from '@/context/theme-provider'
import type { ThemeScale } from '@/lib/theme-scale'

const COLOR_SCHEMES: {
  value: Theme
  labelKey: string
  icon: typeof Sun
}[] = [
  { value: 'light', labelKey: 'theme.colorSchemeOptions.light', icon: Sun },
  { value: 'dark', labelKey: 'theme.colorSchemeOptions.dark', icon: Moon },
  {
    value: 'system',
    labelKey: 'theme.colorSchemeOptions.system',
    icon: Monitor,
  },
]

const DISPLAY_SIZES: { value: ThemeScale; labelKey: string }[] = [
  { value: 'sm', labelKey: 'theme.scaleOptions.sm' },
  { value: 'default', labelKey: 'theme.scaleOptions.auto' },
  { value: 'lg', labelKey: 'theme.scaleOptions.lg' },
  { value: 'xl', labelKey: 'theme.scaleOptions.xl' },
]

export function ThemeCustomizer() {
  const { t } = useTranslation()
  const titleId = useId()
  const { theme, resolvedTheme, setTheme, scale, setScale } = useTheme()
  const ThemeIcon = resolvedTheme === 'dark' ? Moon : Sun

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('theme.appearance')}
          />
        }
      >
        <ThemeIcon className='size-4' aria-hidden='true' />
      </PopoverTrigger>
      <PopoverContent
        align='end'
        className='w-80 max-w-[calc(100vw-2rem)]'
        aria-labelledby={titleId}
      >
        <h2 id={titleId} className='mb-4 text-sm font-semibold'>
          {t('theme.title')}
        </h2>
        <div className='space-y-4'>
          <section className='space-y-2'>
            <h3 className='text-muted-foreground text-xs font-medium'>
              {t('theme.colorScheme')}
            </h3>
            <RadioGroup
              value={theme}
              onValueChange={(value) => setTheme(value as Theme)}
              className='grid grid-cols-3 gap-1'
              aria-label={t('theme.colorScheme')}
            >
              {COLOR_SCHEMES.map((option) => (
                <RadioPrimitive.Root
                  key={option.value}
                  value={option.value}
                  data-touch-target
                  className='border-border data-checked:border-primary data-checked:bg-accent data-checked:text-accent-foreground focus-visible:ring-focus-ring relative flex min-h-9 items-center justify-center gap-1.5 rounded-md border px-2 text-xs outline-none focus-visible:ring-2'
                >
                  <option.icon
                    className='size-3.5 shrink-0'
                    aria-hidden='true'
                  />
                  {t(option.labelKey)}
                  <RadioPrimitive.Indicator className='absolute top-0.5 right-0.5'>
                    <Check className='size-2.5' aria-hidden='true' />
                  </RadioPrimitive.Indicator>
                </RadioPrimitive.Root>
              ))}
            </RadioGroup>
          </section>
          <section className='space-y-2'>
            <h3 className='text-muted-foreground text-xs font-medium'>
              {t('theme.scale')}
            </h3>
            <RadioGroup
              value={scale}
              onValueChange={(value) => setScale(value as ThemeScale)}
              className='grid grid-cols-4 gap-1'
              aria-label={t('theme.scale')}
            >
              {DISPLAY_SIZES.map((option) => (
                <RadioPrimitive.Root
                  key={option.value}
                  value={option.value}
                  data-touch-target
                  className='border-border data-checked:border-primary data-checked:bg-accent data-checked:text-accent-foreground focus-visible:ring-focus-ring relative flex min-h-9 items-center justify-center rounded-md border px-1 text-xs outline-none focus-visible:ring-2'
                >
                  {t(option.labelKey)}
                  <RadioPrimitive.Indicator className='absolute top-0.5 right-0.5'>
                    <Check className='size-2.5' aria-hidden='true' />
                  </RadioPrimitive.Indicator>
                </RadioPrimitive.Root>
              ))}
            </RadioGroup>
          </section>
        </div>
      </PopoverContent>
    </Popover>
  )
}
