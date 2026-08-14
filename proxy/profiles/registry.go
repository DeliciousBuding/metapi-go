package profiles

import "github.com/deliciousbuding/metapi-go/proxy/types"

// All returns all CLI profile definitions for registration.
// Callers must register them with proxy.RegisterDetectionProfile / RegisterFallbackProfile.
func All() []*types.CliProfileDefinition {
	return []*types.CliProfileDefinition{
		claudeCodeProfile(),
		codexProfile(),
		geminiCliProfile(),
	}
}

// Generic returns the generic fallback profile.
func Generic() *types.CliProfileDefinition {
	return &types.CliProfileDefinition{
		ID: types.ProfileGeneric,
		Detect: func(input types.DetectInput) *types.DetectedProfile {
			return &types.DetectedProfile{
				ID: types.ProfileGeneric,
			}
		},
	}
}
