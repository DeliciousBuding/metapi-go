import type { ReactNode } from 'react'

type PageHeaderProps = {
  title: string
  description?: string
  actions?: ReactNode
  children?: ReactNode
}

/** Shared page composition; the page owns its scroll container and content. */
export function PageHeader(props: PageHeaderProps) {
  return (
    <header className='flex shrink-0 flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-start sm:justify-between'>
      <div className='min-w-0 space-y-1 sm:flex-1 sm:basis-64'>
        <h1 className='page-title wrap-anywhere'>{props.title}</h1>
        {props.description ? (
          <p className='text-muted-foreground text-sm leading-relaxed wrap-anywhere'>
            {props.description}
          </p>
        ) : null}
        {props.children}
      </div>
      {props.actions ? (
        <div className='flex min-w-0 flex-wrap items-center gap-2 sm:max-w-full'>
          {props.actions}
        </div>
      ) : null}
    </header>
  )
}
