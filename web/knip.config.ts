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
  ],
}

export default config
