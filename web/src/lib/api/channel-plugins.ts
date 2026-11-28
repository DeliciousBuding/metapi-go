import { channelPluginCatalog } from './channel-plugin-catalog'
import { channelPluginManagement } from './channel-plugin-management'

export {
  pluginBindingSchema,
  providerBindingSchema,
  type PluginBinding,
  type ProviderBinding,
} from './channel-plugin-bindings'
export type { ChannelPlugin } from './channel-plugin-catalog'
export {
  wireManifest,
  providerManifest,
  type ChannelPluginManifest,
  type ChannelPluginOption,
} from './channel-plugin-manifest'

export const channelPluginQueryKey = ['channel-plugins'] as const

export const channelPluginsApi = {
  ...channelPluginCatalog,
  ...channelPluginManagement,
}
