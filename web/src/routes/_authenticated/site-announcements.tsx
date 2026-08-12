// metapi-go/routes — legacy /site-announcements redirect.
// The management surface now lives in Settings > Data & Messaging >
// Announcements (settings-v2 consolidation, single source of truth). Keep
// old links / bookmarks landing on the canonical surface instead of a 404.

import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/site-announcements')({
  beforeLoad: () => {
    throw redirect({
      to: '/settings/$subarea/$section',
      params: { subarea: 'content', section: 'announcements' },
    })
  },
})
