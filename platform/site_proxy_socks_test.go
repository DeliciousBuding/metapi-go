package platform

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// socks5TestServer is a minimal in-process RFC 1928 SOCKS5 server (no auth,
// CONNECT only) used to prove that the account/site proxy path speaks SOCKS5
// through Go's net/http transport natively (bundled SOCKS5 client since
// Go 1.9) — no extra dependency required.
type socks5TestServer struct {
	t        *testing.T
	listener net.Listener
	// resolve maps a requested host to the host that should actually be
	// dialed; nil dials the requested host as-is.
	resolve func(host string) string

	mu      sync.Mutex
	targets []string
}

func startSocks5TestServer(t *testing.T, resolve func(host string) string) *socks5TestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen socks5 test server: %v", err)
	}
	s := &socks5TestServer{t: t, listener: ln, resolve: resolve}
	go s.acceptLoop()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *socks5TestServer) addr() string { return s.listener.Addr().String() }

// connectTargets returns every CONNECT target (as requested by the client)
// observed by the proxy, in order.
func (s *socks5TestServer) connectTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.targets...)
}

func (s *socks5TestServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleConn(conn)
	}
}

func (s *socks5TestServer) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Greeting: VER NMETHODS METHODS... — pick "no auth required" (0x00).
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return
	}
	if greeting[0] != 0x05 {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	noAuth := false
	for _, m := range methods {
		if m == 0x00 {
			noAuth = true
		}
	}
	if !noAuth {
		_, _ = conn.Write([]byte{0x05, 0xFF}) // no acceptable methods
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Request: VER CMD RSV ATYP DST.ADDR DST.PORT — CONNECT only.
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != 0x05 || header[1] != 0x01 {
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // command not supported
		return
	}
	var host string
	switch header[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03: // FQDN — the socks5h proxy-side-resolution path
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return
		}
		domain := make([]byte, int(lenByte[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		host = string(domain)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		_, _ = conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // address type not supported
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return
	}
	port := strconv.Itoa(int(portBytes[0])<<8 | int(portBytes[1]))

	s.mu.Lock()
	s.targets = append(s.targets, net.JoinHostPort(host, port))
	s.mu.Unlock()

	dialHost := host
	if s.resolve != nil {
		dialHost = s.resolve(host)
	}
	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(dialHost, port), 5*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // connection refused
		return
	}
	defer upstream.Close()
	// Success reply with a dummy BND address (CONNECT clients ignore it).
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	_ = conn.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
}

// TestSiteProxy_Socks5AccountProxy_EndToEnd proves that an account-level
// socks5:// proxy (extraConfig.proxyUrl -> ProxyConfig.ProxyURL) works end
// to end through the same dispatch the platform adapters use: Go's net/http
// transport handles socks5:// natively, so no extra dependency is needed.
func TestSiteProxy_Socks5AccountProxy_EndToEnd(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello via socks5"))
	}))
	defer backend.Close()

	socks := startSocks5TestServer(t, nil)
	sp := NewSiteProxy("")

	req, err := http.NewRequest(http.MethodGet, backend.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := sp.Do(context.Background(), req, &ProxyConfig{ProxyURL: "socks5://" + socks.addr()})
	if err != nil {
		t.Fatalf("Do via socks5 proxy: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "hello via socks5" {
		t.Fatalf("response = %d %q, want 200 %q", resp.StatusCode, body, "hello via socks5")
	}

	targets := socks.connectTargets()
	if len(targets) != 1 {
		t.Fatalf("socks5 CONNECT count = %d, want exactly 1 (request must traverse the proxy)", len(targets))
	}
	if !strings.HasPrefix(targets[0], "127.0.0.1:") {
		t.Fatalf("socks5 CONNECT target = %q, want the backend address", targets[0])
	}
}

// TestDoWithProxy_Socks5H_ResolvesAtProxy proves socks5h:// support: the
// request hostname reaches the proxy unresolved (FQDN address type) and the
// proxy performs the resolution — the exact socks5h contract.
func TestDoWithProxy_Socks5H_ResolvesAtProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello via socks5h"))
	}))
	defer backend.Close()
	// backend.URL is http://127.0.0.1:port — keep only the port so the fake
	// hostname can be mapped back to the loopback backend by the proxy.
	backendHostPort := strings.TrimPrefix(backend.URL, "http://")
	_, backendPort, err := net.SplitHostPort(backendHostPort)
	if err != nil {
		t.Fatalf("split backend hostport: %v", err)
	}

	const fakeHost = "socks5h-target.invalid"
	socks := startSocks5TestServer(t, func(host string) string {
		if host == fakeHost {
			return "127.0.0.1"
		}
		return host
	})

	req, err := http.NewRequest(http.MethodGet, "http://"+net.JoinHostPort(fakeHost, backendPort)+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DoWithProxy(context.Background(), req, &ProxyConfig{ProxyURL: "socks5h://" + socks.addr()})
	if err != nil {
		t.Fatalf("DoWithProxy via socks5h proxy: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "hello via socks5h" {
		t.Fatalf("response = %d %q, want 200 %q", resp.StatusCode, body, "hello via socks5h")
	}

	targets := socks.connectTargets()
	if len(targets) != 1 || targets[0] != net.JoinHostPort(fakeHost, backendPort) {
		t.Fatalf("socks5h CONNECT targets = %v, want [%s] (hostname must reach the proxy unresolved)", targets, net.JoinHostPort(fakeHost, backendPort))
	}
}
