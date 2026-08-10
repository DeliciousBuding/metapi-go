// stub — will be fully migrated from master:web/pages/token-routes/types.ts in phase 2
// TODO(phase 2): complete migration with full type definitions + BrandIcon/tokenRouteContract deps

export type RouteRowKind = 'persisted' | 'zero_channel';

export type RouteSummaryRow = {
  id: number;
  modelPattern: string;
  displayName?: string | null;
  displayIcon?: string | null;
  modelMapping?: string | null;
  routingStrategy?: string | null;
  enabled: boolean;
  channelCount: number;
  enabledChannelCount?: number;
  siteNames?: string[];
  decisionSnapshot?: unknown;
  decisionRefreshedAt?: string | null;
  kind?: RouteRowKind;
  readOnly?: boolean;
  isVirtual?: boolean;
  // TODO(phase 2): complete remaining fields from master
};
