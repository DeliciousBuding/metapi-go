import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: [
    'src/components/ui/**',
    // Type declaration files for JS modules under shared/ — knip cannot trace
    // .d.ts consumers via the .js import path, so these show as unused files.
    'shared/**/*.d.ts',
  ],
}

export default config
