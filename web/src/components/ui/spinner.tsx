// metapi-go/ui — Spinner: indeterminate loading indicator.
//
// Spins the hugeicons loading glyph. By default it is a polite live region
// (role="status") carrying a translated label; callers that pair it with their
// own visible text pass aria-hidden to keep it purely decorative. Size and
// color are the caller's to set via className (size-*, text-*).

import { Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type SpinnerProps = Omit<
  React.ComponentProps<typeof HugeiconsIcon>,
  'icon' | 'strokeWidth'
> & {
  /** Icon stroke weight. @default 2 */
  strokeWidth?: number
}

export function Spinner({
  className,
  strokeWidth = 2,
  ...props
}: SpinnerProps) {
  const { t } = useTranslation()

  return (
    <HugeiconsIcon
      icon={Loading03Icon}
      strokeWidth={strokeWidth}
      role='status'
      aria-label={t('common.loading')}
      className={cn('size-4 animate-spin', className)}
      {...props}
    />
  )
}
