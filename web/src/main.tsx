// metapi-go — application entry (main.tsx).
// Orchestrates: QueryClient + Router (shared instances from lib/router, so
// non-React call sites can navigate too) → RouterProvider → 3-layer Provider
// stack (Theme → Direction → ThemeCustomization).

import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { setNonce } from 'get-nonce'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { getCspNonce } from '@/lib/csp-nonce'
import { queryClient, router } from '@/lib/router'

// Global styles (Tailwind 4 entry + theme tokens)
import './styles/index.css'
import { initI18n } from './i18n/config'

// CSP handshake (#1035 S2): hand the per-request nonce (injected by the Go
// server as <meta name="csp-nonce">) to get-nonce before the router renders,
// so react-style-singleton stamps the <style> tags it injects for dialog /
// command-palette scroll locking with the nonce attribute. ESM evaluates all
// static imports first, but the scroll-lock style injection happens on
// component mount — after this call. No-ops when the meta tag is absent
// (dev server / no CSP).
const cspNonce = getCspNonce()
if (cspNonce) {
  setNonce(cspNonce)
}

const rootElement = document.querySelector<HTMLElement>('#root')
if (!rootElement) {
  throw new Error('Root element not found')
}

// Load the active language bundle before first paint so the SPA never
// renders untranslated keys. The sibling locale lazy-loads on first switch.
await initI18n()

if (!rootElement.innerHTML) {
  const root = ReactDOM.createRoot(rootElement)
  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <DirectionProvider>
            <ThemeCustomizationProvider>
              <RouterProvider router={router} />
            </ThemeCustomizationProvider>
          </DirectionProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>
  )
}
