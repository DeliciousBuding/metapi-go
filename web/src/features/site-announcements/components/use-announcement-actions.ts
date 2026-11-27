import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { siteAnnouncementsKeys } from '../types'

export function useAnnouncementActions() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  function invalidateAnnouncements() {
    void queryClient.invalidateQueries({
      queryKey: siteAnnouncementsKeys.all,
    })
    void queryClient.invalidateQueries({ queryKey: ['dashboard-attention'] })
  }
  const markReadMutation = useMutation({
    mutationFn: async (id: number) => api.markSiteAnnouncementRead(id),
    onSuccess: invalidateAnnouncements,
    onError: () => toast.error(t('siteAnnouncements.toast.markReadFailed')),
  })

  const markAllMutation = useMutation({
    mutationFn: async () => api.markAllSiteAnnouncementsRead(),
    onSuccess: () => {
      invalidateAnnouncements()
      toast.success(t('siteAnnouncements.toast.allMarkedRead'))
    },
    onError: () => toast.error(t('siteAnnouncements.toast.markAllFailed')),
  })

  const clearMutation = useMutation({
    mutationFn: async () => api.clearSiteAnnouncements(),
    onSuccess: () => {
      invalidateAnnouncements()
      toast.success(t('siteAnnouncements.toast.cleared'))
    },
    onError: () => toast.error(t('siteAnnouncements.toast.clearFailed')),
  })
  return { markReadMutation, markAllMutation, clearMutation }
}
