// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const smtpTimeout = 15 * time.Second

type smtpClient struct{}

type smtpConfig struct {
	Host          string
	Port          int
	Encryption    string
	AuthMethod    string
	Username      string
	Password      string
	FromAddress   string
	FromName      string
	Recipients    []string
	SubjectPrefix string
}

// NewSMTPClient returns a stateless NotificationAdapterClient.
func NewSMTPClient() *smtpClient { return &smtpClient{} }

// Test sends a real test email through the configured SMTP relay.
func (c *smtpClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseSMTPConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor SMTP communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromSMTPConfig(parsed), msg)
}

// Send delivers the notification as a plain text email.
func (c *smtpClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseSMTPConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-smtp: empty payload (title and message both blank)")
	}

	subject := buildSubject(parsed.SubjectPrefix, msg.Title)
	body := buildBody(msg)
	raw := buildEmail(parsed.FromHeader(), parsed.Recipients, subject, body, msg.Files)

	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()
	return sendSMTP(ctx, parsed, raw)
}

func parseSMTPConfig(cfg map[string]string) (smtpConfig, error) {
	host := strings.TrimSpace(cfg["host"])
	if host == "" {
		return smtpConfig{}, errors.New("comm-smtp: smtp host is required")
	}

	portText := strings.TrimSpace(cfg["port"])
	if portText == "" {
		portText = "587"
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return smtpConfig{}, fmt.Errorf("comm-smtp: invalid smtp port %q", portText)
	}

	encryption, ok := normalizeEncryption(firstNonEmpty(cfg["encryption"], cfg["security"]))
	if !ok {
		return smtpConfig{}, errors.New("comm-smtp: encryption must be one of starttls, ssl_tls, or none")
	}

	username := strings.TrimSpace(cfg["username"])
	password := strings.TrimSpace(cfg["password"])
	authMethod := strings.ToLower(strings.TrimSpace(cfg["auth_method"]))
	if authMethod == "" {
		if username != "" || password != "" {
			authMethod = "plain"
		} else {
			authMethod = "none"
		}
	}
	if authMethod != "none" && authMethod != "plain" && authMethod != "login" && authMethod != "cram_md5" {
		return smtpConfig{}, errors.New("comm-smtp: auth_method must be one of none, plain, login, or cram_md5")
	}
	if authMethod == "none" {
		username = ""
		password = ""
	} else if username == "" || password == "" {
		return smtpConfig{}, errors.New("comm-smtp: username and password are required when authentication is enabled")
	}

	fromAddress := strings.TrimSpace(firstNonEmpty(cfg["from_address"], cfg["from"]))
	parsedFrom, err := mail.ParseAddress(fromAddress)
	if err != nil {
		return smtpConfig{}, fmt.Errorf("comm-smtp: invalid from address: %w", err)
	}
	fromName := strings.TrimSpace(cfg["from_name"])

	recipients, err := parseRecipients(cfg["to"])
	if err != nil {
		return smtpConfig{}, err
	}

	return smtpConfig{
		Host:          host,
		Port:          port,
		Encryption:    encryption,
		AuthMethod:    authMethod,
		Username:      username,
		Password:      password,
		FromAddress:   parsedFrom.Address,
		FromName:      fromName,
		Recipients:    recipients,
		SubjectPrefix: strings.TrimSpace(cfg["subject_prefix"]),
	}, nil
}

func parseRecipients(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}
		parsed, err := mail.ParseAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("comm-smtp: invalid recipient address %q: %w", addr, err)
		}
		recipients = append(recipients, parsed.Address)
	}
	if len(recipients) == 0 {
		return nil, errors.New("comm-smtp: at least one recipient is required")
	}
	return recipients, nil
}

func sendSMTP(ctx context.Context, cfg smtpConfig, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: smtpTimeout}

	var conn net.Conn
	var err error
	if cfg.Encryption == "ssl_tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("comm-smtp: connect to smtp server %s: %w", addr, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("comm-smtp: create smtp client: %w", err)
	}
	defer client.Close()

	if cfg.Encryption == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("comm-smtp: server does not advertise STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("comm-smtp: starttls failed: %w", err)
		}
	}

	if auth := buildAuth(cfg); auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("comm-smtp: smtp auth failed: %w", err)
		}
	}
	if err := client.Mail(cfg.FromAddress); err != nil {
		return fmt.Errorf("comm-smtp: set sender: %w", err)
	}
	for _, recipient := range cfg.Recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("comm-smtp: add recipient %s: %w", recipient, err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("comm-smtp: open data writer: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("comm-smtp: write message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("comm-smtp: finish message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("comm-smtp: quit smtp session: %w", err)
	}
	return nil
}

func buildAuth(cfg smtpConfig) smtp.Auth {
	switch cfg.AuthMethod {
	case "plain":
		return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	case "login":
		return loginAuth{username: cfg.Username, password: cfg.Password}
	case "cram_md5":
		return smtp.CRAMMD5Auth(cfg.Username, cfg.Password)
	default:
		return nil
	}
}

type loginAuth struct {
	username string
	password string
}

func (a loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected server challenge: %s", string(fromServer))
	}
}

func buildSubject(prefix, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "McHarbor notification"
	}
	if prefix == "" {
		return title
	}
	return strings.TrimSpace(prefix + " " + title)
}

func buildBody(msg plugin.NotificationMessage) string {
	var b strings.Builder
	if level := strings.TrimSpace(msg.Level); level != "" {
		b.WriteString("Level: ")
		b.WriteString(level)
		b.WriteString("\n\n")
	}
	if title := strings.TrimSpace(msg.Title); title != "" {
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(msg.Message))
	b.WriteString("\n")
	return b.String()
}

func buildEmail(from string, recipients []string, subject, body string, files []plugin.NotificationFile) []byte {
	var b strings.Builder
	w := bufio.NewWriter(&b)
	fmt.Fprintf(w, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(w, "To: %s\r\n", sanitizeHeader(strings.Join(recipients, ", ")))
	fmt.Fprintf(w, "Subject: %s\r\n", sanitizeHeader(subject))
	fmt.Fprint(w, "MIME-Version: 1.0\r\n")

	// Without attachments the message stays a single text/plain part —
	// the shape every mail client renders most predictably. Wrapping a
	// plain note in multipart just because the code can would make every
	// existing notification slightly worse.
	if len(files) == 0 {
		fmt.Fprint(w, "Content-Type: text/plain; charset=UTF-8\r\n")
		fmt.Fprint(w, "Content-Transfer-Encoding: 8bit\r\n")
		fmt.Fprint(w, "\r\n")
		fmt.Fprint(w, body)
		_ = w.Flush()
		return []byte(b.String())
	}

	boundary := mimeBoundary()
	fmt.Fprintf(w, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	fmt.Fprint(w, "\r\n")

	fmt.Fprintf(w, "--%s\r\n", boundary)
	fmt.Fprint(w, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprint(w, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprint(w, "\r\n")
	fmt.Fprint(w, body)
	fmt.Fprint(w, "\r\n")

	for _, f := range files {
		contentType := strings.TrimSpace(f.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		name := sanitizeHeader(strings.TrimSpace(f.Filename))
		if name == "" {
			name = "attachment"
		}

		fmt.Fprintf(w, "--%s\r\n", boundary)
		fmt.Fprintf(w, "Content-Type: %s; name=%q\r\n", sanitizeHeader(contentType), name)
		fmt.Fprint(w, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(w, "Content-Disposition: attachment; filename=%q\r\n", name)
		fmt.Fprint(w, "\r\n")
		writeBase64Lines(w, f.Data)
		fmt.Fprint(w, "\r\n")
	}

	fmt.Fprintf(w, "--%s--\r\n", boundary)
	_ = w.Flush()
	return []byte(b.String())
}

// mimeBoundary returns a boundary that cannot collide with message
// content. crypto/rand rather than math/rand: a predictable boundary
// appearing inside an attachment would truncate the message.
func mimeBoundary() string {
	var buf [18]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// A boundary must exist even if the entropy source fails; the
		// timestamp is unique enough to keep the message well-formed.
		return fmt.Sprintf("mcharbor-%d", time.Now().UnixNano())
	}
	return "mcharbor-" + base64.RawURLEncoding.EncodeToString(buf[:])
}

// writeBase64Lines emits base64 wrapped at 76 characters, as RFC 2045
// requires. Mail servers may reject or mangle longer lines.
func writeBase64Lines(w *bufio.Writer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	for len(encoded) > lineLen {
		fmt.Fprint(w, encoded[:lineLen], "\r\n")
		encoded = encoded[lineLen:]
	}
	if encoded != "" {
		fmt.Fprint(w, encoded, "\r\n")
	}
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

func configMapFromSMTPConfig(cfg smtpConfig) map[string]string {
	return map[string]string{
		"host":           cfg.Host,
		"port":           strconv.Itoa(cfg.Port),
		"encryption":     cfg.Encryption,
		"auth_method":    cfg.AuthMethod,
		"username":       cfg.Username,
		"password":       cfg.Password,
		"from_address":   cfg.FromAddress,
		"from_name":      cfg.FromName,
		"to":             strings.Join(cfg.Recipients, ","),
		"subject_prefix": cfg.SubjectPrefix,
	}
}

func (cfg smtpConfig) FromHeader() string {
	if cfg.FromName == "" {
		return cfg.FromAddress
	}
	return (&mail.Address{Name: cfg.FromName, Address: cfg.FromAddress}).String()
}

func normalizeEncryption(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "starttls", true
	case "tls", "ssl_tls":
		return "ssl_tls", true
	case "starttls":
		return "starttls", true
	case "none":
		return "none", true
	default:
		return "", false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
