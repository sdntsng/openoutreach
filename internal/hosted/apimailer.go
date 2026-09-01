package hosted

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
)

const AccountProviderResend = "resend"
const AccountProviderCFEmail = "cf_email"

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

func (s *Server) handleAddCFEmailAccount(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["cf_email"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_CF_EMAIL is disabled")
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
		APIToken   string `json:"api_token"`
		AccountID  string `json:"account_id"`
		DailyLimit int    `json:"daily_limit"`
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.APIToken) == "" || strings.TrimSpace(req.AccountID) == "" {
		writeErr(w, http.StatusBadRequest, "invalid_json", "email, api_token, and account_id are required")
		return
	}
	if req.DailyLimit <= 0 {
		req.DailyLimit = 50
	}
	ws := s.workspaceFromRequest(r)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	accountID := strings.TrimSpace(req.AccountID)
	meta, _ := json.Marshal(map[string]string{"account_id": accountID})
	cred, err := PutIntegrationCredential(s.Store.DB, key, ws, IntegrationCredentialInput{
		Provider: AccountProviderCFEmail,
		Name:     email,
		Secret:   req.APIToken,
		Metadata: string(meta),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "vault_failed", err.Error())
		return
	}
	var id int64
	err = queryRow(s.Store.DB, `
		INSERT INTO accounts (workspace_id, email, daily_limit, provider, gws_config_dir)
		VALUES (?, ?, ?, ?, '')
		RETURNING id`, ws, email, req.DailyLimit, AccountProviderCFEmail).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "add_failed", err.Error())
		return
	}
	if s.GWS != nil {
		if s.GWS.APIMailers == nil {
			s.GWS.APIMailers = map[string]*APIMailerProvider{}
		}
		s.GWS.APIMailers[email] = &APIMailerProvider{
			Provider: AccountProviderCFEmail, APIKey: req.APIToken, From: email, AccountID: accountID,
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"id": id, "email": email, "provider": AccountProviderCFEmail, "daily_limit": req.DailyLimit,
		"credential_id": cred.ID,
	}, Warnings: []string{"Cloudflare Email Sending is transactional; route inbound mail to this Worker for replies. API token never returned."}})
}

func (s *Server) hydrateAPIMailers() {
	if s.GWS == nil {
		return
	}
	key := s.encKey()
	if key == nil {
		return
	}
	rows, err := query(s.Store.DB, `SELECT email, provider FROM accounts WHERE status = 'active' AND provider IN (?, ?)`,
		AccountProviderResend, AccountProviderCFEmail)
	if err != nil {
		return
	}
	type row struct{ email, provider string }
	var list []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.email, &item.provider); err != nil {
			rows.Close()
			return
		}
		list = append(list, item)
	}
	rows.Close()
	if s.GWS.APIMailers == nil {
		s.GWS.APIMailers = map[string]*APIMailerProvider{}
	}
	ws := s.WorkspaceID
	for _, item := range list {
		secret, err := ResolveIntegrationSecret(s.Store.DB, key, ws, item.provider, item.email, 0)
		if err != nil {
			continue
		}
		p := &APIMailerProvider{Provider: item.provider, APIKey: secret, From: item.email}
		if item.provider == AccountProviderCFEmail {
			var meta string
			_ = queryRow(s.Store.DB, `SELECT metadata FROM integration_credentials WHERE workspace_id = ? AND provider = ? AND name = ?`,
				ws, item.provider, item.email).Scan(&meta)
			p.AccountID = jsonStringField(meta, "account_id")
		}
		s.GWS.APIMailers[strings.ToLower(item.email)] = p
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

func (s *Server) handleCFEmailInbound(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["cf_email"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_CF_EMAIL is disabled")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
		Raw  string `json:"raw"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	from := parseAddrEmail(req.From)
	to := parseAddrEmail(req.To)
	raw := req.Raw
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "invalid_json", "raw message is required")
		return
	}
	subject, text := parseRawMessage(raw)
	msgID := headerFromRaw(raw, "Message-ID")
	if msgID == "" {
		msgID = fmt.Sprintf("cf-in-%d", time.Now().UnixNano())
	}
	inReply := headerFromRaw(raw, "In-Reply-To")
	thread := strings.TrimSpace(headerFromRaw(raw, "References"))
	if thread == "" {
		thread = inReply
	}
	// Cloudflare Email Sending generates Message-ID; replies In-Reply-To that
	// provider id, not the RFC id we stored. Overlay the lead's last sent thread.
	if storedMsg, storedThread := s.threadFromLeadSent(from); storedMsg != "" || storedThread != "" {
		if storedThread != "" {
			thread = storedThread
		} else {
			thread = storedMsg
		}
	}
	var acct internal.Account
	err := queryRow(s.Store.DB, `SELECT id, email, provider FROM accounts WHERE lower(email) = ? LIMIT 1`, strings.ToLower(to)).
		Scan(&acct.ID, &acct.Email, &acct.Provider)
	if err != nil {
		err = queryRow(s.Store.DB, `SELECT id, email, provider FROM accounts WHERE status = 'active' AND provider = ? ORDER BY id LIMIT 1`, AccountProviderCFEmail).
			Scan(&acct.ID, &acct.Email, &acct.Provider)
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "account_not_found", "no sending account matches inbound recipient")
		return
	}
	snippet := text
	if len(snippet) > 240 {
		snippet = snippet[:240]
	}
	msg := internal.GWSMessage{
		ID:        msgID,
		ThreadID:  thread,
		Snippet:   snippet,
		TextBody:  text,
		From:      from,
		To:        to,
		Subject:   subject,
		InReplyTo: inReply,
		Headers: map[string]string{
			"Message-ID":  msgID,
			"In-Reply-To": inReply,
			"From":        from,
			"To":          to,
			"Subject":     subject,
		},
		Date: time.Now().UTC(),
	}
	res, err := internal.IngestInboundMessage(s.Store.DB, acct, msg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ingest_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"replies": res.Replies, "unsubscribes": res.Unsubscribes, "bounces": res.Bounces,
	}})
}

func (s *Server) threadFromLeadSent(from string) (messageID, threadID string) {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" || !strings.Contains(from, "@") {
		return "", ""
	}
	_ = queryRow(s.Store.DB, `
		SELECT e.message_id, COALESCE(e.thread_id, '')
		FROM events e
		JOIN leads l ON l.id = e.lead_id
		WHERE lower(l.email) = ? AND e.type = 'sent'
		ORDER BY e.id DESC LIMIT 1`, from).Scan(&messageID, &threadID)
	return messageID, threadID
}

func parseAddrEmail(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(v); err == nil && addr.Address != "" {
		return strings.ToLower(addr.Address)
	}
	return strings.ToLower(v)
}
