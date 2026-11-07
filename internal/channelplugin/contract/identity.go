// Package contract contains plugin configuration identity rules without
// depending on the executable JS runtime. Storage validates data, not code.
package contract

import "regexp"

const MaxProtocols = 6

const MaxProviderProtocols = 26

// Drivers address the gateway's typed endpoint slots. This separate predicate
// does not broaden the existing wire-hook binding or transport permissions.
func ValidProviderProtocol(protocol string) bool {
	switch protocol {
	case "chat", "responses", "messages", "gemini", "completions", "embeddings", "rerank",
		"imageGeneration", "imageEdit", "imageVariation", "audioSpeech", "audioTranscription",
		"audioTranslation", "moderations", "video", "geminiEmbeddings", "jinaEmbeddings",
		"modelscopeImageGeneration", "seedanceVideo", "zenmuxVideo", "ollama", "systemOne",
		"alphaSearch", "responsesCompact", "responsesWebsocket", "decisions":
		return true
	default:
		return false
	}
}

// Protocols name endpoint slots, not interchangeable wire formats. In
// particular an HTTP Responses binding grants neither compact nor WS hooks.
func ValidProtocol(protocol string) bool {
	switch protocol {
	case "chat", "responses", "messages", "gemini", "responsesCompact", "responsesWebsocket":
		return true
	default:
		return false
	}
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidFingerprint(value string) bool { return fingerprintPattern.MatchString(value) }

var cursorPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func ValidCursorParameter(value string) bool { return value == "" || cursorPattern.MatchString(value) }

func ValidKey(key string) bool { return keyPattern.MatchString(key) }
