import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: [
    // Type declaration files for JS modules under shared/ — knip cannot trace
    // .d.ts consumers via the .js import path, so these show as unused files.
    'shared/**/*.d.ts',
    // index.html loads these bootstrap entry scripts through synchronous
    // <script src> tags (FOUC theme bootstrap + body data-attribute
    // hydration). knip only traces import graphs, so it flags them as unused
    // files even though the HTML references them directly.
    'public/bootstrap.js',
    'public/theme-init.js',
    // One-off probes archived under web/scripts/oneoff/ — kept for reference
    // with no package.json entry and no importer, so knip would (correctly)
    // flag them as unused. The live inventory lives in
    // docs/internal/analysis/e2e-acceptance-platform.md.
    'scripts/oneoff/**',
  ],
  ignoreDependencies: [
    // scripts/a11y-scan.mjs injects node_modules/axe-core/axe.min.js via
    // Playwright addScriptTag (a file path, not an import) — knip cannot
    // trace it, so declare it explicitly.
    'axe-core',
  ],
}

export default config
