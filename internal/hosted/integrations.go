package hosted

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
)

// IntegrationCredential is a workspace-scoped API key (secret never returned to API/MCP).
type IntegrationCredential struct {
	ID          int64     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Provider    string    `json:"provider"`
	Name        string    `json:"name"`
	Metadata    string    `json:"metadata,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	HasSecret   bool      `json:"has_secret"`
	SecretHint  string    `json:"secret_hint,omitempty"`
}

// IntegrationCredentialInput creates or updates a credential.
type IntegrationCredentialInput struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Secret   string `json:"secret"`
	Metadata string `json:"metadata"`
	Status   string `json:"status"`
}

func maskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

// ListIntegrationCredentials returns masked credentials for a workspace.
func ListIntegrationCredentials(db *sql.DB, key []byte, workspaceID string) ([]IntegrationCredential, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	rows, err := query(db, `
		SELECT id, workspace_id, provider, name, encrypted_secret, metadata, status, created_at, updated_at
		FROM integration_credentials
		WHERE workspace_id = ?
		ORDER BY provider, name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IntegrationCredential
	for rows.Next() {
		var c IntegrationCredential
		var enc string
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.Name, &enc, &c.Metadata, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.HasSecret = enc != ""
		if key != nil && enc != "" {
			plain, err := Decrypt(key, enc)
			if err == nil {
				c.SecretHint = maskSecret(string(plain))
			} else {
				c.SecretHint = "****"
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutIntegrationCredential upserts an encrypted credential. Empty secret keeps existing.
func PutIntegrationCredential(db *sql.DB, key []byte, workspaceID string, in IntegrationCredentialInput) (*IntegrationCredential, error) {
	if key == nil {
		return nil, fmt.Errorf("encryption key is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	in.Name = strings.TrimSpace(in.Name)
	in.Secret = strings.TrimSpace(in.Secret)
	if in.Provider == "" || in.Name == "" {
		return nil, fmt.Errorf("provider and name are required")
	}
	if in.Metadata == "" {
		in.Metadata = "{}"
	}
	if in.Status == "" {
		in.Status = "active"
	}

	var existingID int64
	var existingEnc string
	err := queryRow(db, `
		SELECT id, encrypted_secret FROM integration_credentials
		WHERE workspace_id = ? AND provider = ? AND name = ?`,
		workspaceID, in.Provider, in.Name).Scan(&existingID, &existingEnc)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	enc := existingEnc
	if in.Secret != "" {
		enc, err = Encrypt(key, []byte(in.Secret))
		if err != nil {
			return nil, err
		}
	}
	if enc == "" {
		return nil, fmt.Errorf("secret is required for new credentials")
	}

	if existingID == 0 {
		res, err := exec(db, `
			INSERT INTO integration_credentials (workspace_id, provider, name, encrypted_secret, metadata, status)
			VALUES (?, ?, ?, ?, ?, ?)`,
			workspaceID, in.Provider, in.Name, enc, in.Metadata, in.Status)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		existingID = id
	} else {
		_, err = exec(db, `
			UPDATE integration_credentials
			SET encrypted_secret = ?, metadata = ?, status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, enc, in.Metadata, in.Status, existingID)
		if err != nil {
			return nil, err
		}
	}

	list, err := ListIntegrationCredentials(db, key, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == existingID || (list[i].Provider == in.Provider && list[i].Name == in.Name) {
			return &list[i], nil
		}
	}
	return &IntegrationCredential{
		ID: existingID, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name,
		Metadata: in.Metadata, Status: in.Status, HasSecret: true, SecretHint: maskSecret(in.Secret),
	}, nil
}

// DeleteIntegrationCredential removes a credential by id within a workspace.
func DeleteIntegrationCredential(db *sql.DB, workspaceID string, id int64) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	res, err := exec(db, `DELETE FROM integration_credentials WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

// ResolveIntegrationSecret decrypts a credential secret by id or provider+name.
func ResolveIntegrationSecret(db *sql.DB, key []byte, workspaceID, provider, name string, id int64) (string, error) {
	if key == nil {
		return "", fmt.Errorf("encryption key is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	var enc string
	var err error
	if id > 0 {
		err = queryRow(db, `SELECT encrypted_secret FROM integration_credentials WHERE id = ? AND workspace_id = ? AND status = 'active'`,
			id, workspaceID).Scan(&enc)
	} else {
		err = queryRow(db, `SELECT encrypted_secret FROM integration_credentials WHERE workspace_id = ? AND provider = ? AND name = ? AND status = 'active'`,
			workspaceID, strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(name)).Scan(&enc)
	}
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("credential not found")
	}
	if err != nil {
		return "", err
	}
	plain, err := Decrypt(key, enc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// LogEnrichmentCall records a connector usage event for cost visibility.
func LogEnrichmentCall(db *sql.DB, workspaceID, provider, operation, detail string, units float64) error {
	if units <= 0 {
		units = 1
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	_, err := exec(db, `
		INSERT INTO enrichment_calls (workspace_id, provider, units, operation, detail)
		VALUES (?, ?, ?, ?, ?)`,
		workspaceID, strings.ToLower(provider), units, operation, detail)
	return err
}

// HostedSecretResolver resolves secret:ID refs from integration_credentials (provider=secret).
type HostedSecretResolver struct {
	DB          *sql.DB
	Key         []byte
	WorkspaceID string
}

func (r HostedSecretResolver) ResolveSecret(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "env:") {
		return internal.ResolveSecretRef(ref)
	}
	if !strings.HasPrefix(ref, "secret:") {
		return "", fmt.Errorf("unsupported secret reference")
	}
	idStr := strings.TrimPrefix(ref, "secret:")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		return ResolveIntegrationSecret(r.DB, r.Key, r.WorkspaceID, "secret", idStr, 0)
	}
	return ResolveIntegrationSecret(r.DB, r.Key, r.WorkspaceID, "", "", id)
}

// EncryptionKeyBytes returns the server encryption key if CREDENTIAL_ENCRYPTION_KEY is set.
func EncryptionKeyBytes() []byte {
	raw := strings.TrimSpace(os.Getenv("CREDENTIAL_ENCRYPTION_KEY"))
	if raw == "" {
		return nil
	}
	k, err := DeriveKey(raw)
	if err != nil {
		return nil
	}
	return k
}
