// metapi-go/features/settings/components — shared layout primitives used by
// every real section. Keeping the Card header + save-button layout here lets
// each section file focus on its own fields instead of repeating markup.

import type { ReactNode } from 'react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type SettingsSectionCardProps = {
  title: string
  description?: string
  /** Slot for the section content (form fields, tables, etc.). */
  children: ReactNode
  /** Optional right-aligned actions in the header (test/save buttons). */
  actions?: ReactNode
}

/**
 * Card shell with a translated title + description. Header actions (test /
 * save) render right-aligned so long sections stay scannable.
 */
export function SettingsSectionCard({
  title,
  description,
  children,
  actions,
}: SettingsSectionCardProps) {
  return (
    <Card>
      <CardHeader className='flex flex-row items-start justify-between gap-4'>
        <div className='space-y-1'>
          <CardTitle>{title}</CardTitle>
          {description ? (
            <CardDescription>{description}</CardDescription>
          ) : null}
        </div>
        {actions ? <div className='flex shrink-0 gap-2'>{actions}</div> : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

/** Loading placeholder rendered while the runtime-settings query is fetching. */
export function SettingsSectionSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className='h-5 w-40' />
        <Skeleton className='h-4 w-64' />
      </CardHeader>
      <CardContent className='space-y-4'>
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-1/2' />
      </CardContent>
    </Card>
  )
}
