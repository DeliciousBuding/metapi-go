// metapi-go/components/layout — route-level error boundary page.
//
// Mounted via the router's defaultErrorComponent: render-time crashes land
// here with a retry/reload path instead of a white screen. The raw error
// message stays hidden by default (debug-only disclosure) to avoid leaking
// internals; the console still logs the full error.

import { useRouter } from '@tanstack/react-router'
import { RefreshCw, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function ErrorPage({ error }: { error: Error }) {
  const { t } = useTranslation()
  const router = useRouter()
  const [showDetail, setShowDetail] = useState(false)

  return (
    <div className='flex min-h-screen flex-col items-center justify-center gap-4 p-6'>
      <div className='bg-destructive/10 flex size-16 items-center justify-center rounded-2xl'>
        <TriangleAlert className='text-destructive size-8' />
      </div>
      <div className='space-y-1 text-center'>
        <p className='text-2xl font-semibold'>{t('errors.renderTitle')}</p>
        <p className='text-muted-foreground text-sm'>
          {t('errors.renderDescription')}
        </p>
      </div>
      <div className='flex gap-2'>
        <Button onClick={() => router.invalidate()}>
          <RefreshCw className='size-4' />
          {t('errors.retry')}
        </Button>
        <Button variant='outline' onClick={() => window.location.reload()}>
          {t('errors.reload')}
        </Button>
      </div>
      <button
        type='button'
        className='text-muted-foreground text-xs underline underline-offset-2'
        onClick={() => setShowDetail((v) => !v)}
      >
        {t('errors.showDetail')}
      </button>
      {showDetail ? (
        <pre className='bg-muted/60 max-w-xl overflow-auto rounded-lg border p-3 text-xs'>
          {error?.message ?? String(error)}
        </pre>
      ) : null}
    </div>
  )
}
