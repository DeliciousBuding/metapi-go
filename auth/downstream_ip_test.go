package auth

import (
	"strings"
	"testing"
)

func ptrString(s string) *string { return &s }

func TestSplitIPList(t *testing.T) {
	cases := []struct {
		name string
		raw  *string
		want []string
	}{
		{"nil", nil, nil},
		{"empty", ptrString(""), nil},
		{"whitespace only", ptrString("   \n  "), nil},
		{"newline separated", ptrString("1.2.3.4\n10.0.0.0/8\n 5.6.7.8 "), []string{"1.2.3.4", "10.0.0.0/8", "5.6.7.8"}},
		{"comma separated", ptrString("1.2.3.4,10.0.0.0/8,5.6.7.8"), []string{"1.2.3.4", "10.0.0.0/8", "5.6.7.8"}},
		{"mixed crlf+comma", ptrString("1.2.3.4\r\n10.0.0.0/8, 5.6.7.8"), []string{"1.2.3.4", "10.0.0.0/8", "5.6.7.8"}},
		{"trailing comma", ptrString("1.2.3.4,\n"), []string{"1.2.3.4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitIPList(c.raw)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("idx %d: got %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestCheckDownstreamKeyIP(t *testing.T) {
	// nil key → unrestricted
	if ok, _ := CheckDownstreamKeyIP(nil, "1.2.3.4"); !ok {
		t.Fatal("nil key should allow")
	}
	// empty clientIP → unrestricted (caller responsibility)
	key := &managedKeyView{}
	if ok, _ := CheckDownstreamKeyIP(key, ""); !ok {
		t.Fatal("empty clientIP should allow")
	}
	// both lists empty → allow
	if ok, _ := CheckDownstreamKeyIP(key, "1.2.3.4"); !ok {
		t.Fatal("empty lists should allow")
	}

	// allowlist set, IP matches → allow
	keyAllow := &managedKeyView{IPAllowlist: ptrString("1.2.3.4\n10.0.0.0/8")}
	if ok, r := CheckDownstreamKeyIP(keyAllow, "10.0.0.5"); !ok {
		t.Fatalf("CIDR allow match should allow, got reason %q", r)
	}
	// allowlist set, IP not matching → deny (ip_not_allowed)
	if ok, r := CheckDownstreamKeyIP(keyAllow, "8.8.8.8"); ok || r != "ip_not_allowed" {
		t.Fatalf("non-matching allowlist should deny ip_not_allowed, got ok=%v r=%q", ok, r)
	}
	// IPv4-mapped IPv6 normalization
	if ok, r := CheckDownstreamKeyIP(keyAllow, "::ffff:1.2.3.4"); !ok {
		t.Fatalf("IPv4-mapped IPv6 should normalize+allow, got reason %q", r)
	}

	// blocklist wins over allowlist match
	keyBoth := &managedKeyView{IPAllowlist: ptrString("0.0.0.0/0"), IPBlocklist: ptrString("1.2.3.4")}
	if ok, r := CheckDownstreamKeyIP(keyBoth, "1.2.3.4"); ok || r != "ip_blocked" {
		t.Fatalf("blocklist should win with ip_blocked, got ok=%v r=%q", ok, r)
	}
	// 1.2.3.5 is in 0.0.0.0/0 allowlist and not blocklisted → allow
	if ok, r := CheckDownstreamKeyIP(keyBoth, "1.2.3.5"); !ok {
		t.Fatalf("1.2.3.5 should be allowed (in 0.0.0.0/0 allowlist, not blocklisted), got reason %q", r)
	}

	// invalid allowlist entries silently skipped, valid one still enforces
	keyBad := &managedKeyView{IPAllowlist: ptrString("not-an-ip\n1.2.3.4")}
	if ok, _ := CheckDownstreamKeyIP(keyBad, "1.2.3.4"); !ok {
		t.Fatal("valid entry should allow despite invalid sibling")
	}
	if ok, r := CheckDownstreamKeyIP(keyBad, "5.5.5.5"); ok || r != "ip_not_allowed" {
		t.Fatalf("non-listed IP should deny, got ok=%v r=%q", ok, r)
	}
}

func TestCheckDownstreamKeyIPv6Loopback(t *testing.T) {
	key := &managedKeyView{IPAllowlist: ptrString("127.0.0.1")}
	// ::1 normalizes to 127.0.0.1
	if ok, r := CheckDownstreamKeyIP(key, "::1"); !ok {
		t.Fatalf("::1 should normalize to 127.0.0.1 and allow, got %q", r)
	}
	if ok, r := CheckDownstreamKeyIP(key, "192.168.1.1"); ok || r != "ip_not_allowed" {
		t.Fatalf("192.168.1.1 not in allowlist should deny, got ok=%v r=%q", ok, r)
	}
}

func TestSplitIPListDoesNotMutateInput(t *testing.T) {
	raw := "1.2.3.4\n10.0.0.0/8"
	p := ptrString(raw)
	_ = splitIPList(p)
	if *p != raw {
		t.Fatalf("splitIPList mutated input: %q", *p)
	}
	if strings.Contains(raw, "\r") {
		t.Fatal("test setup: raw should not contain CR")
	}
}
