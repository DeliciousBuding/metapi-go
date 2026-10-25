package proxyhandler

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
)

// downstreamSessionNamespace derives a stable, non-reversible namespace from
// the authenticated downstream key. The raw key/id never appears in the value;
// different keys never share a namespace, so identical client session ids map
// to different upstream session ids (audit L28). Empty when no auth context is
// available (unit callers that exercise wire construction directly).
func downstreamSessionNamespace(pac *auth.ProxyAuthContext) string {
	if pac == nil {
		return ""
	}
	identity := ""
	if pac.KeyID != nil {
		identity = "key:" + strconv.FormatInt(*pac.KeyID, 10)
	} else if token := strings.TrimSpace(pac.Token); token != "" {
		identity = "token:" + token
	}
	if identity == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("metapi-downstream-namespace\x00" + identity))
	return hex.EncodeToString(digest[:16])
}
