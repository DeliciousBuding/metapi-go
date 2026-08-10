// stub — will be fully migrated from master:web/pages/token-routes/utils.ts in phase 2
// TODO(phase 2): complete migration with full implementation + tokenRoutePatterns deps

export function isExactModelPattern(pattern: string): boolean {
  // Exact model pattern = no regex/wildcard metacharacters.
  // Phase 2 will replace with the real isExactTokenRouteModelPattern from shared/tokenRoutePatterns.js
  return !/[*[\]()?{}|^$\\]/.test(pattern);
}
