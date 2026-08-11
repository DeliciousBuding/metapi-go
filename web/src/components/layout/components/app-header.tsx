// metapi-go/layout — app-header adapted from newapi. AGPL header stripped.
// Minimal header: brand (left) + language switcher + theme toggle (right).
// newapi's full header (TopNav / Search / NotificationPopover / ConfigDrawer /
// ProfileDropdown) is intentionally deferred — those features land in phases 2-3.
// The header shell uses the --app-header-height CSS var consumed by SidebarInset.

import { Check, Languages, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/context/theme-provider'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { metapiIdentity } from '@/lib/identity-branding'
import { cn } from '@/lib/utils'

/**
 * Skeleton application header.
 *
 * Renders the Metapi brand on the left and light/dark theme toggle on
 * the right. Right-side slots (search, notifications, config, profile)
 * will be filled in as their features land.
 *
 * @example
 * // Basic usage
 * <AppHeader />
 *
 * @example
 * // Hide the theme toggle
 * <AppHeader showThemeToggle={false} />
 */
type AppHeaderProps = {
  /**
   * Whether to show the theme toggle button
   * @default true
   */
  showThemeToggle?: boolean
  /**
   * Custom left content, overrides the brand if provided
   */
  leftContent?: React.ReactNode
  /**
   * Custom right content, overrides the default right content if provided
   */
  rightContent?: React.ReactNode
}

/**
 * Language switcher dropdown.
 *
 * Persists via i18next-browser-languagedetector's localStorage cache — no
 * extra storage logic needed. react-i18next's useTranslation subscription
 * re-renders this component (and the rest of the app) on language change.
 */
function LanguageSwitcher() {
  const { i18n, t } = useTranslation()
  const currentLanguage = normalizeInterfaceLanguage(i18n.language)

  const handleChangeLanguage = (code: 'en' | 'zhCN') => {
    void i18n.changeLanguage(code)
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('common.language')}
          />
        }
      >
        <Languages className='size-4' />
        <span className='sr-only'>{t('common.language')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuItem
          onClick={() => handleChangeLanguage('en')}
          aria-current={currentLanguage === 'en' ? 'true' : undefined}
        >
          {t('common.languageName.en')}
          {currentLanguage === 'en' && <Check className='ms-auto size-4' />}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handleChangeLanguage('zhCN')}
          aria-current={currentLanguage === 'zhCN' ? 'true' : undefined}
        >
          {t('common.languageName.zhCN')}
          {currentLanguage === 'zhCN' && <Check className='ms-auto size-4' />}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AppHeader({
  showThemeToggle = true,
  leftContent,
  rightContent,
}: AppHeaderProps) {
  const { theme, setTheme } = useTheme()

  const toggleTheme = () => {
    setTheme(theme === 'dark' ? 'light' : 'dark')
  }

  return (
    <header
      className={cn(
        'bg-background/95 supports-[backdrop-filter]:bg-background/60',
        'sticky top-0 z-50 w-full',
        'flex h-14 items-center gap-2 border-b px-4',
        '[--app-header-height:3.5rem]'
      )}
    >
      {leftContent ?? (
        <div className='flex items-center gap-2'>
          <img
            src={metapiIdentity.logoPath}
            alt={metapiIdentity.name}
            className='size-6 rounded-sm'
          />
          <span className='text-sm font-semibold tracking-tight'>
            {metapiIdentity.name}
          </span>
        </div>
      )}

      {rightContent ?? (
        <div className='ms-auto flex items-center gap-1'>
          <LanguageSwitcher />
          {showThemeToggle && (
            <Button
              variant='ghost'
              size='icon'
              onClick={toggleTheme}
              aria-label='Toggle theme'
            >
              <Sun className='size-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0' />
              <Moon className='absolute size-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100' />
            </Button>
          )}
        </div>
      )}
    </header>
  )
}
