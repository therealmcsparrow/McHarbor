// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"time"
)

// SMTPConfig holds SMTP server connection parameters.
type SMTPConfig struct {
	Host        string
	Port        int
	Encryption  string // "none", "starttls", "ssl_tls"
	AuthMethod  string // "none", "plain", "login", "cram_md5"
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

// SendSMTP sends an email via SMTP.
func SendSMTP(ctx context.Context, cfg SMTPConfig, to, subject, bodyHTML string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	msg := buildMIMEMessage(cfg.FromAddress, cfg.FromName, to, subject, bodyHTML)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendSMTPInternal(ctx, cfg, addr, to, msg)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("smtp send timed out: %w", ctx.Err())
	}
}

func sendSMTPInternal(ctx context.Context, cfg SMTPConfig, addr, to string, msg []byte) error {
	var conn net.Conn
	var err error

	tlsCfg := &tls.Config{ServerName: cfg.Host}

	if cfg.Encryption == "ssl_tls" {
		dialer := &tls.Dialer{Config: tlsCfg}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
	} else {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("tcp dial: %w", err)
		}
	}

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp new client: %w", errors.Join(err, conn.Close()))
	}
	defer func() {
		if cerr := c.Close(); cerr != nil && !errors.Is(cerr, net.ErrClosed) {
			slog.Warn("failed to close smtp client", "error", cerr)
		}
	}()

	if cfg.Encryption == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if cfg.AuthMethod != "none" && cfg.AuthMethod != "" {
		auth, err := buildAuth(cfg)
		if err != nil {
			return err
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := c.Mail(cfg.FromAddress); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}

	return c.Quit()
}

func buildAuth(cfg SMTPConfig) (smtp.Auth, error) {
	switch cfg.AuthMethod {
	case "plain":
		return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host), nil
	case "login":
		return LoginAuth(cfg.Username, cfg.Password), nil
	case "cram_md5":
		return smtp.CRAMMD5Auth(cfg.Username, cfg.Password), nil
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
	}
}

func buildMIMEMessage(fromAddr, fromName, to, subject, bodyHTML string) []byte {
	var from string
	if fromName != "" {
		from = fmt.Sprintf("%s <%s>", fromName, fromAddr)
	} else {
		from = fromAddr
	}

	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(bodyHTML)

	return buf.Bytes()
}
