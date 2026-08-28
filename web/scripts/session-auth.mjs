// metapi-go — shared admin-session auth seed for browser harness scripts.
//
// Since the session model (#1034) the SPA no longer reads an admin token from
// localStorage (the legacy `auth_token` keys are actively wiped on every
// session access). Browser contexts must authenticate the way the UI does:
// POST /api/auth/login, which validates the master token server-side and sets
// the HttpOnly `metapi_session` cookie. `context.request` shares the browser
// context's cookie jar, so one login call authenticates every page and API
// call made through that context.
//
// The Bearer master-token track stays valid for direct API probes (dual-track
// auth in auth/admin.go); only browser-page auth needed this migration.
export async function loginSession(context, { baseUrl, token }) {
  const response = await context.request.post(`${baseUrl}/api/auth/login`, {
    data: { token },
  })
  if (!response.ok()) {
    const body = await response.text().catch(() => '')
    throw new Error(`session login failed: HTTP ${response.status()} ${body}`)
  }
}
