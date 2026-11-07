package platform

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
)

// The caller has validated the public IP and preserves the original hostname
// in Host/SNI. Proxies receive that pinned IP too, not another DNS lookup.
// Clone a bounded existing transport rather than cache attacker-chosen SNI
// names forever. Response close releases this download's idle connections.
func DoPinnedDownloadStreamBudget(ctx context.Context, req *http.Request, cfg *ProxyConfig, serverName string, budgetMs int64) (*http.Response, error) {
	if req.URL == nil || req.URL.User != nil || req.URL.Fragment != "" || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.Method != http.MethodGet || strings.ContainsAny(serverName, "\r\n\x00") {
		return nil, fmt.Errorf("invalid pinned download")
	}
	if _, err := netip.ParseAddr(req.URL.Hostname()); err != nil {
		return nil, fmt.Errorf("download target must be a pinned IP")
	}
	var proxy func(*http.Request) (*url.URL, error)
	if cfg != nil && cfg.ProxyURL != "" {
		target, err := url.Parse(cfg.ProxyURL)
		if err != nil || !supportedProxySchemes[strings.ToLower(target.Scheme)] {
			return nil, fmt.Errorf("invalid download proxy")
		}
		proxy = http.ProxyURL(target)
	}
	transport := getCachedTransportForMode(proxy, false, false, true).Clone()
	if cfg != nil && cfg.ProxyURL != "" {
		target, _ := url.Parse(cfg.ProxyURL)
		if target.Scheme == "http" || target.Scheme == "https" {
			// Absolute-form HTTP proxying rewrites Host from the target URL.
			// Tunnel even plain HTTP so IP pinning and virtual Host both hold.
			transport.Proxy = nil
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialPinnedProxyTunnel(ctx, target, address)
			}
		}
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.ServerName = serverName
	transport.TLSClientConfig.InsecureSkipVerify = false
	transport.DialTLSContext = nil
	request := req.Clone(ctx)
	request.Body = nil
	request.GetBody = nil
	request.ContentLength = 0
	request.Header = make(http.Header)
	for _, key := range []string{"Range", "If-Range", "Accept"} {
		if value := req.Header.Get(key); value != "" {
			request.Header.Set(key, value)
		}
	}
	client := newStreamProxyClient(transport)
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.User != nil || next.URL.Fragment != "" || next.URL.Scheme != request.URL.Scheme {
			return fmt.Errorf("invalid download redirect")
		}
		// A CDN can emit an absolute URL for its original hostname. Map that
		// exact logical origin back to the already-validated IP, never resolve
		// it again. Other hosts/ports remain subject to the existing guard.
		if request.Host != "" && strings.EqualFold(next.URL.Host, request.Host) {
			next.URL.Host = request.URL.Host
		}
		if err := RejectCrossOriginRedirect(next, via); err != nil {
			return err
		}
		next.Host = request.Host
		return nil
	}
	response, err := httpclient.DoWithHeaderBudget(request, budgetMs, client.Do)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	response.Body = &pinnedDownloadBody{ReadCloser: response.Body, transport: transport}
	return response, nil
}

type pinnedDownloadBody struct {
	io.ReadCloser
	transport *http.Transport
}

func (b *pinnedDownloadBody) Close() error {
	err := b.ReadCloser.Close()
	b.transport.CloseIdleConnections()
	return err
}

func dialPinnedProxyTunnel(ctx context.Context, proxy *url.URL, address string) (result net.Conn, err error) {
	port := proxy.Port()
	if port == "" {
		port = "80"
		if proxy.Scheme == "https" {
			port = "443"
		}
	}
	conn, err := newSiteDialContext()(ctx, "tcp", net.JoinHostPort(proxy.Hostname(), port))
	if err != nil {
		return nil, err
	}
	baseConn := conn
	stop := context.AfterFunc(ctx, func() { _ = baseConn.Close() })
	defer func() {
		stop()
		if err != nil {
			_ = conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if proxy.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: proxy.Hostname()})
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if proxy.User != nil {
		password, _ := proxy.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password)))
	}
	if err = request.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("download proxy refused tunnel: %d", response.StatusCode)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return &pinnedProxyConn{Conn: conn, reader: reader}, nil
}

type pinnedProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *pinnedProxyConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
