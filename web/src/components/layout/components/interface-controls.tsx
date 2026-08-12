// metapi-go/components — shared language, appearance, and color-scheme controls.

import { Check, Languages, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ThemeCustomizer } from '@/components/theme-customizer'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/context/theme-provider'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { cn } from '@/lib/utils'

type InterfaceControlsProps = {
  className?: string
  showCustomizer?: boolean
  showThemeToggle?: boolean
}

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

export function InterfaceControls({
  className,
  showCustomizer = true,
  showThemeToggle = true,
}: InterfaceControlsProps) {
  const { t } = useTranslation()
  const { resolvedTheme, setTheme } = useTheme()

  const toggleTheme = () => {
    setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')
  }

  return (
    <div className={cn('flex items-center gap-1', className)}>
      <LanguageSwitcher />
      {showCustomizer && <ThemeCustomizer />}
      {showThemeToggle && (
        <Button
          variant='ghost'
          size='icon'
          onClick={toggleTheme}
          aria-label={t('Toggle theme')}
        >
          <Sun className='size-4 scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90' />
          <Moon className='absolute size-4 scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0' />
        </Button>
      )}
    </div>
  )
}
