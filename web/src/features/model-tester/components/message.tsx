// Adapted from Vercel AI Elements' Message / MessageContent / MessageResponse.
// https://registry.ai-sdk.dev/message.json (Apache-2.0)
// Copyright 2023 Vercel, Inc. See THIRD_PARTY_NOTICES.md.
// Keep only the pieces used by this playground; controls use the app's Base UI.
import type { HTMLAttributes } from 'react'

import { MarkdownContent } from '@/components/common/markdown-content'
import { cn } from '@/lib/utils'

export function Message({
  from,
  className,
  ...props
}: HTMLAttributes<HTMLDivElement> & { from: 'user' | 'assistant' }) {
  return (
    <div
      className={cn(
        'group flex w-full min-w-0 flex-col gap-2',
        from === 'user'
          ? 'is-user ml-auto max-w-[90%] items-end'
          : 'is-assistant',
        className
      )}
      {...props}
    />
  )
}

export function MessageContent({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex max-w-full min-w-0 flex-col gap-2 text-sm leading-relaxed break-words',
        'group-[.is-user]:rounded-xl group-[.is-user]:bg-muted group-[.is-user]:px-4 group-[.is-user]:py-3',
        className
      )}
      {...props}
    />
  )
}

export const MessageResponse = MarkdownContent
