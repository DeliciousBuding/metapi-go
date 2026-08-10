package service

// EffectiveProxyTokensSQL returns a SQL expression that prefers total_tokens
// and falls back to prompt_tokens + completion_tokens. Avoids under-counting
// partial upstream usage payloads and avoids double-counting when both are set.
// Requires the proxy_logs alias `pl`; site filtering (if any) is done by the
// caller's WHERE clause.
const EffectiveProxyTokensSQL = `CASE
	WHEN COALESCE(pl.total_tokens, 0) > 0 THEN COALESCE(pl.total_tokens, 0)
	ELSE COALESCE(pl.prompt_tokens, 0) + COALESCE(pl.completion_tokens, 0)
END`

// EffectiveProxyTokensOnActiveSitesSQL is EffectiveProxyTokensSQL plus a
// site-status branch that zeroes rows of disabled sites. Use only for LEFT
// JOIN paths that keep all rows for counting but must not pollute token
// aggregates; requires the `s` alias. Under an INNER JOIN + active filter the
// branch is a no-op, so prefer the base expression there.
const EffectiveProxyTokensOnActiveSitesSQL = `CASE
	WHEN s.status != 'active' THEN 0
	WHEN COALESCE(pl.total_tokens, 0) > 0 THEN COALESCE(pl.total_tokens, 0)
	ELSE COALESCE(pl.prompt_tokens, 0) + COALESCE(pl.completion_tokens, 0)
END`
