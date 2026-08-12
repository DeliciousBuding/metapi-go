import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: [
    'src/components/ui/**',
    // Type declaration files for JS modules under shared/ — knip cannot trace
    // .d.ts consumers via the .js import path, so these show as unused files.
    'shared/**/*.d.ts',
  ],
  ignoreDependencies: [
    // scripts/a11y-scan.mjs injects node_modules/axe-core/axe.min.js via
    // Playwright addScriptTag (a file path, not an import) — knip cannot
    // trace it, so declare it explicitly.
    'axe-core',
  ],
}

export default config
