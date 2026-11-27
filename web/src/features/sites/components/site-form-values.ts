import type { ConnectionTemplate } from '@/lib/platform-catalog'

import {
  parseEndpointsEditorText,
  serializeEndpointsForEditor,
  type ParsedEndpoint,
} from '../lib/endpoints'
import { analyzePrimarySiteUrl } from '../lib/site-primary-url'
import type { SiteFormValues } from '../lib/sites-schema'
import type { Site, SiteFormPayload, SiteProbeScope } from '../types'

export function matchesTemplate(
  template: ConnectionTemplate,
  url: string,
  platform: string
) {
  function savedURL(value: string) {
    const analysis = analyzePrimarySiteUrl(value)
    return analysis.action === 'auto_strip_known_api_suffix'
      ? analysis.persistedUrl
      : value.trim().replace(/\/+$/, '')
  }
  return (
    platform.trim() === template.platform &&
    savedURL(url) === savedURL(template.url)
  )
}

export function siteToFormValues(site: Site): SiteFormValues {
  return {
    name: site.name ?? '',
    url: site.url ?? '',
    externalCheckinUrl: site.externalCheckinUrl ?? '',
    platform: site.platform ?? '',
    proxyUrl: site.proxyUrl ?? '',
    useSystemProxy: site.useSystemProxy ?? false,
    customHeaders: site.customHeaders ?? '',
    customHeadersOverrideRequestHeaders:
      site.customHeadersOverrideRequestHeaders ?? false,
    globalWeight: site.globalWeight ?? 1,
    maxConcurrency: site.maxConcurrency ?? 0,
    postRefreshProbeEnabled: site.postRefreshProbeEnabled ?? false,
    postRefreshProbeModel: site.postRefreshProbeModel ?? '',
    postRefreshProbeScope:
      (site.postRefreshProbeScope as SiteProbeScope | undefined) ?? 'single',
    postRefreshProbeLatencyThresholdMs:
      site.postRefreshProbeLatencyThresholdMs ?? 0,
    resinEnabled: site.resinEnabled ?? null,
    useUtls: site.useUtls ?? null,
    apiEndpointsText: serializeEndpointsForEditor(site.apiEndpoints),
  }
}

export function buildPayload(
  values: SiteFormValues,
  editingSite: Site | null,
  apiEndpointsTouched: boolean,
  primaryUrl: string
): SiteFormPayload {
  const preservedEndpoints = (editingSite?.apiEndpoints ?? []).map(
    (endpoint) => ({
      url: endpoint.url,
      enabled: endpoint.enabled ?? true,
      sortOrder: endpoint.sortOrder ?? 0,
    })
  )
  // Untouched preserve: as long as the operator did not edit the endpoints
  // editor, the original endpoint objects pass through unparsed (the
  // row-free behaviour for name-only edits). When touched, the schema
  // has already validated the text, so the parse cannot fail.
  const parsedEndpoints = parseEndpointsEditorText(values.apiEndpointsText)
  let apiEndpoints: ParsedEndpoint[] = preservedEndpoints
  if (apiEndpointsTouched && 'endpoints' in parsedEndpoints) {
    apiEndpoints = parsedEndpoints.endpoints
  }
  return {
    name: values.name,
    url: primaryUrl,
    externalCheckinUrl: values.externalCheckinUrl,
    platform: values.platform,
    proxyUrl: values.proxyUrl,
    useSystemProxy: values.useSystemProxy,
    apiEndpoints,
    customHeaders: values.customHeaders,
    customHeadersOverrideRequestHeaders:
      values.customHeadersOverrideRequestHeaders,
    globalWeight: values.globalWeight,
    maxConcurrency: values.maxConcurrency,
    postRefreshProbeEnabled: values.postRefreshProbeEnabled,
    postRefreshProbeModel: values.postRefreshProbeModel,
    postRefreshProbeScope: values.postRefreshProbeScope,
    postRefreshProbeLatencyThresholdMs:
      values.postRefreshProbeLatencyThresholdMs,
    resinEnabled: values.resinEnabled,
    useUtls: values.useUtls,
  }
}
