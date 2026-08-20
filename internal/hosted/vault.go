package hosted

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GoogleCredential is the decrypted in-memory credential (never log tokens).
type GoogleCredential struct {
	ID              int64
	WorkspaceID     string
	AccountID       int64
	GoogleAccountID string
	RefreshToken    string
	AccessToken     string
	TokenExpiry     time.Time
	Scopes          string
}

// CredentialStore persists encrypted Google OAuth credentials.
type CredentialStore interface {
	GetGoogleCredential(ctx context.Context, accountID string) (*GoogleCredential, error)
	GetGoogleCredentialByAccountID(ctx context.Context, accountID int64) (*GoogleCredential, error)
	PutGoogleCredential(ctx context.Context, credential *GoogleCredential) error
	DeleteGoogleCredential(ctx context.Context, accountID string) error
	DeleteGoogleCredentialByAccountID(ctx context.Context, accountID int64) error
}

type DBCredentialStore struct {
	DB  *sql.DB
	Key []byte
}

func NewDBCredentialStore(db *sql.DB, key []byte) *DBCredentialStore {
	return &DBCredentialStore{DB: db, Key: key}
}

func (s *DBCredentialStore) GetGoogleCredential(ctx context.Context, accountID string) (*GoogleCredential, error) {
	accountID = strings.TrimSpace(accountID)
	var id int64
	if _, err := fmt.Sscan(accountID, &id); err != nil {
		return nil, fmt.Errorf("invalid account id")
	}
	return s.GetGoogleCredentialByAccountID(ctx, id)
}

func (s *DBCredentialStore) GetGoogleCredentialByAccountID(ctx context.Context, accountID int64) (*GoogleCredential, error) {
	_ = ctx
	return s.getByAccountID(accountID)
}

func (s *DBCredentialStore) getByAccountID(accountID int64) (*GoogleCredential, error) {
	var (
		c              GoogleCredential
		encRefresh     string
		encAccess      string
		expiryNull     sql.NullTime
	)
	err := queryRow(s.DB, `
		SELECT id, workspace_id, account_id, google_account_id,
			encrypted_refresh_token, encrypted_access_token, token_expiry, scopes
		FROM google_credentials WHERE account_id = ?`, accountID).Scan(
		&c.ID, &c.WorkspaceID, &c.AccountID, &c.GoogleAccountID,
		&encRefresh, &encAccess, &expiryNull, &c.Scopes,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refresh, err := Decrypt(s.Key, encRefresh)
	if err != nil {
		return nil, fmt.Errorf("decrypt refresh token: %w", err)
	}
	c.RefreshToken = string(refresh)
	if encAccess != "" {
		access, err := Decrypt(s.Key, encAccess)
		if err != nil {
			return nil, fmt.Errorf("decrypt access token: %w", err)
		}
		c.AccessToken = string(access)
	}
	if expiryNull.Valid {
		c.TokenExpiry = expiryNull.Time
	}
	return &c, nil
}

func (s *DBCredentialStore) PutGoogleCredential(ctx context.Context, credential *GoogleCredential) error {
	if credential == nil {
		return fmt.Errorf("credential is nil")
	}
	encRefresh, err := Encrypt(s.Key, []byte(credential.RefreshToken))
	if err != nil {
		return err
	}
	encAccess := ""
	if credential.AccessToken != "" {
		encAccess, err = Encrypt(s.Key, []byte(credential.AccessToken))
		if err != nil {
			return err
		}
	}
	workspaceID := strings.TrimSpace(credential.WorkspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	var expiry any
	if !credential.TokenExpiry.IsZero() {
		expiry = credential.TokenExpiry.UTC()
	}
	_, err = exec(s.DB, `
		INSERT INTO google_credentials (
			workspace_id, account_id, google_account_id,
			encrypted_refresh_token, encrypted_access_token, token_expiry, scopes, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			google_account_id = excluded.google_account_id,
			encrypted_refresh_token = excluded.encrypted_refresh_token,
			encrypted_access_token = excluded.encrypted_access_token,
			token_expiry = excluded.token_expiry,
			scopes = excluded.scopes,
			updated_at = CURRENT_TIMESTAMP
	`, workspaceID, credential.AccountID, credential.GoogleAccountID,
		encRefresh, encAccess, expiry, credential.Scopes)
	return err
}

func (s *DBCredentialStore) DeleteGoogleCredential(ctx context.Context, accountID string) error {
	var id int64
	if _, err := fmt.Sscan(strings.TrimSpace(accountID), &id); err != nil {
		return fmt.Errorf("invalid account id")
	}
	return s.DeleteGoogleCredentialByAccountID(ctx, id)
}

func (s *DBCredentialStore) DeleteGoogleCredentialByAccountID(ctx context.Context, accountID int64) error {
	_, err := exec(s.DB, `DELETE FROM google_credentials WHERE account_id = ?`, accountID)
	return err
}
