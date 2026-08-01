import { chromium } from '@playwright/test';
const base = 'http://127.0.0.1:4000';
const token = 'test-token-123';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
// Emulate OS-level reduced motion BEFORE navigation.
await page.emulateMedia({ reducedMotion: 'reduce' });
await page.addInitScript(({ token }) => {
  try {
    localStorage.setItem('theme_mode', 'light');
    localStorage.setItem('app_language', 'zh');
    localStorage.setItem('auth_token', token);
    localStorage.setItem('auth_token_expires_at', String(Date.now() + 12 * 60 * 60 * 1000));
  } catch {}
}, { token });
await page.goto(base + '/', { waitUntil: 'networkidle', timeout: 30000 }).catch(() => page.goto(base + '/', { waitUntil: 'domcontentloaded' }));
await page.waitForTimeout(2500);
const info = await page.evaluate(() => {
  let sec = 0, fallback = 0, canvases = 0;
  for (const c of document.querySelectorAll('canvas')) {
    canvases++;
    try {
      const ctx = c.getContext('2d');
      const { width, height } = c;
      const img = ctx.getImageData(0, 0, width, height).data;
      for (let i = 0; i < img.length; i += 4) {
        const d1 = (img[i]-0x5f)**2 + (img[i+1]-0x63)**2 + (img[i+2]-0x68)**2;
        const d2 = (img[i]-0x9c)**2 + (img[i+1]-0xa3)**2 + (img[i+2]-0xaf)**2;
        if (d1 <= 18*18) sec++;
        if (d2 <= 18*18) fallback++;
      }
    } catch {}
  }
  return {
    reducedMotion: matchMedia('(prefers-reduced-motion: reduce)').matches,
    canvases,
    secPixels: sec,
    fallbackPixels: fallback,
  };
});
console.log(JSON.stringify(info));
await browser.close();
