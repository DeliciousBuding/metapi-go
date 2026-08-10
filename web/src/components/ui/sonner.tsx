// metapi-go/ui — sonner component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
'use client'

import {
  CheckmarkCircle02Icon,
  InformationCircleIcon,
  Alert02Icon,
  MultiplicationSignCircleIcon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'

// TODO: depends on @/context/theme-provider (useTheme) — provided by other subagent (context layer)
import { useTheme } from '@/context/theme-provider'

const Toaster = (props: ToasterProps) => {
  const { resolvedTheme } = useTheme()

  return (
    <Sonner
      theme={resolvedTheme}
      className='toaster group'
      icons={{
        success: (
          <HugeiconsIcon
            icon={CheckmarkCircle02Icon}
            strokeWidth={2}
            className='size-4'
          />
        ),
        info: (
          <HugeiconsIcon
            icon={InformationCircleIcon}
            strokeWidth={2}
            className='size-4'
          />
        ),
        warning: (
          <HugeiconsIcon
            icon={Alert02Icon}
            strokeWidth={2}
            className='size-4'
          />
        ),
        error: (
          <HugeiconsIcon
            icon={MultiplicationSignCircleIcon}
            strokeWidth={2}
            className='size-4'
          />
        ),
        loading: (
          <HugeiconsIcon
            icon={Loading03Icon}
            strokeWidth={2}
            className='size-4 animate-spin'
          />
        ),
      }}
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
          '--success-bg':
            'color-mix(in oklch, var(--success) 16%, var(--popover))',
          '--success-border':
            'color-mix(in oklch, var(--success) 35%, var(--border))',
          '--success-text': 'var(--success)',
          '--info-bg': 'color-mix(in oklch, var(--info) 16%, var(--popover))',
          '--info-border':
            'color-mix(in oklch, var(--info) 35%, var(--border))',
          '--info-text': 'var(--info)',
          '--warning-bg':
            'color-mix(in oklch, var(--warning) 18%, var(--popover))',
          '--warning-border':
            'color-mix(in oklch, var(--warning) 38%, var(--border))',
          '--warning-text': 'var(--warning)',
          '--error-bg':
            'color-mix(in oklch, var(--destructive) 16%, var(--popover))',
          '--error-border':
            'color-mix(in oklch, var(--destructive) 35%, var(--border))',
          '--error-text': 'var(--destructive)',
          '--border-radius': 'var(--radius)',
        } as React.CSSProperties
      }
      {...props}
    />
  )
}

export { Toaster }
