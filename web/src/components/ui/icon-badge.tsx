// metapi-go/ui — icon badge (soft solid-tone chip for stat cards / headers).
//
// Conceptually borrowed from newapi's icon-badge (quantumnous, AGPL) and
// trimmed to the metapi-go semantic tokens. Solid soft fills only —
// DESIGN.md §1 forbids gradients. Tones map 1:1 onto the OKLCH status tokens
// declared in styles/theme.css, so every tone stays theme- and preset-aware.

import { cva, type VariantProps } from 'class-variance-authority'
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

const iconBadgeVariants = cva(
  'flex shrink-0 items-center justify-center [&>svg]:shrink-0',
  {
    variants: {
      tone: {
        default: 'bg-muted text-muted-foreground',
        primary: 'bg-primary/10 text-primary',
        success: 'bg-success/10 text-success',
        warning: 'bg-warning/10 text-warning',
        info: 'bg-info/10 text-info',
        destructive: 'bg-destructive/10 text-destructive',
      },
      size: {
        sm: 'size-7 rounded-md [&>svg]:size-3.5',
        md: 'size-8 rounded-lg [&>svg]:size-4',
        lg: 'size-10 rounded-xl [&>svg]:size-5',
      },
    },
    defaultVariants: {
      tone: 'default',
      size: 'md',
    },
  }
)

type IconBadgeTone = NonNullable<VariantProps<typeof iconBadgeVariants>['tone']>
type IconBadgeSize = NonNullable<VariantProps<typeof iconBadgeVariants>['size']>

type IconBadgeProps = {
  children?: ReactNode
  tone?: IconBadgeTone
  size?: IconBadgeSize
  className?: string
  /** Icons are decorative by default; the surrounding label carries meaning. */
  decorative?: boolean
}

export function IconBadge(props: IconBadgeProps) {
  return (
    <span
      className={cn(
        iconBadgeVariants({ tone: props.tone, size: props.size }),
        props.className
      )}
      aria-hidden={props.decorative ?? true}
    >
      {props.children}
    </span>
  )
}
