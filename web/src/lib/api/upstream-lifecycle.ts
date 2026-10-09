import { buildQueryString, request } from './transport'

export type UpstreamEntityKind =
  | 'channel'
  | 'model'
  | 'credential'
  | 'grant'
  | 'group'
  | 'member'
  | 'route'
export type UpstreamDeletionPreview = {
  kind: UpstreamEntityKind | 'source'
  id: number
  counts: Partial<
    Record<
      | 'channels'
      | 'models'
      | 'credentials'
      | 'grants'
      | 'groups'
      | 'members'
      | 'routes'
      | 'routeChannels'
      | 'routeGroupSources'
      | 'downstreamKeys'
      | 'sourceMappings',
      number
    >
  >
  affectedRouteIds: number[]
  revision: string
  requiresCascade: boolean
}

function entityPath(kind: UpstreamEntityKind, id: number) {
  if (kind === 'route') return `/api/routes/${id}`
  if (kind === 'channel') return `/api/imported-upstreams/${id}`
  return `/api/imported-upstreams/${kind}s/${id}`
}

export const upstreamLifecycleApi = {
  previewUpstreamDeletion: (kind: UpstreamEntityKind, id: number) =>
    request<UpstreamDeletionPreview>(
      `${entityPath(kind, id)}/deletion-preview`
    ),
  deleteUpstreamEntity: (
    kind: UpstreamEntityKind,
    id: number,
    options: { cascade?: boolean; expectedRevision: string }
  ) =>
    request<
      UpstreamDeletionPreview & {
        success: boolean
        downstreamKeysWithOnlyDeletedRoutes?: number[]
      }
    >(`${entityPath(kind, id)}${buildQueryString(options)}`, {
      method: 'DELETE',
      skipErrorHandler: true,
    }),
}
