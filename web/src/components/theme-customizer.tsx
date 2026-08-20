// metapi-go/components — theme customizer panel
// (color scheme / preset / font / radius / scale / content-layout).
// Adapted from newapi's config-drawer (AGPL header stripped): a compact
// popover for the app header instead of a full sheet. Consumes the
// ThemeCustomizationProvider axes plus the ThemeProvider color scheme, so the
// mode can always be returned to "follow system".

import { Radio as RadioPrimitive } from '@base-ui/react/radio'
import { Check, Monitor, Moon, Palette, RotateCcw, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RadioGroup } from '@/components/ui/radio-group'
import { Separator } from '@/components/ui/separator'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme, type Theme } from '@/context/theme-provider'
import {
  THEME_PRESETS,
  type ContentLayout,
  type ThemeFont,
  type ThemePreset,
  type ThemeRadius,
  type ThemeScale,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

const COLOR_SCHEME_OPTIONS: {
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

const FONT_OPTIONS: {
  value: ThemeFont
  labelKey: string
  // CSS font-family for the "Aa" preview. `undefined` inherits the active
  // theme font, so "Auto" previews what the resolved font actually is.
  preview?: string
}[] = [
  { value: 'default', labelKey: 'theme.fontOptions.auto' },
  {
    value: 'sans',
    labelKey: 'theme.fontOptions.sans',
    preview: 'var(--font-sans)',
  },
  {
    value: 'serif',
    labelKey: 'theme.fontOptions.serif',
    preview: 'var(--font-serif)',
  },
]

const RADIUS_OPTIONS: {
  value: ThemeRadius
  labelKey: string
  // CSS border-radius applied to the preview corner.
  preview: string
}[] = [
  { value: 'default', labelKey: 'theme.radiusOptions.auto', preview: '1rem' },
  { value: 'none', labelKey: 'theme.radiusOptions.none', preview: '0' },
  { value: 'sm', labelKey: 'theme.radiusOptions.sm', preview: '0.3rem' },
  { value: 'md', labelKey: 'theme.radiusOptions.md', preview: '0.5rem' },
  { value: 'lg', labelKey: 'theme.radiusOptions.lg', preview: '0.75rem' },
  { value: 'xl', labelKey: 'theme.radiusOptions.xl', preview: '1rem' },
]

const SCALE_OPTIONS: {
  value: ThemeScale
  labelKey: string
  // Font size used by the "Aa" preview tile.
  fontSize: string
}[] = [
  {
    value: 'default',
    labelKey: 'theme.scaleOptions.auto',
    fontSize: '0.875rem',
  },
  { value: 'sm', labelKey: 'theme.scaleOptions.sm', fontSize: '0.75rem' },
  { value: 'lg', labelKey: 'theme.scaleOptions.lg', fontSize: '1rem' },
  { value: 'xl', labelKey: 'theme.scaleOptions.xl', fontSize: '1.25rem' },
]

const CONTENT_LAYOUT_OPTIONS: {
  value: ContentLayout
  labelKey: string
}[] = [
  { value: 'full', labelKey: 'theme.contentLayoutOptions.full' },
  { value: 'centered', labelKey: 'theme.contentLayoutOptions.centered' },
]

const TILE_CLASSES = cn(
  'ring-border relative h-12 rounded-md ring-[1px] transition',
  'group-data-checked:ring-primary group-data-checked:shadow-md',
  'group-focus-visible:ring-2',
  'group-hover:ring-primary/60'
)

/** Small check badge shown on the selected tile. */
function TileCheck() {
  return (
    <span
      aria-hidden='true'
      className='bg-primary absolute top-0 right-0 z-10 flex size-4.5 translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full group-data-unchecked:hidden'
    >
      <Check className='text-primary-foreground size-3' />
    </span>
  )
}

/** Section heading with a per-axis reset, shown only when the axis is customized. */
function SectionTitle(props: {
  title: string
  showReset: boolean
  onReset: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='text-muted-foreground mb-2 flex items-center gap-2 text-xs font-semibold tracking-wide uppercase'>
      {props.title}
      {props.showReset && (
        <Button
          variant='ghost'
          size='icon'
          className='size-5'
          onClick={props.onReset}
          aria-label={t('theme.reset')}
        >
          <RotateCcw className='size-3' aria-hidden='true' />
        </Button>
      )}
    </div>
  )
}

function ColorSchemeSection() {
  const { t } = useTranslation()
  const { defaultTheme, theme, setTheme, resetTheme } = useTheme()
  return (
    <section>
      <SectionTitle
        title={t('theme.colorScheme')}
        showReset={theme !== defaultTheme}
        onReset={resetTheme}
      />
      <RadioGroup
        value={theme}
        onValueChange={(value) => setTheme(value as Theme)}
        className='grid grid-cols-3 gap-2'
        aria-label={t('theme.colorScheme')}
      >
        {COLOR_SCHEME_OPTIONS.map((option) => (
          <RadioPrimitive.Root
            key={option.value}
            value={option.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(option.labelKey)}
          >
            <div className={TILE_CLASSES}>
              <option.icon
                aria-hidden='true'
                className='text-foreground absolute inset-0 m-auto size-4.5'
              />
              <TileCheck />
            </div>
            <div className='mt-1.5 text-center text-xs'>
              {t(option.labelKey)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

function PresetSection() {
  const { t } = useTranslation()
  const { defaults, customization, setPreset } = useThemeCustomization()
  return (
    <section>
      <SectionTitle
        title={t('theme.colorPreset')}
        showReset={customization.preset !== defaults.preset}
        onReset={() => setPreset(defaults.preset)}
      />
      <RadioGroup
        value={customization.preset}
        onValueChange={(value) => setPreset(value as ThemePreset)}
        className='grid grid-cols-5 gap-2'
        aria-label={t('theme.colorPreset')}
      >
        {THEME_PRESETS.map((preset) => (
          <RadioPrimitive.Root
            key={preset.value}
            value={preset.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(`theme.preset.${preset.value}`)}
          >
            <div className={TILE_CLASSES}>
              <div
                aria-hidden='true'
                className='absolute inset-0 rounded-md'
                style={{
                  backgroundColor: preset.swatches[0],
                }}
              />
              <TileCheck />
            </div>
            <div className='mt-1.5 truncate text-center text-xs'>
              {t(`theme.preset.${preset.value}`)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

function FontSection() {
  const { t } = useTranslation()
  const { defaults, customization, setFont } = useThemeCustomization()
  return (
    <section>
      <SectionTitle
        title={t('theme.font')}
        showReset={customization.font !== defaults.font}
        onReset={() => setFont(defaults.font)}
      />
      <RadioGroup
        value={customization.font}
        onValueChange={(value) => setFont(value as ThemeFont)}
        className='grid grid-cols-3 gap-2'
        aria-label={t('theme.font')}
      >
        {FONT_OPTIONS.map((option) => (
          <RadioPrimitive.Root
            key={option.value}
            value={option.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(option.labelKey)}
          >
            <div className={TILE_CLASSES}>
              <span
                aria-hidden='true'
                className='text-foreground absolute inset-0 flex items-center justify-center text-lg leading-none font-medium'
                style={
                  option.preview
                    ? { fontFamily: option.preview }
                    : { font: 'inherit', fontSize: '1.125rem' }
                }
              >
                Aa
              </span>
              <TileCheck />
            </div>
            <div className='mt-1.5 text-center text-xs'>
              {t(option.labelKey)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

function RadiusSection() {
  const { t } = useTranslation()
  const { defaults, customization, setRadius } = useThemeCustomization()
  return (
    <section>
      <SectionTitle
        title={t('theme.radius')}
        showReset={customization.radius !== defaults.radius}
        onReset={() => setRadius(defaults.radius)}
      />
      <RadioGroup
        value={customization.radius}
        onValueChange={(value) => setRadius(value as ThemeRadius)}
        className='grid grid-cols-6 gap-2'
        aria-label={t('theme.radius')}
      >
        {RADIUS_OPTIONS.map((option) => (
          <RadioPrimitive.Root
            key={option.value}
            value={option.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(option.labelKey)}
          >
            <div className={TILE_CLASSES}>
              <span
                aria-hidden='true'
                className='border-foreground/70 absolute top-2.5 left-2.5 size-3.5 border-t-[1.5px] border-l-[1.5px]'
                style={{ borderTopLeftRadius: option.preview }}
              />
              <TileCheck />
            </div>
            <div className='mt-1.5 text-center text-xs'>
              {t(option.labelKey)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

function ScaleSection() {
  const { t } = useTranslation()
  const { defaults, customization, setScale } = useThemeCustomization()
  return (
    <section>
      <SectionTitle
        title={t('theme.scale')}
        showReset={customization.scale !== defaults.scale}
        onReset={() => setScale(defaults.scale)}
      />
      <RadioGroup
        value={customization.scale}
        onValueChange={(value) => setScale(value as ThemeScale)}
        className='grid grid-cols-4 gap-2'
        aria-label={t('theme.scale')}
      >
        {SCALE_OPTIONS.map((option) => (
          <RadioPrimitive.Root
            key={option.value}
            value={option.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(option.labelKey)}
          >
            <div className={TILE_CLASSES}>
              <span
                aria-hidden='true'
                className='text-foreground absolute inset-0 flex items-center justify-center leading-none font-medium'
                style={{ fontSize: option.fontSize }}
              >
                Aa
              </span>
              <TileCheck />
            </div>
            <div className='mt-1.5 text-center text-xs'>
              {t(option.labelKey)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

function ContentLayoutSection() {
  const { t } = useTranslation()
  const { defaults, customization, setContentLayout } = useThemeCustomization()
  return (
    <section>
      <SectionTitle
        title={t('theme.contentLayout')}
        showReset={customization.contentLayout !== defaults.contentLayout}
        onReset={() => setContentLayout(defaults.contentLayout)}
      />
      <RadioGroup
        value={customization.contentLayout}
        onValueChange={(value) => setContentLayout(value as ContentLayout)}
        className='grid grid-cols-2 gap-2'
        aria-label={t('theme.contentLayout')}
      >
        {CONTENT_LAYOUT_OPTIONS.map((option) => (
          <RadioPrimitive.Root
            key={option.value}
            value={option.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(option.labelKey)}
          >
            <div className={TILE_CLASSES}>
              {/* Mini page diagram: full-width content vs a centered column. */}
              <span
                aria-hidden='true'
                className='border-border/80 bg-muted-foreground/25 absolute inset-x-1.5 inset-y-2 rounded-[3px] border'
              />
              <span
                aria-hidden='true'
                className={cn(
                  'bg-muted-foreground absolute inset-y-3.5 rounded-[2px]',
                  option.value === 'full'
                    ? 'inset-x-3'
                    : 'left-1/2 w-1/3 -translate-x-1/2'
                )}
              />
              <TileCheck />
            </div>
            <div className='mt-1.5 text-center text-xs'>
              {t(option.labelKey)}
            </div>
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    </section>
  )
}

/**
 * Header entry for the theme customizer. Renders the trigger button and
 * hosts the appearance panel (color scheme / color preset / font / radius /
 * scale / content layout) in a popover.
 */
export function ThemeCustomizer() {
  const { t } = useTranslation()
  const { resetCustomization } = useThemeCustomization()

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
        <Palette className='size-4' aria-hidden='true' />
        <span className='sr-only'>{t('theme.appearance')}</span>
      </PopoverTrigger>
      <PopoverContent
        align='end'
        className='max-h-[min(75vh,38rem)] w-80 overflow-y-auto'
      >
        <div className='flex items-center justify-between gap-2 px-1 pt-0.5'>
          <div className='text-sm font-semibold'>{t('theme.title')}</div>
          <Button
            variant='ghost'
            size='icon'
            className='size-6'
            onClick={resetCustomization}
            aria-label={t('theme.reset')}
          >
            <RotateCcw className='size-3.5' aria-hidden='true' />
          </Button>
        </div>
        <Separator className='my-2.5' />
        <div className='flex flex-col gap-3.5'>
          <ColorSchemeSection />
          <Separator />
          <PresetSection />
          <Separator />
          <FontSection />
          <Separator />
          <RadiusSection />
          <Separator />
          <ScaleSection />
          <Separator />
          <ContentLayoutSection />
        </div>
      </PopoverContent>
    </Popover>
  )
}
