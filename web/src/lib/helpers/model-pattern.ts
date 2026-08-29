// metapi-go/lib — model pattern grammar predicates (S5 boundary inversion).
//
// Pure string predicates for the token-route model-pattern grammar
// (`re:` prefix = regex, glob metacharacters = non-exact). Shared by the
// token-routes feature and lib helpers (zeroChannelRoutes) — rule 5 of
// docs/internal/web-package-boundaries.md: a pure helper two layers need
// belongs in lib (sanitizeAuthRedirect precedent). Presentation/error
// helpers that need i18n stay in features/token-routes/utils.ts.

export function isRegexModelPattern(modelPattern: string): boolean {
  return (modelPattern || '').trim().startsWith('re:')
}

export function isExactModelPattern(modelPattern: string): boolean {
  const normalized = (modelPattern || '').trim()
  if (!normalized) return false
  if (isRegexModelPattern(normalized)) return false
  return !/[*[\]()?{}|^$\\]/.test(normalized)
}
