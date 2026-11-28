package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	utls "github.com/refraction-networking/utls"
)

// newSiteDialer builds the shared outbound dialer. The connect timeout is
// configurable via PROXY_CONNECT_TIMEOUT_SEC (#1009); the keep-alive period
// stays fixed.
func newSiteDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   effectiveProxyTimeouts().connect,
		KeepAlive: defaultProxyKeepAliveInitial,
	}
}

func newSiteDialContext() ssrf.DialContextFunc {
	return ssrf.NewSiteDialContext(net.DefaultResolver, newSiteDialer().DialContext)
}

// newUTLSTransport builds a pooled *http.Transport whose DialTLSContext
// performs a uTLS handshake with HelloChrome_Auto (the latest Chrome
// ClientHello spec), masking the JA3/JA4 fingerprint that Cloudflare and
// similar WAFs use to block Go's default crypto/tls ClientHello.
//
// On uTLS handshake failure the connection falls back to standard Go TLS
// (tls.Client + HandshakeContext) so a transient uTLS incompatibility never
// breaks the request — the operator can opt in knowing it is best-effort.
//
// Note: DialTLSContext is honored for non-proxied HTTPS connections (per the
// net/http contract). When an HTTP/HTTPS proxy is configured the transport
// still dials through it and uses TLSClientConfig for the post-CONNECT TLS
// handshake; uTLS masking applies to direct connections. SOCKS proxies are
// handled at the dial layer so DialTLSContext covers them as well.
func newUTLSTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) *http.Transport {
	transport := newPooledTransport(proxy, insecureSkipTLS)
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialUTLSContext(ctx, network, addr, insecureSkipTLS)
	}
	return transport
}

// dialUTLSContext performs the uTLS Chrome-ClientHello handshake with a
// standard-TLS fallback. It dials the raw TCP connection, attempts the uTLS
// handshake, and on failure redials and retries with crypto/tls. Both paths
// honor insecureSkipTLS for sites with self-signed or expired certificates.
func dialUTLSContext(ctx context.Context, network, addr string, insecureSkipTLS bool) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	dialContext := newSiteDialContext()

	rawConn, err := dialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("utls dial: %w", err)
	}

	// uTLS handshake with Chrome ClientHello spec.
	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipTLS,
	}, utls.HelloChrome_Auto)
	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		// Fallback to standard Go TLS on uTLS failure — don't let a uTLS
		// incompatibility break the request.
		fallbackConn, dialErr := dialContext(ctx, network, addr)
		if dialErr != nil {
			return nil, fmt.Errorf("utls handshake: %w; fallback dial: %v", err, dialErr)
		}
		stdConn := tls.Client(fallbackConn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecureSkipTLS,
		})
		if handshakeErr := stdConn.HandshakeContext(ctx); handshakeErr != nil {
			fallbackConn.Close()
			return nil, fmt.Errorf("utls handshake: %w; fallback tls handshake: %v", err, handshakeErr)
		}
		return stdConn, nil
	}
	return uConn, nil
}
