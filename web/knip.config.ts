import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: ['src/components/ui/**', 'src/routeTree.gen.ts', 'shared/proxyLogMeta.d.ts'],
  ignoreDependencies: ['tailwindcss', 'tw-animate-css'],
}

export default config
