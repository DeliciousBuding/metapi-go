package backup

import "github.com/deliciousbuding/metapi-go/routing"

// AxonHub provider types this importer has audited.
//
// The key set is a frozen copy of AxonHub's `channel.Type` enum
// (internal/ent/channel/channel.go) and `defaultEndpointsForChannelType`
// (internal/server/biz/channel_endpoint.go) as of axonhub commit
// e863c6fe1942deddd0f6e471fa003c430e5314f0. A type that appears upstream but
// not here is refused by name, so an AxonHub upgrade cannot silently turn a new
// provider into a mis-mapped Metapi channel.
//
// Protocols is the servable subset of the type's *default* endpoints, used only
// when a channel declares no endpoints of its own. UnservableDefaults records
// that the type's built-in contract also covers formats Metapi's direct relay
// cannot carry (embeddings, images, audio, video); those are reported per
// channel instead of being modeled.
type axonHubProviderType struct {
	Protocols          int
	UnservableDefaults bool
	// Supported is false when the source channel's wire contract cannot be
	// reproduced by a Metapi direct grant: request/response translation,
	// non-static or signed credentials, or a non OpenAI/Anthropic wire format.
	Supported bool
	Reason    string
}

const (
	reasonProviderTranslation = "provider_translation_unsupported"
	reasonProviderCredentials = "provider_credentials_unsupported"
	reasonProviderWire        = "provider_wire_format_unsupported"
)

const (
	protoChat      = routing.UpstreamProtocolChat
	protoResponses = routing.UpstreamProtocolResponses
	protoMessages  = routing.UpstreamProtocolAnthropic
)

var axonHubProviderTypes = map[string]axonHubProviderType{
	// OpenAI-compatible chat / Responses / Anthropic-message channels with a
	// static API key. These are the types a direct grant can actually serve.
	"openai":                {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"openai_responses":      {Protocols: protoResponses, Supported: true},
	"atlascloud":            {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"qiniu":                 {Protocols: protoChat, Supported: true},
	"qiniu_anthropic":       {Protocols: protoMessages, Supported: true},
	"cline":                 {Protocols: protoChat, Supported: true},
	"fenno":                 {Protocols: protoResponses, Supported: true},
	"vercel":                {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"anthropic":             {Protocols: protoMessages, Supported: true},
	"gemini_openai":         {Protocols: protoChat, Supported: true},
	"deepseek":              {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"deepseek_anthropic":    {Protocols: protoMessages, Supported: true},
	"deepinfra":             {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"fireworks":             {Protocols: protoChat, Supported: true},
	"doubao":                {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"doubao_anthropic":      {Protocols: protoMessages, Supported: true},
	"moonshot":              {Protocols: protoChat, Supported: true},
	"moonshot_anthropic":    {Protocols: protoMessages, Supported: true},
	"moonshot_coding":       {Protocols: protoMessages, Supported: true},
	"zhipu":                 {Protocols: protoChat, Supported: true},
	"zai":                   {Protocols: protoChat, Supported: true},
	"zhipu_anthropic":       {Protocols: protoMessages, Supported: true},
	"zai_anthropic":         {Protocols: protoMessages, Supported: true},
	"openrouter":            {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"xiaomi":                {Protocols: protoChat, Supported: true},
	"xiaomi_anthropic":      {Protocols: protoMessages, Supported: true},
	"xai":                   {Protocols: protoChat, Supported: true},
	"ppio":                  {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"siliconflow":           {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"volcengine":            {Protocols: protoChat, Supported: true},
	"volcengine_anthropic":  {Protocols: protoMessages, Supported: true},
	"longcat":               {Protocols: protoChat, Supported: true},
	"longcat_anthropic":     {Protocols: protoMessages, Supported: true},
	"minimax":               {Protocols: protoChat, Supported: true},
	"minimax_anthropic":     {Protocols: protoMessages, Supported: true},
	"aihubmix":              {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"aihubmix_anthropic":    {Protocols: protoMessages, Supported: true},
	"burncloud":             {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"modelscope":            {Protocols: protoChat, Supported: true},
	"bailian":               {Protocols: protoChat, Supported: true},
	"bailian_anthropic":     {Protocols: protoMessages, Supported: true},
	"github":                {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"cerebras":              {Protocols: protoChat, Supported: true},
	"nanogpt":               {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"nanogpt_responses":     {Protocols: protoResponses, Supported: true},
	"ollama_anthropic":      {Protocols: protoMessages, Supported: true},
	"evolink":               {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"evolink_anthropic":     {Protocols: protoMessages, Supported: true},
	"groq":                  {Protocols: protoChat, UnservableDefaults: true, Supported: true},
	"opencode_go":           {Protocols: protoChat, Supported: true},
	"opencode_go_anthropic": {Protocols: protoMessages, Supported: true},

	// Wire formats Metapi's direct relay has no transformer for.
	"gemini":         {Reason: reasonProviderTranslation},
	"gemini_vertex":  {Reason: reasonProviderTranslation},
	"antigravity":    {Reason: reasonProviderTranslation},
	"jina":           {Reason: reasonProviderWire},
	"ollama":         {Reason: reasonProviderWire},
	"openai_fake":    {Reason: reasonProviderWire},
	"anthropic_fake": {Reason: reasonProviderWire},

	// Channels whose authentication cannot be a static API key in a Metapi
	// direct grant, or whose request/response shape is provider-specific.
	"anthropic_aws":  {Protocols: protoMessages, Reason: reasonProviderCredentials},
	"anthropic_gcp":  {Protocols: protoMessages, Reason: reasonProviderCredentials},
	"codex":          {Protocols: protoResponses, Reason: reasonProviderCredentials},
	"claudecode":     {Protocols: protoMessages, Reason: reasonProviderCredentials},
	"github_copilot": {Protocols: protoChat, Reason: reasonProviderCredentials},
}

// axonHubServableFormats maps source api_format values onto Metapi's direct
// relay protocols. Anything outside this table cannot be reached through a
// direct grant, whatever the channel type claims.
var axonHubServableFormats = map[string]int{
	"openai/chat_completions": protoChat,
	"openai/responses":        protoResponses,
	"anthropic/messages":      protoMessages,
}

// axonHubDefaultPaths mirrors the direct-relay defaults used by the Octopus
// importer; a source endpoint path always overrides them.
var axonHubDefaultPaths = map[int]string{
	protoChat:      "/v1/chat/completions",
	protoResponses: "/v1/responses",
	protoMessages:  "/v1/messages",
}

// axonHubResidualProtocolFormats names api_format values that AxonHub may grant
// but a Metapi direct grant cannot carry. Kept as an explicit list so a preview
// can explain *why* a protocol was left behind instead of just counting it.
var axonHubResidualProtocolFormats = map[string]string{
	"openai/completions":          "legacy completions endpoint is not routed by direct grants",
	"openai/responses_compact":    "compact Responses variant is not routed by direct grants",
	"openai/embeddings":           "embeddings are not routed by direct grants",
	"openai/image_generation":     "image generation is not routed by direct grants",
	"openai/image_edit":           "image editing is not routed by direct grants",
	"openai/image_variation":      "image variations are not routed by direct grants",
	"openai/video":                "video generation is not routed by direct grants",
	"openai/audio_speech":         "text-to-speech is not routed by direct grants",
	"openai/audio_transcriptions": "transcription is not routed by direct grants",
	"openai/audio_translations":   "translation is not routed by direct grants",
	"openai/moderations":          "moderations are not routed by direct grants",
	"gemini/contents":             "Gemini native protocol is not routed by direct grants",
	"gemini/embeddings":           "Gemini embeddings are not routed by direct grants",
	"jina/rerank":                 "Jina rerank is not routed by direct grants",
	"jina/embeddings":             "Jina embeddings are not routed by direct grants",
	"ollama/chat":                 "Ollama native protocol is not routed by direct grants",
	"seedance/video":              "Seedance video is not routed by direct grants",
	"aisdk/text":                  "AI SDK text protocol is not routed by direct grants",
	"aisdk/datastream":            "AI SDK data stream protocol is not routed by direct grants",
}
