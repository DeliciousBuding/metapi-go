// metapi-go/components/common — safe external-link cell shared by list
// columns that expose operator URLs (sites name/URL cells #985, accounts
// site cells #1108). One implementation of the render ladder: unsafe /
// non-http(s) / forbidden-target URLs degrade to plain text instead of
// rendering a clickable link.

import { ExternalLink as ExternalLinkIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { isValidEndpointUrl } from '@/lib/url-validation'
import { cn } from '@/lib/utils'

export type SafeExternalLinkProps = {
  url: string
  children: ReactNode
  className?: string
  title?: string
}

/**
 * Renders `children` as an external link when `url` is a valid http(s)
 * endpoint the backend would allow; otherwise renders plain truncated text.
 * The link is keyboard-focusable with the standard focus ring, opens a new
 * tab (`noopener noreferrer`), and stops propagation so row-click handlers
 * (detail sheets) are not hijacked.
 */
export function SafeExternalLink({
  url,
  children,
  className,
  title,
}: SafeExternalLinkProps) {
  const href = url.trim()
  const content = <span className='min-w-0 truncate'>{children}</span>
  const isSafeExternalURL =
    /^https?:\/\//i.test(href) && isValidEndpointUrl(href)

  if (!isSafeExternalURL) {
    return (
      <span className={cn('block truncate', className)} title={title}>
        {children}
      </span>
    )
  }

  return (
    <a
      href={href}
      target='_blank'
      rel='noopener noreferrer'
      className={cn(
        'group inline-flex min-w-0 items-center gap-1 rounded-sm underline-offset-4 hover:text-primary hover:underline focus-visible:text-primary focus-visible:underline focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-inset focus-visible:outline-none',
        className
      )}
      title={title}
      onClick={(event) => event.stopPropagation()}
    >
      {content}
      <ExternalLinkIcon
        aria-hidden='true'
        className='size-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100'
      />
    </a>
  )
}
