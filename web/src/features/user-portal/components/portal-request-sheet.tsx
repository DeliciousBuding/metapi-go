import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useEffect, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'

import { userPortalApi, type PersonalRequest } from '../api'
import { RequestDetail } from './portal-request-detail'

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  const role = target.getAttribute('role')
  return (
    role === 'combobox' ||
    role === 'listbox' ||
    role === 'slider' ||
    role === 'textbox'
  )
}

export function RequestSheet({
  userId,
  requestId,
  items,
  detailEntry,
  onCloseDetail,
  navigateToRequest,
}: {
  userId: number
  requestId?: string
  items: PersonalRequest[]
  detailEntry: RefObject<HTMLAnchorElement | null>
  onCloseDetail: () => void
  navigateToRequest: (id: string) => void
}) {
  const { t } = useTranslation()
  const detail = useQuery({
    queryKey: ['userPortal', userId, 'request', requestId],
    queryFn: () => {
      if (!requestId) {
        return Promise.reject(new Error('Missing request ID'))
      }
      return userPortalApi.request(requestId)
    },
    enabled: Boolean(requestId),
    refetchOnMount: 'always',
  })
  const currentIndex = requestId
    ? items.findIndex((item) => item.id === requestId)
    : -1
  const prevItem = currentIndex > 0 ? items[currentIndex - 1] : undefined
  const nextItem =
    currentIndex >= 0 && currentIndex + 1 < items.length
      ? items[currentIndex + 1]
      : undefined
  useEffect(() => {
    if (!requestId || (!prevItem && !nextItem)) return
    function handleKeyDown(event: KeyboardEvent) {
      if (
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        event.shiftKey ||
        event.isComposing ||
        event.keyCode === 229
      ) {
        return
      }
      if (isEditableTarget(event.target)) return
      if (event.key === 'ArrowUp' && prevItem) {
        event.preventDefault()
        navigateToRequest(prevItem.id)
      } else if (event.key === 'ArrowDown' && nextItem) {
        event.preventDefault()
        navigateToRequest(nextItem.id)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [requestId, prevItem, nextItem, navigateToRequest])
  function renderDetailBody() {
    if (detail.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    if (detail.isError) {
      return (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void detail.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      )
    }
    if (!detail.data) return null
    return <RequestDetail item={detail.data} />
  }
  return (
    <Sheet
      open={Boolean(requestId)}
      onOpenChange={(open) => {
        if (!open) onCloseDetail()
      }}
    >
      <SheetContent finalFocus={detailEntry} className='sm:max-w-lg'>
        <SheetHeader>
          <div className='flex items-center justify-between gap-2'>
            <SheetTitle>{t('userPortal.requestDetail')}</SheetTitle>
            {(prevItem || nextItem) && (
              <div className='flex items-center gap-1'>
                <Button
                  type='button'
                  size='icon-xs'
                  variant='outline'
                  aria-label={t('requestLogs.prevRecord')}
                  disabled={!prevItem}
                  onClick={() => prevItem && navigateToRequest(prevItem.id)}
                >
                  <ChevronUp aria-hidden='true' className='size-3.5' />
                </Button>
                <Button
                  type='button'
                  size='icon-xs'
                  variant='outline'
                  aria-label={t('requestLogs.nextRecord')}
                  disabled={!nextItem}
                  onClick={() => nextItem && navigateToRequest(nextItem.id)}
                >
                  <ChevronDown aria-hidden='true' className='size-3.5' />
                </Button>
              </div>
            )}
          </div>
          <SheetDescription>{t('userPortal.requestsHint')}</SheetDescription>
        </SheetHeader>
        <div className='min-h-0 min-w-0 flex-1 overflow-y-auto px-4 pb-4'>
          {renderDetailBody()}
        </div>
      </SheetContent>
    </Sheet>
  )
}
