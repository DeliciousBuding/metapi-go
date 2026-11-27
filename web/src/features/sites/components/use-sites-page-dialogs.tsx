import { useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { ImportWizardDialog } from '@/features/import'
import { toast } from '@/lib/toast'

import { useDeleteSite } from '../api'
import type { Site } from '../types'
import { SiteCreatedModal } from './site-created-modal'
import { SiteDetailSheet } from './site-detail-sheet'
import { SiteFormSheet } from './site-form-sheet'

export function useSitesPageDialogs(
  sites: Site[] | undefined,
  isLoading: boolean
) {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/sites' })
  const navigate = useNavigate()
  const deleteSite = useDeleteSite()
  const [formOpen, setFormOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [editingSite, setEditingSite] = useState<Site | null>(null)
  const [viewingSite, setViewingSite] = useState<Site | null>(null)
  const [createdSite, setCreatedSite] = useState<Site | null>(null)
  const [deletingSite, setDeletingSite] = useState<Site | null>(null)
  // Consume the one-shot `?create=1` deep link exactly once: open the create
  // dialog (in create mode — no edit target), then strip the transient
  // `create` param from the URL so a refetch / remount never reopens it.
  // Mirrors the accounts page's create handling; the sites version is simpler
  // — no `siteId` preselection, so it does not wait for the list snapshot.
  // The `useRef` guard survives the strict-mode double-invoke and the
  // post-navigate re-render (search.create becomes undefined).
  const createConsumed = useRef(false)
  useEffect(() => {
    if (createConsumed.current || search.create !== true) return
    setEditingSite(null)
    setFormOpen(true)
    createConsumed.current = true
    navigate({
      to: '/sites',
      search: { ...search, create: undefined },
      replace: true,
    })
  }, [search, navigate])

  // Consume the one-shot `?edit=<id>` deep link exactly once: resolve the
  // referenced id against the loaded sites snapshot, open the edit dialog
  // for it, then strip the transient `edit` param so a refetch / remount
  // never reopens it. Waits for the list to resolve (isLoading false) so a
  // stale or unknown id is ignored instead of opening a blank dialog —
  // mirrors the accounts page's deep-link wait. The `useRef` guard survives
  // the strict-mode double-invoke and the post-navigate re-render.
  const editConsumed = useRef(false)
  useEffect(() => {
    if (editConsumed.current || search.edit === undefined || isLoading) {
      return
    }
    const site = sites?.find((s) => s.id === search.edit)
    editConsumed.current = true
    if (site) {
      setEditingSite(site)
      setFormOpen(true)
    }
    navigate({
      to: '/sites',
      search: { ...search, edit: undefined },
      replace: true,
    })
  }, [search, isLoading, sites, navigate])

  function handleAddSite() {
    setEditingSite(null)
    setFormOpen(true)
  }

  async function confirmDelete() {
    if (!deletingSite) return
    const site = deletingSite
    try {
      await deleteSite.mutateAsync(site.id)
      toast.success(t('sites.toast.deleted', { name: site.name }))
    } catch {
      // http-client toasted
    } finally {
      setDeletingSite(null)
    }
  }

  function openEdit(site: Site) {
    setEditingSite(site)
    setFormOpen(true)
  }

  return {
    openCreate: handleAddSite,
    openImport: () => setImportOpen(true),
    rowActions: {
      onEdit: openEdit,
      onView: setViewingSite,
      onDelete: setDeletingSite,
    },
    dialogs: (
      <>
        <SiteFormSheet
          open={formOpen}
          onOpenChange={setFormOpen}
          editingSite={editingSite}
          onCreated={setCreatedSite}
        />

        <ImportWizardDialog open={importOpen} onOpenChange={setImportOpen} />

        <SiteCreatedModal
          site={createdSite}
          open={createdSite !== null}
          onOpenChange={(open) => {
            if (!open) setCreatedSite(null)
          }}
        />

        <SiteDetailSheet
          site={viewingSite}
          open={viewingSite !== null}
          onOpenChange={(open) => {
            if (!open) setViewingSite(null)
          }}
          onEdit={(site) => {
            setViewingSite(null)
            openEdit(site)
          }}
        />

        <Dialog
          open={deletingSite !== null}
          onOpenChange={(open) => {
            if (!open) setDeletingSite(null)
          }}
        >
          <DialogContent className='sm:max-w-sm'>
            <DialogHeader>
              <DialogTitle>{t('sites.deleteConfirm.title')}</DialogTitle>
              <DialogDescription>
                {deletingSite
                  ? t('sites.deleteConfirm.body', { name: deletingSite.name })
                  : t('sites.deleteConfirm.bodyFallback')}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant='outline'
                onClick={() => setDeletingSite(null)}
                disabled={deleteSite.isPending}
              >
                {t('sites.deleteConfirm.cancel')}
              </Button>
              <Button
                variant='destructive'
                onClick={confirmDelete}
                disabled={deleteSite.isPending}
              >
                {deleteSite.isPending && <Spinner />}
                {t('sites.deleteConfirm.confirm')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </>
    ),
  }
}
