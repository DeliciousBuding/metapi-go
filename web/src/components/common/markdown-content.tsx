// Markdown composition adapted from Vercel AI Elements MessageResponse.
// Copyright 2023 Vercel, Inc. Apache-2.0; see THIRD_PARTY_NOTICES.md.
import { memo } from 'react'
import { Streamdown } from 'streamdown'

export const MarkdownContent = memo(function MarkdownContent({
  children,
}: {
  children: string
}) {
  return (
    <Streamdown
      mode='static'
      controls={false}
      skipHtml
      components={{
        pre: ({ children }) => (
          <pre
            tabIndex={0}
            className='bg-muted focus-visible:outline-ring max-w-full overflow-x-auto rounded-lg border p-3 text-xs'
          >
            {children}
          </pre>
        ),
        a: ({ href, children }) =>
          href ? (
            <a href={href} target='_blank' rel='noopener noreferrer'>
              {children}
            </a>
          ) : (
            <span>{children}</span>
          ),
        img: ({ alt }) => <span>{alt}</span>,
      }}
      className='[&_pre]:bg-muted [&_a]:text-primary min-w-0 space-y-3 [overflow-wrap:anywhere] [&_a]:underline [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:text-sm [&_h3]:font-semibold [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:max-w-full [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:p-3 [&_pre]:text-xs [&_table]:text-xs [&_ul]:list-disc [&_ul]:pl-5'
    >
      {children}
    </Streamdown>
  )
})
