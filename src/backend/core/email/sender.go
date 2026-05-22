// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strings"
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

// OAuthConfig holds OAuth2 client credentials for Exchange/Gmail.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	TenantID     string // Exchange only
	FromAddress  string
	FromName     string
}

// SendSMTP sends an email via SMTP.
func SendSMTP(ctx context.Context, cfg SMTPConfig, to, subject, bodyHTML string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	msg := buildMIMEMessage(cfg.FromAddress, cfg.FromName, to, subject, bodyHTML)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendSMTPInternal(cfg, addr, to, msg)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("smtp send timed out: %w", ctx.Err())
	}
}

func sendSMTPInternal(cfg SMTPConfig, addr, to string, msg []byte) error {
	var conn net.Conn
	var err error

	tlsCfg := &tls.Config{ServerName: cfg.Host}

	if cfg.Encryption == "ssl_tls" {
		conn, err = tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("tcp dial: %w", err)
		}
	}

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer c.Close()

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

// SendExchange sends an email via Microsoft Graph API using client credentials.
func SendExchange(ctx context.Context, cfg OAuthConfig, to, subject, bodyHTML string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := getExchangeToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("exchange token: %w", err)
	}

	payload := map[string]interface{}{
		"message": map[string]interface{}{
			"subject": subject,
			"body": map[string]string{
				"contentType": "HTML",
				"content":     bodyHTML,
			},
			"toRecipients": []map[string]interface{}{
				{
					"emailAddress": map[string]string{
						"address": to,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling exchange payload: %w", err)
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", cfg.FromAddress)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating exchange request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("exchange send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("exchange API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SendGmail sends an email via Gmail API using client credentials.
func SendGmail(ctx context.Context, cfg OAuthConfig, to, subject, bodyHTML string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := getGmailToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("gmail token: %w", err)
	}

	msg := buildMIMEMessage(cfg.FromAddress, cfg.FromName, to, subject, bodyHTML)
	encoded := base64.URLEncoding.EncodeToString(msg)

	payload := map[string]string{
		"raw": encoded,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling gmail payload: %w", err)
	}

	url := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/%s/messages/send", cfg.FromAddress)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating gmail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gmail send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("gmail API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getExchangeToken fetches an OAuth2 token via client credentials for Microsoft Graph.
func getExchangeToken(ctx context.Context, cfg OAuthConfig) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID)

	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://graph.microsoft.com/.default",
		cfg.ClientID, cfg.ClientSecret)

	return fetchOAuthToken(ctx, tokenURL, data)
}

// getGmailToken fetches an OAuth2 token via client credentials for Gmail API.
func getGmailToken(ctx context.Context, cfg OAuthConfig) (string, error) {
	tokenURL := "https://oauth2.googleapis.com/token"

	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://www.googleapis.com/auth/gmail.send",
		cfg.ClientID, cfg.ClientSecret)

	return fetchOAuthToken(ctx, tokenURL, data)
}

func fetchOAuthToken(ctx context.Context, tokenURL, formData string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(formData))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	if result.Error != "" {
		return "", fmt.Errorf("oauth2 error: %s — %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return result.AccessToken, nil
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
