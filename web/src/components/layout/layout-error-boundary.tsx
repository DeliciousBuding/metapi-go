// metapi-go/components/layout — layout-preserving route error boundary.
//
// Mounted as the errorComponent on the _authenticated route so a render-time
// crash in any page (a child route) replaces only the page content, not the
// sidebar/nav shell. The router's defaultErrorComponent (ErrorPage) is
// full-screen and is still used for crashes outside the authenticated shell
// (e.g. the sign-in route or the root outlet).
//
// The error boundary reuses the same AppHeader + AppSidebar + SidebarProvider
// shell as AuthenticatedLayout, so the sidebar, brand, and interface controls
// (language/theme) stay interactive — only the page area swaps to the error
// card with Retry / Reload / show-detail controls.

import { useRouter } from '@tanstack/react-router'
import { RotateCw, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SkipToMain } from '@/components/skip-to-main'
import { Button } from '@/components/ui/button'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

import { AppHeader } from './components/app-header'
import { AppSidebar } from './components/app-sidebar'

export function LayoutErrorBoundary({ error }: { error: Error }) {
  const { t } = useTranslation()
  const router = useRouter()
  const [showDetail, setShowDetail] = useState(false)

  return (
    <SidebarProvider className='flex-col'>
      <SkipToMain />
      <AppHeader />
      <div className='flex min-h-0 w-full flex-1 [--app-header-height:3.5rem]'>
        <AppSidebar />
        <SidebarInset
          id='content'
          className={cn(
            '@container/content',
            'flex min-h-0 items-center justify-center overflow-x-hidden overflow-y-auto p-6',
            'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
          )}
        >
          <div className='flex w-full max-w-md flex-col items-center gap-4'>
            <div className='bg-destructive/10 flex size-16 items-center justify-center rounded-2xl'>
              <TriangleAlert className='text-destructive size-8' />
            </div>
            <div className='space-y-1 text-center'>
              <p className='text-2xl font-semibold'>
                {t('errors.renderTitle')}
              </p>
              <p className='text-muted-foreground text-sm'>
                {t('errors.renderDescription')}
              </p>
            </div>
            <div className='flex gap-2'>
              <Button onClick={() => router.invalidate()}>
                <RotateCw className='mr-1.5 size-4' />
                {t('errors.retry')}
              </Button>
              <Button
                variant='outline'
                onClick={() => window.location.reload()}
              >
                {t('errors.reload')}
              </Button>
            </div>
            <button
              type='button'
              className='text-muted-foreground text-xs underline underline-offset-2'
              onClick={() => setShowDetail((prev) => !prev)}
            >
              {t('errors.showDetail')}
            </button>
            {showDetail ? (
              <pre className='bg-muted/60 max-w-xl overflow-auto rounded-lg border p-3 text-xs'>
                {error?.message ?? String(error)}
              </pre>
            ) : null}
          </div>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}
