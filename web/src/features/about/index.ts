// metapi-go/features/about — barrel re-exports.

export { AboutPage } from './components/about-page'

export { useAboutInfo } from './api'

export {
  aboutKeys,
  KEY_DEPENDENCIES,
  type AboutInfo,
  type AboutDependency,
} from './types'
