package service

// NativeChatRequestProfile resolves the thinking contract of a recognized
// provider URL. Arbitrary compatible hosts do not inherit a brand's profile.
func NativeChatRequestProfile(rawURL, platform string) string {
	preset := DetectSiteInitializationPreset(rawURL, platform)
	if preset == nil {
		return ""
	}
	switch preset.ID {
	case "deepseek-openai":
		return "deepseek"
	case "zhipu-openai", "zhipu-coding-plan-openai", "zai-openai", "zai-coding-plan-openai", "xiaomi-openai":
		return "zai"
	default:
		return ""
	}
}
