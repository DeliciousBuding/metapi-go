// Per-request CSP nonce handshake with the Go server (router/security.go).
//
// The server mints a fresh nonce per response, puts it in the
// Content-Security-Policy style-src directive, and injects
// <meta name="csp-nonce" content="..."> into the served index.html. Runtime
// <style> injectors that cannot know the nonce at build time read it here:
//
//   - main.tsx hands it to get-nonce's setNonce(), so react-style-singleton
//     (dialog/command-palette scroll-lock styles via cmdk) stamps the
//     <style> tags it injects with the nonce attribute.
//   - components/ui/chart.tsx passes it as the nonce prop of the ChartStyle
//     <style> element.
//
// Returns '' when the meta tag is absent (rsbuild dev server, vitest/jsdom,
// or a proxied HTML that bypassed the Go handler). Callers treat '' as
// "no nonce available" and simply skip stamping — which is correct wherever
// no CSP applies, and degrades to blocked styles only if a strict policy is
// served without the meta (a server bug the Go tests pin against).
export function getCspNonce(): string {
  return (
    document.querySelector<HTMLMetaElement>('meta[name="csp-nonce"]')
      ?.content ?? ''
  )
}
