package notify

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestSMTPPortAllowed(t *testing.T) {
	for _, port := range []int{25, 465, 587} {
		if !smtpPortAllowed(port, false) {
			t.Errorf("smtpPortAllowed(%d, false) = false, want true", port)
		}
	}
	for _, port := range []int{0, -1, 22, 2525, 10025, 70000} {
		if smtpPortAllowed(port, false) {
			t.Errorf("smtpPortAllowed(%d, false) = true, want false", port)
		}
	}
	// The explicit operator escape hatch accepts any valid TCP port.
	for _, port := range []int{22, 2525, 65535} {
		if !smtpPortAllowed(port, true) {
			t.Errorf("smtpPortAllowed(%d, true) = false, want true", port)
		}
	}
	if smtpPortAllowed(0, true) || smtpPortAllowed(65536, true) {
		t.Error("custom-port escape hatch must still require a valid TCP port")
	}
}

// TestGuardSMTPTargetRejectsPrivateAndMetadata is the M12 core regression:
// the SMTP target is session-writable, so private, metadata, ULA and
// 0.0.0.0/8 addresses must be refused before any connection.
func TestGuardSMTPTargetRejectsPrivateAndMetadata(t *testing.T) {
	for _, host := range []string{
		"127.0.0.1",
		"localhost",
		"10.0.0.5",
		"192.168.1.5",
		"169.254.169.254",
		"[fd20:ce::254]",
		"0.1.2.3",
	} {
		if err := guardSMTPTarget(host, 587, false); err == nil {
			t.Errorf("guardSMTPTarget(%q, 587, false) = nil, want rejection", host)
		}
	}
	if err := guardSMTPTarget("smtp.example.com", 587, false); err != nil {
		t.Fatalf("public SMTP target rejected: %v", err)
	}
}

// TestSMTPChannelRejectsNonStandardPort ensures a session cannot repoint the
// port to probe arbitrary internal services (audit M12: the audit's probe
// used port 18100).
func TestSMTPChannelRejectsNonStandardPort(t *testing.T) {
	err := (&SMTPChannel{}).Send(&config.RuntimeSettings{
		SmtpEnabled: true,
		SmtpHost:    "smtp.example.com",
		SmtpPort:    18100,
		SmtpFrom:    "from@example.com",
		SmtpTo:      "to@example.com",
	}, "title", "message", "info", "time")
	if err == nil {
		t.Fatal("SMTP accepted a non-standard port without SMTP_ALLOW_CUSTOM_PORT")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %q, want the port whitelist rejection", err)
	}
}

// TestSMTPChannelDoesNotDialPrivateHost asserts the host guard fires before
// the dialer: a loopback listener must never accept a connection even when
// the operator has allowed a custom port.
func TestSMTPChannelDoesNotDialPrivateHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	sendErr := (&SMTPChannel{}).Send(&config.RuntimeSettings{
		SmtpEnabled:         true,
		SmtpHost:            "127.0.0.1",
		SmtpPort:            port,
		SmtpAllowCustomPort: true,
		SmtpFrom:            "from@example.com",
		SmtpTo:              "to@example.com",
	}, "title", "message", "info", "time")
	if sendErr == nil {
		t.Fatal("SMTP send to 127.0.0.1 succeeded, want rejection")
	}
	if !strings.Contains(sendErr.Error(), "not a public address") {
		t.Fatalf("error = %q, want the public-address rejection", sendErr)
	}

	if tcpListener, ok := listener.(*net.TCPListener); ok {
		_ = tcpListener.SetDeadline(time.Now().Add(200 * time.Millisecond))
	}
	conn, acceptErr := listener.Accept()
	if acceptErr == nil {
		_ = conn.Close()
		t.Fatal("loopback listener accepted a connection from the SMTP channel")
	}
}
