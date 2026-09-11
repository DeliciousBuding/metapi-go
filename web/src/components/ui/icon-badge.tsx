// metapi-go/ui — IconBadge: a soft solid-tone icon chip for stat cards and
// section headers.
//
// Tones map 1:1 onto the OKLCH status tokens declared in styles/theme.css
// (DESIGN.md §2.4 status semantics), so every tone stays theme- and
// preset-aware. Solid soft fills only — DESIGN.md §1 forbids gradients. The
// icon is decorative by default (aria-hidden); the surrounding label carries
// the meaning.

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
        success: 'bg-success/10 text-success-soft-fg',
        warning: 'bg-warning/10 text-warning-soft-fg',
        info: 'bg-info/10 text-info-soft-fg',
        destructive: 'bg-destructive/10 text-destructive-soft-fg',
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

export function IconBadge({
  children,
  tone,
  size,
  className,
  decorative = true,
}: IconBadgeProps) {
  return (
    <span
      className={cn(iconBadgeVariants({ tone, size }), className)}
      aria-hidden={decorative}
    >
      {children}
    </span>
  )
}
