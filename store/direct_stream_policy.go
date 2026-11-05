package store

const (
	DirectStreamUnlimited = "unlimited"
	DirectStreamRequire   = "require"
	DirectStreamForbid    = "forbid"
)

func ValidDirectStreamPolicy(policy string) bool {
	switch policy {
	case "", DirectStreamUnlimited, DirectStreamRequire, DirectStreamForbid:
		return true
	}
	return false
}

func NormalizeDirectStreamPolicy(policy string) string {
	if policy == "" {
		return DirectStreamUnlimited
	}
	return policy
}
