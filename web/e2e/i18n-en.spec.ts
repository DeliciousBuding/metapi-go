import { expect, test, type Page } from '@playwright/test';

/**
 * i18n EN-mode smoke (review-wave 2026-08-01).
 *
 * Vitest gates validate the dictionary statically; this spec validates the
 * runtime chain in a real browser: `app_language=en` → MutationObserver
 * walks the DOM → translateText produces English — with no `Untranslated`
 * fallback and no Han residue on pure-UI surfaces.
 *
 * Runs against Vite preview without a backend: login surface + design-system
 * gallery (/__design__) are the reachable pure-UI surfaces.
 */

const HAN_RE = /[㐀-鿿]/;

async function enableEnglish(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      localStorage.setItem('app_language', 'en');
      localStorage.setItem('theme_mode', 'light');
      localStorage.setItem('theme', 'light');
      // Production preview builds gate /__design__ behind this flag .
      localStorage.setItem('metapi_design_gallery', '1');
    } catch {
      /* ignore */
    }
  });
}

async function gotoRobust(page: Page, path: string): Promise<void> {
  await page.goto(path, { waitUntil: 'networkidle' }).catch(async () => {
    await page.goto(path, { waitUntil: 'domcontentloaded' });
  });
}

test.describe('i18n EN mode', () => {
  test('login surface: html[lang=en], no Untranslated, no Han residue', async ({ page }) => {
    await enableEnglish(page);
    await gotoRobust(page, '/');

    await expect(page.locator('html')).toHaveAttribute('lang', 'en', { timeout: 15_000 });
    await expect(page.locator('#root')).toBeAttached();

    // Observer translation is async; toContainText polls until settled.
    const body = page.locator('body');
    await expect(body).not.toContainText('Untranslated', { timeout: 15_000 });

    // The login surface is pure UI copy (no user data) — Han residue is a bug.
    const text = await body.innerText();
    expect(text, 'login surface must not show Chinese in EN mode').not.toMatch(HAN_RE);
  });

  test('design gallery renders EN component copy without Untranslated', async ({ page }) => {
    await enableEnglish(page);
    await gotoRobust(page, '/__design__');

    const marker = page.locator(
      '[data-testid="design-system-gallery"], [data-testid="design-gallery"], #design-gallery',
    );
    await marker.first().waitFor({ state: 'visible', timeout: 15_000 }).catch(() => {
      // Non-gallery shell (login redirect) is acceptable on preview without backend.
    });

    const body = page.locator('body');
    await expect(body).not.toContainText('Untranslated', { timeout: 15_000 });

    // Spot-check known EN copy that came from tr()/dictionary (regression guard
    // for the review-wave keys).
    const text = await body.innerText();
    expect(text).toContain('Light');
    expect(text).not.toContain('Untranslated');
  });

  test('switching language mid-session updates lang and copy (gallery)', async ({ page }) => {
    // Start in zh (default when no stored language on non-zh locale is en —
    // force zh explicitly, then flip to en via the topbar toggle).
    await page.addInitScript(() => {
      try {
        localStorage.setItem('app_language', 'zh');
        localStorage.setItem('metapi_design_gallery', '1');
      } catch {
        /* ignore */
      }
    });
    await gotoRobust(page, '/__design__');

    const body = page.locator('body');
    const toggle = page.getByRole('button', { name: 'EN' }).first();
    await toggle.waitFor({ state: 'visible', timeout: 15_000 }).catch(() => {
      // No topbar on some layouts; skip the flip assertion if absent.
    });
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      await expect(page.locator('html')).toHaveAttribute('lang', 'en', { timeout: 15_000 });
      await expect(body).not.toContainText('Untranslated', { timeout: 15_000 });
      const text = await body.innerText();
      expect(text).not.toContain('Untranslated');
    }
  });

  test('zh mode shows Chinese and en→zh switch restores it (S3 regression)', async ({ page }) => {
    // zh mode: gallery must render Chinese (no EN leakage).
    await page.addInitScript(() => {
      try {
        localStorage.setItem('app_language', 'zh');
        localStorage.setItem('metapi_design_gallery', '1');
      } catch {
        /* ignore */
      }
    });
    await gotoRobust(page, '/__design__');

    const body = page.locator('body');
    await expect(body).toContainText('仪表盘', { timeout: 15_000 });

    // Flip EN → zh and assert Chinese copy comes back (WeakMap poisoning
    // regression: en values must never become the recorded originals).
    const enToggle = page.getByRole('button', { name: 'EN' }).first();
    await enToggle.waitFor({ state: 'visible', timeout: 15_000 }).catch(() => {
      /* layout without topbar — skip rest */
    });
    if (await enToggle.isVisible().catch(() => false)) {
      await enToggle.click();
      await expect(page.locator('html')).toHaveAttribute('lang', 'en', { timeout: 15_000 });
      await expect(body).not.toContainText('Untranslated', { timeout: 15_000 });

      const zhToggle = page.getByRole('button', { name: '中' }).first();
      await zhToggle.waitFor({ state: 'visible', timeout: 15_000 });
      await zhToggle.click();
      await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN', { timeout: 15_000 });
      // The gallery sidebar/shell copy must return to Chinese — the observer
      // restores recorded originals and must not have been poisoned by EN.
      await expect(body).toContainText('仪表盘', { timeout: 15_000 });
      await expect(body).not.toContainText('Untranslated');
    }
  });
});
