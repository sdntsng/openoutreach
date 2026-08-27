package hosted

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const AccountProviderResend = "resend"

func (s *Server) handleAddResendAccount(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["resend"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_RESEND is disabled")
		return
	}
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Email      string `json:"email"`
		APIKey     string `json:"api_key"`
		DailyLimit int    `json:"daily_limit"`
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.APIKey) == "" {
		writeErr(w, http.StatusBadRequest, "invalid_json", "email and api_key are required")
		return
	}
	if req.DailyLimit <= 0 {
		req.DailyLimit = 50
	}
	ws := s.workspaceFromRequest(r)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	cred, err := PutIntegrationCredential(s.Store.DB, key, ws, IntegrationCredentialInput{
		Provider: "resend",
		Name:     email,
		Secret:   req.APIKey,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "vault_failed", err.Error())
		return
	}
	var id int64
	err = queryRow(s.Store.DB, `
		INSERT INTO accounts (workspace_id, email, daily_limit, provider, gws_config_dir)
		VALUES (?, ?, ?, ?, '')
		RETURNING id`, ws, email, req.DailyLimit, AccountProviderResend).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "add_failed", err.Error())
		return
	}
	if s.GWS != nil {
		if s.GWS.APIMailers == nil {
			s.GWS.APIMailers = map[string]*APIMailerProvider{}
		}
		s.GWS.APIMailers[email] = &APIMailerProvider{Provider: "resend", APIKey: req.APIKey, From: email}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"id": id, "email": email, "provider": AccountProviderResend, "daily_limit": req.DailyLimit,
		"credential_id": cred.ID,
	}, Warnings: []string{"send-only; configure bounce webhook; API key never returned"}})
}

func (s *Server) hydrateAPIMailers() {
	if s.GWS == nil {
		return
	}
	key := s.encKey()
	if key == nil {
		return
	}
	rows, err := query(s.Store.DB, `SELECT email FROM accounts WHERE status = 'active' AND provider = ?`, AccountProviderResend)
	if err != nil {
		return
	}
	defer rows.Close()
	if s.GWS.APIMailers == nil {
		s.GWS.APIMailers = map[string]*APIMailerProvider{}
	}
	ws := s.WorkspaceID
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return
		}
		secret, err := ResolveIntegrationSecret(s.Store.DB, key, ws, "resend", email, 0)
		if err != nil {
			continue
		}
		s.GWS.APIMailers[strings.ToLower(email)] = &APIMailerProvider{Provider: "resend", APIKey: secret, From: email}
	}
}

func (s *Server) handleResendEvents(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["resend"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_RESEND is disabled")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var payload struct {
		Type string `json:"type"`
		Data struct {
			To      []string `json:"to"`
			EmailID string   `json:"email_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	kind := strings.ToLower(payload.Type)
	if !strings.Contains(kind, "bounce") && !strings.Contains(kind, "failed") {
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"ignored": true, "type": payload.Type}})
		return
	}
	var marked int
	for _, to := range payload.Data.To {
		if markLeadBounced(s.Store.DB, strings.ToLower(strings.TrimSpace(to)), payload.Data.EmailID) {
			marked++
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"marked": marked}})
}

func markLeadBounced(db *sql.DB, email, messageID string) bool {
	if email == "" || !strings.Contains(email, "@") {
		return false
	}
	var leadID int64
	if err := queryRow(db, `SELECT id FROM leads WHERE lower(email) = ?`, email).Scan(&leadID); err != nil {
		return false
	}
	_, _ = exec(db, `UPDATE leads SET global_status = 'bounced' WHERE id = ?`, leadID)
	_, _ = exec(db, `UPDATE campaign_leads SET status = 'bounced' WHERE lead_id = ? AND status IN ('active', 'pending')`, leadID)
	_, _ = exec(db, `UPDATE scheduled_sends SET status = 'skipped' WHERE lead_id = ? AND status = 'pending'`, leadID)
	if messageID != "" {
		_, _ = exec(db, `INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, timestamp)
			SELECT cl.campaign_id, ?, COALESCE(ss.account_id, 0), 'bounce', 0, ?, CURRENT_TIMESTAMP
			FROM campaign_leads cl
			LEFT JOIN scheduled_sends ss ON ss.campaign_id = cl.campaign_id AND ss.lead_id = cl.lead_id
			WHERE cl.lead_id = ?
			LIMIT 1`, leadID, messageID, leadID)
	}
	return true
}
