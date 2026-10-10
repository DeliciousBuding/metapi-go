package backup

import "github.com/deliciousbuding/metapi-go/store"

// AxonHub provider types this importer has audited.
//
// The key set is a frozen copy of AxonHub's `channel.Type` enum
// (internal/ent/channel/channel.go) and `defaultEndpointsForChannelType`
// (internal/server/biz/channel_endpoint.go) as of axonhub commit
// e863c6fe1942deddd0f6e471fa003c430e5314f0. A type that appears upstream but
// not here is refused by name, so an AxonHub upgrade cannot silently turn a new
// provider into a mis-mapped Metapi channel.
//
// DefaultFormats retains the source endpoint order, including formats that
// remain unsupported. Masks, member order and residuals are derived from this
// one list; different source formats never acquire grants by sharing a bit.
type axonHubProviderType struct {
	DefaultFormats []string
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
	protoChat               = store.DirectProtocolChat
	protoResponses          = store.DirectProtocolResponses
	protoMessages           = store.DirectProtocolMessages
	protoGemini             = store.DirectProtocolGemini
	protoCompletions        = store.DirectProtocolCompletions
	protoEmbeddings         = store.DirectProtocolEmbeddings
	protoRerank             = store.DirectProtocolRerank
	protoImageGeneration    = store.DirectProtocolImageGeneration
	protoImageEdit          = store.DirectProtocolImageEdit
	protoImageVariation     = store.DirectProtocolImageVariation
	protoAudioSpeech        = store.DirectProtocolAudioSpeech
	protoAudioTranscription = store.DirectProtocolAudioTranscription
	protoAudioTranslation   = store.DirectProtocolAudioTranslation
	protoModerations        = store.DirectProtocolModerations
	protoVideo              = store.DirectProtocolVideo
	protoGeminiEmbeddings   = store.DirectProtocolGeminiEmbeddings
	protoJinaEmbeddings     = store.DirectProtocolJinaEmbeddings
	protoModelScopeImage    = store.DirectProtocolModelScopeImageGeneration
	protoSeedanceVideo      = store.DirectProtocolSeedanceVideo
	protoZenmuxVideo        = store.DirectProtocolZenmuxVideo
	protoOllama             = store.DirectProtocolOllama
	protoSystemOne          = store.DirectProtocolSystemOne
	protoAlphaSearch        = store.DirectProtocolAlphaSearch
)

var axonHubProviderTypes = map[string]axonHubProviderType{
	// OpenAI-compatible chat / Responses / Anthropic-message channels with a
	// static API key. These are the types a direct grant can actually serve.
	"openai":                {DefaultFormats: axonHubFullFormats, Supported: true},
	"openai_responses":      {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"atlascloud":            {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"qiniu":                 {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"qiniu_anthropic":       {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"cline":                 {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"fenno":                 {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"vercel":                {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"anthropic":             {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"gemini_openai":         {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"deepseek":              {DefaultFormats: []string{"openai/chat_completions", "openai/completions"}, Supported: true},
	"deepseek_anthropic":    {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"deepinfra":             {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"fireworks":             {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"doubao":                {DefaultFormats: []string{"openai/chat_completions", "seedance/video"}, Supported: true},
	"doubao_anthropic":      {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"moonshot":              {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"moonshot_anthropic":    {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"moonshot_coding":       {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"zhipu":                 {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"zai":                   {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"zhipu_anthropic":       {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"zai_anthropic":         {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"openrouter":            {DefaultFormats: axonHubChatAudioFormats, Supported: true},
	"xiaomi":                {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"xiaomi_anthropic":      {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"xai":                   {DefaultFormats: []string{"openai/chat_completions", "openai/responses"}, Supported: true},
	"ppio":                  {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"siliconflow":           {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"volcengine":            {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"volcengine_anthropic":  {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"longcat":               {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"longcat_anthropic":     {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"minimax":               {DefaultFormats: []string{"openai/chat_completions", "openai/image_generation"}, Supported: true},
	"minimax_anthropic":     {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"aihubmix":              {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"aihubmix_anthropic":    {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"burncloud":             {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"modelscope":            {DefaultFormats: []string{"openai/chat_completions", "modelscope/image_generation"}, Supported: true},
	"bailian":               {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"bailian_anthropic":     {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"github":                {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"cerebras":              {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"nanogpt":               {DefaultFormats: axonHubFullFormats, Supported: true},
	"nanogpt_responses":     {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"ollama_anthropic":      {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"evolink":               {DefaultFormats: axonHubCompatibleFormats, Supported: true},
	"evolink_anthropic":     {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"groq":                  {DefaultFormats: axonHubChatAudioFormats, Supported: true},
	"opencode_go":           {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"opencode_go_anthropic": {DefaultFormats: []string{}, Supported: true},

	"xai_responses":         {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"bailian_responses":     {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"zenmux":                {DefaultFormats: axonHubFullFormats, Supported: true},
	"zenmux_responses":      {DefaultFormats: []string{"openai/responses"}, Supported: true},
	"zenmux_anthropic":      {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"commandcode":           {DefaultFormats: []string{"openai/chat_completions"}, Supported: true},
	"commandcode_anthropic": {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"xai_subscription":      {DefaultFormats: []string{"openai/responses"}, Reason: reasonProviderCredentials},
	"zenmux_gemini":         {DefaultFormats: []string{"gemini/contents", "gemini/embeddings"}, Supported: true},
	"zenmux_video":          {DefaultFormats: []string{"zenmux/video"}, Supported: true},
	"typesafe":              {DefaultFormats: []string{"typesafe/systemone"}, Supported: true},

	// Native provider families retain their own wire and credential contracts.
	"gemini":         {DefaultFormats: []string{"gemini/contents", "gemini/embeddings"}, Supported: true},
	"gemini_vertex":  {DefaultFormats: []string{"gemini/contents", "gemini/embeddings"}, Reason: reasonProviderTranslation},
	"antigravity":    {DefaultFormats: []string{"gemini/contents"}, Reason: reasonProviderTranslation},
	"jina":           {DefaultFormats: []string{"jina/rerank", "jina/embeddings"}, Supported: true},
	"ollama":         {DefaultFormats: []string{"ollama/chat"}, Supported: true},
	"openai_fake":    {DefaultFormats: []string{"openai/chat_completions"}, Reason: reasonProviderWire},
	"anthropic_fake": {DefaultFormats: []string{"anthropic/messages"}, Reason: reasonProviderWire},

	// Channels whose authentication cannot be a static API key in a Metapi
	// direct grant, or whose request/response shape is provider-specific.
	"anthropic_aws":  {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"anthropic_gcp":  {DefaultFormats: []string{"anthropic/messages"}, Reason: reasonProviderCredentials},
	"codex":          {DefaultFormats: []string{"openai/responses", "openai/alpha_search", "openai/image_generation", "openai/image_edit"}, Supported: true},
	"claudecode":     {DefaultFormats: []string{"anthropic/messages"}, Supported: true},
	"github_copilot": {DefaultFormats: []string{"openai/chat_completions"}, Reason: reasonProviderCredentials},
}

// axonHubServableFormats maps source api_format values onto Metapi's direct
// relay protocols. Anything outside this table cannot be reached through a
// direct grant, whatever the channel type claims.
var axonHubServableFormats = map[string]int{
	"openai/chat_completions":     protoChat,
	"openai/responses":            protoResponses,
	"anthropic/messages":          protoMessages,
	"gemini/contents":             protoGemini,
	"openai/completions":          protoCompletions,
	"openai/embeddings":           protoEmbeddings,
	"openai/image_generation":     protoImageGeneration,
	"openai/image_edit":           protoImageEdit,
	"openai/image_variation":      protoImageVariation,
	"openai/audio_speech":         protoAudioSpeech,
	"openai/audio_transcriptions": protoAudioTranscription,
	"openai/audio_translations":   protoAudioTranslation,
	"openai/moderations":          protoModerations,
	"openai/video":                protoVideo,
	"gemini/embeddings":           protoGeminiEmbeddings,
	"jina/rerank":                 protoRerank,
	"jina/embeddings":             protoJinaEmbeddings,
	"modelscope/image_generation": protoModelScopeImage,
	"seedance/video":              protoSeedanceVideo,
	"zenmux/video":                protoZenmuxVideo,
	"ollama/chat":                 protoOllama,
	"typesafe/systemone":          protoSystemOne,
	"openai/alpha_search":         protoAlphaSearch,
}

var axonHubCompatibleFormats = []string{
	"openai/chat_completions", "openai/embeddings", "openai/image_generation", "openai/image_edit",
	"openai/image_variation", "openai/video", "openai/moderations",
}
var axonHubAudioFormats = []string{
	"openai/audio_speech", "openai/audio_transcriptions", "openai/audio_translations",
}
var axonHubFullFormats = append(append([]string{}, axonHubCompatibleFormats...), axonHubAudioFormats...)
var axonHubChatAudioFormats = append([]string{"openai/chat_completions"}, axonHubAudioFormats...)

// Source formats without a corresponding executor stay explicit in previews.
var axonHubResidualProtocolFormats = map[string]string{
	"openai/responses_compact": "compact Responses variant is not routed by direct grants",
	"aisdk/text":               "AI SDK text protocol is not routed by direct grants",
	"openai/decisions":         "decisions are not routed by direct grants",
	"aisdk/datastream":         "AI SDK data stream protocol is not routed by direct grants",
}
