// metapi-go/features/oauth — Zod schemas for the start form and URL search.
//
// `oauthStartSchema` validates the "start authorization" dialog. Numeric
// fields are absent here (the form has none). Error messages are i18next
// keys (resolved by `<FormMessage>`). The conditional "projectId required
// when provider.requiresProjectId" check is enforced in the dialog's
// submit handler (it needs the runtime provider list the schema cannot see),
// not in the schema itself.
//
// `oauthSearchSchema` is the URL search-state contract for the connections
// list page (pagination / sorting / global filter / status filter). The page
// safe-parses `window.location.search` directly so it works before the
// `/oauth` route file lands its own `validateSearch`.

import { z } from 'zod'

import { isEmptyOrProxyUrl } from '@/lib/helpers/proxyUrl'
import {
  encodeSortingParam,
  stringSearchParam,
  tableSortingItemSchema,
} from '@/lib/helpers/searchParams'

const HTTP_OR_EMPTY_MESSAGE_KEY = 'oauth.form.errors.invalidProxyUrl'

export const oauthStartSchema = z.object({
  provider: z.string().trim().min(1, 'oauth.form.errors.providerRequired'),
  projectId: z.string().trim(),
  proxyUrl: z.string().refine(isEmptyOrProxyUrl, HTTP_OR_EMPTY_MESSAGE_KEY),
  useSystemProxy: z.boolean(),
})

export type OAuthStartValues = z.infer<typeof oauthStartSchema>

export const OAUTH_START_DEFAULT_VALUES: OAuthStartValues = {
  provider: '',
  projectId: '',
  proxyUrl: '',
  useSystemProxy: false,
}

// --- URL search state -------------------------------------------------------

export const oauthSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).catch(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).catch(20).default(20),
  sort: z
    .union([z.string(), z.array(tableSortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  status: stringSearchParam,
})
