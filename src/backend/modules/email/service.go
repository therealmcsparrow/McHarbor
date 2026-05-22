// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	coreEmail "github.com/therealmcsparrow/mcharbor/core/email"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Service handles email server business logic and database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new email server service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: database, enc: enc}
}

// List returns all email servers (secrets excluded).
func (s *Service) List(ctx context.Context) ([]EmailServer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, server_type, is_default, enabled, host, port,
		        encryption, auth_method, username, client_id, tenant_id,
		        from_address, from_name, created_at, updated_at
		 FROM email_servers ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing email servers: %w", err)
	}
	defer rows.Close()

	var items []EmailServer
	for rows.Next() {
		srv, err := scanEmailServer(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning email server row: %w", err)
		}
		items = append(items, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating email server rows: %w", err)
	}

	if items == nil {
		items = []EmailServer{}
	}

	return items, nil
}

// ByID returns a single email server by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*EmailServer, error) {
	var srv EmailServer
	var host, encr, authMethod, username, clientID, tenantID, fromName sql.NullString
	var port sql.NullInt64
	var isDefault, enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, server_type, is_default, enabled, host, port,
		        encryption, auth_method, username, client_id, tenant_id,
		        from_address, from_name, created_at, updated_at
		 FROM email_servers WHERE id = ?`, id,
	).Scan(&srv.ID, &srv.Name, &srv.ServerType, &isDefault, &enabled,
		&host, &port, &encr, &authMethod, &username, &clientID, &tenantID,
		&srv.FromAddress, &fromName, &srv.CreatedAt, &srv.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting email server %s: %w", id, err)
	}

	srv.IsDefault = isDefault.Bool
	srv.Enabled = enabled.Bool
	srv.Host = host.String
	srv.Port = int(port.Int64)
	srv.Encryption = encr.String
	srv.AuthMethod = authMethod.String
	srv.Username = username.String
	srv.ClientID = clientID.String
	srv.TenantID = tenantID.String
	srv.FromName = fromName.String

	return &srv, nil
}

// Create inserts a new email server, encrypting sensitive fields.
func (s *Service) Create(ctx context.Context, input CreateEmailServerInput) (*EmailServer, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var encPassword, encClientSecret string
	var err error

	if input.Password != "" {
		encPassword, err = s.enc.Encrypt(input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting password: %w", err)
		}
	}

	if input.ClientSecret != "" {
		encClientSecret, err = s.enc.Encrypt(input.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting client secret: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// If this server is the default, clear existing defaults
	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO email_servers (id, name, server_type, is_default, enabled, host, port,
		 encryption, auth_method, username, password, client_id, client_secret, tenant_id,
		 from_address, from_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ServerType, input.IsDefault,
		nullStr(input.Host), nullInt(input.Port),
		nullStr(input.Encryption), nullStr(input.AuthMethod),
		nullStr(input.Username), nullStr(encPassword),
		nullStr(input.ClientID), nullStr(encClientSecret),
		nullStr(input.TenantID), input.FromAddress, nullStr(input.FromName),
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting email server: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &EmailServer{
		ID:          id,
		Name:        input.Name,
		ServerType:  input.ServerType,
		IsDefault:   input.IsDefault,
		Enabled:     true,
		Host:        input.Host,
		Port:        input.Port,
		Encryption:  input.Encryption,
		AuthMethod:  input.AuthMethod,
		Username:    input.Username,
		ClientID:    input.ClientID,
		TenantID:    input.TenantID,
		FromAddress: input.FromAddress,
		FromName:    input.FromName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Update applies partial updates to an email server.
func (s *Service) Update(ctx context.Context, id string, input UpdateEmailServerInput) (*EmailServer, error) {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM email_servers WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking email server existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if input.Name != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating email server name: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating email server enabled: %w", err)
		}
	}
	if input.IsDefault != nil && *input.IsDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
			return nil, fmt.Errorf("setting new default: %w", err)
		}
	} else if input.IsDefault != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
			return nil, fmt.Errorf("clearing default flag: %w", err)
		}
	}
	if input.Host != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET host = ?, updated_at = ? WHERE id = ?", *input.Host, now, id); err != nil {
			return nil, fmt.Errorf("updating email server host: %w", err)
		}
	}
	if input.Port != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET port = ?, updated_at = ? WHERE id = ?", *input.Port, now, id); err != nil {
			return nil, fmt.Errorf("updating email server port: %w", err)
		}
	}
	if input.Encryption != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET encryption = ?, updated_at = ? WHERE id = ?", *input.Encryption, now, id); err != nil {
			return nil, fmt.Errorf("updating email server encryption: %w", err)
		}
	}
	if input.AuthMethod != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET auth_method = ?, updated_at = ? WHERE id = ?", *input.AuthMethod, now, id); err != nil {
			return nil, fmt.Errorf("updating email server auth method: %w", err)
		}
	}
	if input.Username != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET username = ?, updated_at = ? WHERE id = ?", *input.Username, now, id); err != nil {
			return nil, fmt.Errorf("updating email server username: %w", err)
		}
	}
	if input.Password != nil && *input.Password != "" {
		enc, err := s.enc.Encrypt(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting password: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET password = ?, updated_at = ? WHERE id = ?", enc, now, id); err != nil {
			return nil, fmt.Errorf("updating email server password: %w", err)
		}
	}
	if input.ClientID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET client_id = ?, updated_at = ? WHERE id = ?", *input.ClientID, now, id); err != nil {
			return nil, fmt.Errorf("updating email server client id: %w", err)
		}
	}
	if input.ClientSecret != nil && *input.ClientSecret != "" {
		enc, err := s.enc.Encrypt(*input.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting client secret: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET client_secret = ?, updated_at = ? WHERE id = ?", enc, now, id); err != nil {
			return nil, fmt.Errorf("updating email server client secret: %w", err)
		}
	}
	if input.TenantID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET tenant_id = ?, updated_at = ? WHERE id = ?", *input.TenantID, now, id); err != nil {
			return nil, fmt.Errorf("updating email server tenant id: %w", err)
		}
	}
	if input.FromAddress != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET from_address = ?, updated_at = ? WHERE id = ?", *input.FromAddress, now, id); err != nil {
			return nil, fmt.Errorf("updating email server from address: %w", err)
		}
	}
	if input.FromName != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET from_name = ?, updated_at = ? WHERE id = ?", *input.FromName, now, id); err != nil {
			return nil, fmt.Errorf("updating email server from name: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes an email server by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM email_servers WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting email server %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// SetDefault sets the given email server as the default, clearing any existing default.
func (s *Service) SetDefault(ctx context.Context, id string) error {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM email_servers WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return fmt.Errorf("email server not found")
	} else if err != nil {
		return fmt.Errorf("checking email server existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
		return fmt.Errorf("clearing existing default: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE email_servers SET is_default = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
		return fmt.Errorf("setting new default: %w", err)
	}

	return tx.Commit()
}

// Test sends a test email using the specified email server.
func (s *Service) Test(ctx context.Context, id string, to string) error {
	var serverType, host, encr, authMethod, username, password sql.NullString
	var clientID, clientSecret, tenantID, fromAddress, fromName sql.NullString
	var port sql.NullInt64

	err := s.db.QueryRowContext(ctx,
		`SELECT server_type, host, port, encryption, auth_method, username, password,
		        client_id, client_secret, tenant_id, from_address, from_name
		 FROM email_servers WHERE id = ?`, id,
	).Scan(&serverType, &host, &port, &encr, &authMethod, &username, &password,
		&clientID, &clientSecret, &tenantID, &fromAddress, &fromName)

	if err == sql.ErrNoRows {
		return fmt.Errorf("email server not found")
	}
	if err != nil {
		return fmt.Errorf("looking up email server %s: %w", id, err)
	}

	subject := "McHarbor Test Email"
	body := "<h2>McHarbor Email Test</h2><p>This is a test email sent from McHarbor to verify your email server configuration.</p>"

	switch serverType.String {
	case "smtp":
		decPassword := ""
		if password.String != "" {
			decPassword, err = s.enc.Decrypt(password.String)
			if err != nil {
				return fmt.Errorf("decrypting password: %w", err)
			}
		}
		return coreEmail.SendSMTP(ctx, coreEmail.SMTPConfig{
			Host:        host.String,
			Port:        int(port.Int64),
			Encryption:  encr.String,
			AuthMethod:  authMethod.String,
			Username:    username.String,
			Password:    decPassword,
			FromAddress: fromAddress.String,
			FromName:    fromName.String,
		}, to, subject, body)

	case "exchange":
		decSecret := ""
		if clientSecret.String != "" {
			decSecret, err = s.enc.Decrypt(clientSecret.String)
			if err != nil {
				return fmt.Errorf("decrypting client secret: %w", err)
			}
		}
		return coreEmail.SendExchange(ctx, coreEmail.OAuthConfig{
			ClientID:     clientID.String,
			ClientSecret: decSecret,
			TenantID:     tenantID.String,
			FromAddress:  fromAddress.String,
			FromName:     fromName.String,
		}, to, subject, body)

	case "gmail":
		decSecret := ""
		if clientSecret.String != "" {
			decSecret, err = s.enc.Decrypt(clientSecret.String)
			if err != nil {
				return fmt.Errorf("decrypting client secret: %w", err)
			}
		}
		return coreEmail.SendGmail(ctx, coreEmail.OAuthConfig{
			ClientID:     clientID.String,
			ClientSecret: decSecret,
			FromAddress:  fromAddress.String,
			FromName:     fromName.String,
		}, to, subject, body)

	default:
		return fmt.Errorf("unsupported server type: %s", serverType.String)
	}
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt(n int) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(n), Valid: true}
}

func scanEmailServer(rows *sql.Rows) (EmailServer, error) {
	var srv EmailServer
	var host, encr, authMethod, username, clientID, tenantID, fromName sql.NullString
	var port sql.NullInt64
	var isDefault, enabled sql.NullBool

	if err := rows.Scan(&srv.ID, &srv.Name, &srv.ServerType, &isDefault, &enabled,
		&host, &port, &encr, &authMethod, &username, &clientID, &tenantID,
		&srv.FromAddress, &fromName, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
		return EmailServer{}, err
	}

	srv.IsDefault = isDefault.Bool
	srv.Enabled = enabled.Bool
	srv.Host = host.String
	srv.Port = int(port.Int64)
	srv.Encryption = encr.String
	srv.AuthMethod = authMethod.String
	srv.Username = username.String
	srv.ClientID = clientID.String
	srv.TenantID = tenantID.String
	srv.FromName = fromName.String

	return srv, nil
}
