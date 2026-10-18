package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// smtpDialContext is the guarded dialer for SMTP conversations. SMTP is an
// admin/session-configurable outbound target (smtpHost/smtpPort), so it uses
// the same public-only policy as the HTTP notification channels: loopback,
// RFC1918/ULA, link-local/metadata and 0.0.0.0/8 are refused, and the
// validated DNS answer is pinned for the dial (no TOCTOU window).
var smtpDialContext = ssrf.NewPublicOnlyDialContext(nil, (&net.Dialer{Timeout: 30 * time.Second}).DialContext)

// smtpAllowedPorts are the standard submission ports. A non-standard port is
// allowed only when the operator explicitly sets SMTP_ALLOW_CUSTOM_PORT=true;
// otherwise a session-writable smtpPort turns the notify test into an
// arbitrary-port probe of internal hosts (audit M12).
var smtpAllowedPorts = map[int]struct{}{25: {}, 465: {}, 587: {}}

func smtpPortAllowed(port int, allowCustom bool) bool {
	if allowCustom {
		return port > 0 && port <= 65535
	}
	_, ok := smtpAllowedPorts[port]
	return ok
}

// guardSMTPTarget applies the port whitelist and the public-only host policy
// before any connection is attempted.
func guardSMTPTarget(host string, port int, allowCustom bool) error {
	if !smtpPortAllowed(port, allowCustom) {
		return fmt.Errorf("smtp port %d is not allowed (allowed: 25, 465, 587); set SMTP_ALLOW_CUSTOM_PORT=true to override", port)
	}
	if ssrf.IsForbiddenPublicHost(host) {
		return fmt.Errorf("smtp host %q is not a public address", host)
	}
	return nil
}

// SMTPChannel sends notifications via SMTP email.
type SMTPChannel struct {
	mu                sync.Mutex
	cachedFingerprint string
	cachedClient      *cachedSMTPClient
}

type cachedSMTPClient struct {
	addr   string
	auth   smtp.Auth
	from   string
	useTLS bool
}

func (c *SMTPChannel) Name() string { return "smtp" }

func (c *SMTPChannel) Send(cfg *config.RuntimeSettings, title, message, level, timeFootnote string) error {
	if !cfg.SmtpEnabled || cfg.SmtpHost == "" || cfg.SmtpPort <= 0 || cfg.SmtpFrom == "" || cfg.SmtpTo == "" {
		return fmt.Errorf("smtp not configured")
	}
	if err := guardSMTPTarget(cfg.SmtpHost, cfg.SmtpPort, cfg.SmtpAllowCustomPort); err != nil {
		return err
	}

	fingerprint := getSmtpFingerprint(cfg)

	subject := fmt.Sprintf("[metapi][%s] %s", strings.ToUpper(level), title)
	body := fmt.Sprintf("%s\r\n\r\nLevel: %s\r\n%s", message, level, timeFootnote)
	emailBody := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		cfg.SmtpFrom, cfg.SmtpTo, subject, body)

	addr := fmt.Sprintf("%s:%d", cfg.SmtpHost, cfg.SmtpPort)

	c.mu.Lock()
	// Check if config changed and cached client is invalid
	if c.cachedFingerprint != fingerprint {
		c.cachedFingerprint = fingerprint
		c.cachedClient = nil
	}
	cachedClient := c.cachedClient
	c.mu.Unlock()

	var auth smtp.Auth
	if cachedClient != nil && cachedClient.addr == addr && cachedClient.useTLS == cfg.SmtpSecure {
		auth = cachedClient.auth
	} else if cfg.SmtpUser != "" {
		auth = smtp.PlainAuth("", cfg.SmtpUser, cfg.SmtpPass, cfg.SmtpHost)
	}

	if err := sendSMTP(addr, cfg.SmtpHost, cfg.SmtpSecure, auth, cfg.SmtpFrom, cfg.SmtpTo, emailBody); err != nil {
		return err
	}

	// Cache the client config for reuse
	c.mu.Lock()
	c.cachedClient = &cachedSMTPClient{
		addr:   addr,
		auth:   auth,
		from:   cfg.SmtpFrom,
		useTLS: cfg.SmtpSecure,
	}
	c.mu.Unlock()

	return nil
}

// sendSMTP runs one SMTP conversation over the guarded dialer. It mirrors the
// semantics of smtp.SendMail (implicit TLS when configured, opportunistic
// STARTTLS otherwise, auth only over TLS) but dials through smtpDialContext so
// the target cannot reach private addresses and a DNS answer cannot change
// between validation and dial.
func sendSMTP(addr, host string, secure bool, auth smtp.Auth, from, to, emailBody string) error {
	conn, err := smtpDialContext(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial failed: %w", err)
	}
	defer conn.Close()
	// Bound the whole conversation: net/smtp has no per-command deadline.
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	tlsActive := secure
	if secure {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("smtp tls handshake failed: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp new client failed: %w", err)
	}
	defer client.Close()

	if !tlsActive {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("smtp starttls failed: %w", err)
			}
			tlsActive = true
		}
	}

	// PlainAuth refuses to send credentials over an unencrypted connection;
	// skip auth rather than downgrade, matching smtp.SendMail.
	if auth != nil && tlsActive {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail failed: %w", err)
	}
	for _, recipient := range strings.Split(to, ",") {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp rcpt failed: %w", err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err := wc.Write([]byte(emailBody)); err != nil {
		wc.Close()
		return fmt.Errorf("smtp write failed: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close failed: %w", err)
	}
	_ = client.Quit()
	return nil
}

func getSmtpFingerprint(cfg *config.RuntimeSettings) string {
	return fmt.Sprintf("%s|%d|%t|%s|%s|%s|%s",
		cfg.SmtpHost, cfg.SmtpPort, cfg.SmtpSecure,
		cfg.SmtpUser, cfg.SmtpPass, cfg.SmtpFrom, cfg.SmtpTo,
	)
}
