// metapi-go/routes — downstream keys management (promoted from Settings to a
// first-class left-nav surface).
//
// The route is intentionally thin: the page component owns the header + the
// lazy section chunk. No route loader is wired because the section fetches
// `/api/downstream-keys` via its own query when it mounts.

import { createFileRoute } from '@tanstack/react-router'

import { DownstreamKeysPage } from '@/features/downstream-keys/downstream-keys-page'

export const Route = createFileRoute('/_authenticated/downstream-keys')({
  staticData: { title: 'downstreamKeys.page.title' },
  component: DownstreamKeysPage,
})
