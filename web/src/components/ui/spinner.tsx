// metapi-go/ui — spinner component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
import { Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type SpinnerProps = Omit<
  React.ComponentProps<typeof HugeiconsIcon>,
  'icon' | 'strokeWidth'
> & {
  strokeWidth?: number
}

function Spinner({ className, strokeWidth = 2, ...props }: SpinnerProps) {
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

export { Spinner }
