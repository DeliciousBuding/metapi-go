// metapi-go/features/sites — read-only `customHeaders` example snippets (#1132).
//
// Deliberately NOT templates: no table, no setting, no entity, no sync
// semantics. The snippets are compiled into the bundle and their only effect
// is filling the textarea in front of the user, who owns the result from then
// on. Rebuilding them as persisted templates is the design #1132 was scoped
// down away from.
//
// Version numbers stay `<PLACEHOLDER>` tokens on purpose. The real User-Agent
// of these CLIs changes with every upstream release, so a baked-in "current"
// value would rot into a wrong-but-authoritative-looking constant — and a
// stale template is worse than none because users treat it as the real value.
//
// The header *shapes* are grounded in this repo's own upstream adapters rather
// than guessed:
//   Claude Code → proxy/profiles/claude_code.go (`^claude-cli/<semver>` + `x-app: cli`)
//   Codex CLI   → service/oauth/codex.go (`codex-cli` UA + `originator: codex_cli_rs`)
//   Gemini CLI  → service/oauth/gemini_cli.go buildGeminiCliProxyHeaders
// Every key is one a site is allowed to inject: none is denied by
// platform.IsDeniedCustomHeader or service.isReservedPlatformCustomHeader.
// Both invariants are gated in ../__tests__/custom-headers-examples.test.ts.

export const CUSTOM_HEADERS_EXAMPLES: readonly {
  /** Client product name — identical in both locales, so not translated. */
  client: string
  /** Read-only JSON object inserted verbatim into the `customHeaders` field. */
  snippet: string
}[] = [
  {
    client: 'Claude Code',
    snippet: '{"User-Agent":"claude-cli/<VERSION>","x-app":"cli"}',
  },
  {
    client: 'Codex CLI',
    snippet: '{"User-Agent":"codex-cli/<VERSION>","originator":"codex_cli_rs"}',
  },
  {
    client: 'Gemini CLI',
    snippet:
      '{"User-Agent":"GeminiCLI/<VERSION>/<COMMIT> (<OS>; <ARCH>)","X-Goog-Api-Client":"google-genai-sdk/<VERSION> gl-node/<NODE_VERSION>"}',
  },
]
