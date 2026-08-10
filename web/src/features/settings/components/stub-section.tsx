// metapi-go/features/settings — stub section content for phase 2.
// Every section under sections/<subarea>/ renders a StubSection until phase 3
// migrates the real form from the legacy Settings.tsx / pages. The `legacyRef`
// string is a pointer for the phase 3 migration (which legacy card/page to
// port), not a runtime dependency.

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type StubSectionProps = {
  title: string
  description?: string
  /** Legacy source pointer for the phase 3 migration. */
  legacyRef?: string
}

export function StubSection({
  title,
  description,
  legacyRef,
}: StubSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? (
          <CardDescription>{description}</CardDescription>
        ) : null}
      </CardHeader>
      <CardContent>
        <div className='flex min-h-32 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-10 text-center'>
          <p className='text-sm font-medium text-muted-foreground'>
            TODO phase 3: migrate content from legacy
          </p>
          {legacyRef ? (
            <code className='text-xs text-muted-foreground/70'>
              {legacyRef}
            </code>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
