import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import {
  requestStatuses,
  quotaStates,
} from '@/features/user-portal/request-search'
import { UserPortalPage } from '@/features/user-portal/user-portal-page'

const searchSchema = z.object({
  view: z
    .enum(['overview', 'models', 'keys', 'requests', 'account'])
    .catch('overview'),
  page: z.coerce.number().int().min(1).max(1_000_000).catch(1),
  requestId: z.string().min(1).max(200).optional().catch(undefined),
  q: z.string().max(200).optional().catch(undefined),
  status: z.enum(requestStatuses).optional().catch(undefined),
  quotaState: z.enum(quotaStates).optional().catch(undefined),
  model: z.string().min(1).max(200).optional().catch(undefined),
  protocol: z
    .enum(['chat/completions', 'responses', 'messages', 'embeddings'])
    .optional()
    .catch(undefined),
})
function UserRoute() {
  return <UserPortalPage {...Route.useSearch()} />
}
export const Route = createFileRoute('/user')({
  validateSearch: searchSchema,
  component: UserRoute,
  staticData: { title: 'userPortal.title' },
})
