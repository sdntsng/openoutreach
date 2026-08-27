package hosted

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) encKey() []byte {
	if store, ok := s.Creds.(*DBCredentialStore); ok && store != nil {
		return store.Key
	}
	return nil
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	writeJSON(w, http.StatusOK, envelope{Data: caps})
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	list, err := ListIntegrationCredentials(s.Store.DB, key, s.workspaceFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if list == nil {
		list = []IntegrationCredential{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"integrations": list}})
}

func (s *Server) handlePutIntegration(w http.ResponseWriter, r *http.Request) {
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var in IntegrationCredentialInput
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	cred, err := PutIntegrationCredential(s.Store.DB, key, s.workspaceFromRequest(r), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "put_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: cred, Warnings: []string{"secret is stored encrypted and never returned in full"}})
}

func (s *Server) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "invalid credential id")
		return
	}
	if err := DeleteIntegrationCredential(s.Store.DB, s.workspaceFromRequest(r), id); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"deleted": true, "id": id}})
}

func (s *Server) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ID       int64  `json:"id"`
		Provider string `json:"provider"`
		Name     string `json:"name"`
	}
	_ = json.Unmarshal(body, &req)
	if idStr := r.PathValue("id"); idStr != "" && req.ID == 0 {
		req.ID, _ = strconv.ParseInt(idStr, 10, 64)
	}
	secret, err := ResolveIntegrationSecret(s.Store.DB, key, ws, req.Provider, req.Name, req.ID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "resolve_failed", err.Error())
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" && req.ID > 0 {
		_ = queryRow(s.Store.DB, `SELECT provider FROM integration_credentials WHERE id = ?`, req.ID).Scan(&provider)
	}
	ok, detail, testErr := TestIntegrationProvider(provider, secret)
	_ = LogEnrichmentCall(s.Store.DB, ws, provider, "test", detail, 0)
	if testErr != nil {
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
			"ok": false, "provider": provider, "detail": testErr.Error(),
		}, Warnings: []string{"connection test failed"}})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"ok": ok, "provider": provider, "detail": detail, "secret_hint": maskSecret(secret),
	}})
}

// TestIntegrationProvider validates a provider key without mutating campaigns.
func TestIntegrationProvider(provider, secret string) (bool, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return false, "", fmt.Errorf("secret is empty")
	}
	switch provider {
	case "apollo":
		return testApolloKey(secret)
	case "clay", "webhook", "sheets", "secret", "smtp_password", "resend", "ses", "hunter", "warmup":
		return true, "credential present (provider does not require live ping)", nil
	case "microsoft", "gmail":
		return true, "use OAuth connect flow for mailbox providers", nil
	default:
		if len(secret) >= 8 {
			return true, "credential stored; no live probe for provider " + provider, nil
		}
		return false, "", fmt.Errorf("secret looks too short")
	}
}
