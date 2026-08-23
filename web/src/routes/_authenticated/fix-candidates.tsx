// metapi-go/routes — legacy /fix-candidates redirect (Wave 8 Lane B).
// The standalone fix-candidates page was folded into the model-name
// redirects panel (Settings > Models > Redirects), which already ships the
// full preview/apply flow via POST /api/model-redirects/apply. Keep old
// links / bookmarks landing on the canonical surface instead of a 404.

import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/fix-candidates')({
  beforeLoad: () => {
    throw redirect({
      to: '/settings/$subarea/$section',
      params: { subarea: 'models', section: 'redirects' },
    })
  },
})
