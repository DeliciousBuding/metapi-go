package ssrf

import (
	"net/netip"
	"testing"
)

func TestPublicAndSiteAddrRejectsCGNATAlibabaMetadataAndNAT64(t *testing.T) {
	t.Parallel()

	unsafePublic := []string{
		"100.64.0.1",
		"100.100.100.200",
		"::ffff:100.64.0.1",
		"::ffff:100.100.100.200",
		"64:ff9b::169.254.169.254",
		"64:ff9b::10.0.0.1",
		"64:ff9b:1::10.0.0.1",
		"fd00:ec2::254",
		"fd20:ce::254",
	}
	for _, raw := range unsafePublic {
		addr := netip.MustParseAddr(raw)
		if !IsUnsafePublicAddr(addr) {
			t.Errorf("IsUnsafePublicAddr(%s) = false, want true", raw)
		}
		if !IsForbiddenPublicHost(raw) {
			t.Errorf("IsForbiddenPublicHost(%s) = false, want true", raw)
		}
	}

	forbiddenSite := []string{
		"100.100.100.200",
		"64:ff9b::169.254.169.254",
		"64:ff9b:1::169.254.169.254",
	}
	for _, raw := range forbiddenSite {
		addr := netip.MustParseAddr(raw)
		if !IsForbiddenSiteAddr(addr) {
			t.Errorf("IsForbiddenSiteAddr(%s) = false, want true", raw)
		}
	}

	if IsUnsafePublicAddr(netip.MustParseAddr("1.1.1.1")) {
		t.Error("IsUnsafePublicAddr(1.1.1.1) = true, want false")
	}
}
