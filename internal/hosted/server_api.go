package hosted

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
	"golang.org/x/oauth2"
)

func (s *Server) workspaceFromRequest(r *http.Request) string {
	if ws := strings.TrimSpace(r.Header.Get("X-Workspace-ID")); ws != "" {
		return internal.NormalizeWorkspaceID(ws)
	}
	return s.WorkspaceID
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	warmup := workspaceWarmupStatus(s.Store.DB, ws)
	rows, err := query(s.Store.DB, `
		SELECT a.id, a.workspace_id, a.email, a.daily_limit, a.status, a.provider, a.last_send_at
		FROM accounts a
		WHERE a.workspace_id = ?
		ORDER BY a.email`, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	type acct struct {
		ID                 int64   `json:"id"`
		WorkspaceID        string  `json:"workspace_id"`
		Email              string  `json:"email"`
		DailyLimit         int     `json:"daily_limit"`
		Status             string  `json:"status"`
		Provider           string  `json:"provider"`
		LastSendAt         *string `json:"last_send_at,omitempty"`
		SentToday          int     `json:"sent_today"`
		OAuthHealth        string  `json:"oauth_health"`
		WarmupStatus       string  `json:"warmup_status"`
		ReplyMode          string  `json:"reply_mode"`
		DomainVerification string  `json:"domain_verification"`
	}
	var list []acct
	for rows.Next() {
		var a acct
		var last sql.NullTime
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Email, &a.DailyLimit, &a.Status, &a.Provider, &last); err != nil {
			rows.Close()
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if last.Valid {
			v := last.Time.UTC().Format(time.RFC3339)
			a.LastSendAt = &v
		}
		a.WarmupStatus = warmup
		a.ReplyMode, a.DomainVerification = mailboxSurface(a.Provider)
		list = append(list, a)
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", scanErr.Error())
		return
	}
	cutoff := time.Now().UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	for i := range list {
		_ = queryRow(s.Store.DB, `
			SELECT COUNT(*) FROM events
			WHERE account_id = ? AND type = 'sent' AND timestamp >= ?`, list[i].ID, cutoff).Scan(&list[i].SentToday)
		oauth, _ := GetHostedKV(s.Store.DB, "account_oauth:"+strings.ToLower(list[i].Email))
		if oauth == "" {
			oauth = "ok"
		}
		list[i].OAuthHealth = oauth
	}
	if list == nil {
		list = []acct{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"accounts": list, "workspace_id": ws}})
}

func (s *Server) handleAccountStatus(w http.ResponseWriter, r *http.Request) {
	email, err := s.accountEmail(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	acct, err := engine.GetAccountByEmail(s.Store.DB, email)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	oauth, _ := GetHostedKV(s.Store.DB, "account_oauth:"+strings.ToLower(email))
	if oauth == "" {
		oauth = "ok"
	}
	replyMode, domainVer := mailboxSurface(acct.Provider)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"id": acct.ID, "email": acct.Email, "status": acct.Status,
		"provider": acct.Provider, "daily_limit": acct.DailyLimit,
		"oauth_health":  oauth,
		"warmup_status": workspaceWarmupStatus(s.Store.DB, s.workspaceFromRequest(r)),
		"reply_mode":    replyMode, "domain_verification": domainVer,
	}})
}

// mailboxSurface is the Accounts-page health view for a send provider.
// Resend is send-only (bounce webhook). Cloudflare Email uses Email Routing for replies.
// Warmup is never a send path.
func mailboxSurface(provider string) (replyMode, domainVerification string) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "resend", "ses", "mailgun", "postmark":
		return "send_only", "dns_at_provider"
	case "cf_email", "cloudflare":
		return "email_routing", "dns_at_cloudflare"
	case "smtp_imap", "smtp":
		return "imap", "smtp"
	default:
		return "oauth", "oauth"
	}
}

func workspaceWarmupStatus(db *sql.DB, ws string) string {
	var status, metadata string
	err := queryRow(db, `
		SELECT status, COALESCE(metadata, '')
		FROM integration_credentials
		WHERE workspace_id = ? AND provider = ?
		ORDER BY id DESC LIMIT 1`, ws, "warmup").Scan(&status, &metadata)
	if err != nil {
		return "unset"
	}
	if meta := jsonStringField(metadata, "status"); meta != "" {
		status = meta
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "ok", "healthy", "configured":
		return "healthy"
	case "error", "failed", "unhealthy":
		return "error"
	default:
		return "unknown"
	}
}

func jsonStringField(raw, key string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.OAuth == nil {
		if s.UseMockGmail {
			// Dev shortcut: connect a mock account without Google
			email := r.URL.Query().Get("email")
			if email == "" {
				email = "mock-sender@example.com"
			}
			res, err := internal.AddAccountInWorkspace(s.Store.DB, s.workspaceFromRequest(r), email, 30, "hosted-mock")
			if err != nil {
				writeErr(w, http.StatusBadRequest, "account_error", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
				"status": "connected", "account_id": res.ID, "email": res.Email, "mock": true,
				"next_actions": []string{"create_campaign"},
			}})
			return
		}
		writeErr(w, http.StatusBadRequest, "oauth_not_configured", "GOOGLE_CLIENT_ID/SECRET not set")
		return
	}
	state := randomState()
	ws := s.workspaceFromRequest(r)
	_, err := exec(s.Store.DB, `INSERT INTO oauth_states (state, workspace_id) VALUES (?, ?)`, state, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	url := s.OAuth.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"authorize_url": url, "state": state}})
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.OAuth == nil {
		writeErr(w, http.StatusBadRequest, "oauth_not_configured", "oauth not configured")
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeErr(w, http.StatusBadRequest, "invalid_callback", "missing state or code")
		return
	}
	var ws string
	err := queryRow(s.Store.DB, `SELECT workspace_id FROM oauth_states WHERE state = ?`, state).Scan(&ws)
	if err == sql.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "invalid_state", "unknown or expired oauth state")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	_, _ = exec(s.Store.DB, `DELETE FROM oauth_states WHERE state = ?`, state)

	tok, err := s.OAuth.Exchange(r.Context(), code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "token_exchange_failed", err.Error())
		return
	}
	client := s.OAuth.Client(r.Context(), tok)
	email, googleID, err := FetchGoogleUserEmail(r.Context(), client)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "userinfo_failed", err.Error())
		return
	}
	res, err := internal.AddAccountInWorkspace(s.Store.DB, ws, email, 30, "hosted")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "account_error", err.Error())
		return
	}
	if s.Creds == nil {
		writeErr(w, http.StatusInternalServerError, "vault_missing", "credential encryption not configured")
		return
	}
	cred := &GoogleCredential{
		WorkspaceID:     ws,
		AccountID:       res.ID,
		GoogleAccountID: googleID,
		RefreshToken:    tok.RefreshToken,
		AccessToken:     tok.AccessToken,
		TokenExpiry:     tok.Expiry,
		Scopes:          strings.Join(GoogleOAuthScopes, " "),
	}
	if err := s.Creds.PutGoogleCredential(r.Context(), cred); err != nil {
		writeErr(w, http.StatusInternalServerError, "vault_error", err.Error())
		return
	}
	if s.GWS != nil && s.GWS.API != nil {
		s.GWS.API.RegisterAccount(email, res.ID)
	}
	_ = SetHostedKV(s.Store.DB, "account_oauth:"+strings.ToLower(email), "ok")
	http.Redirect(w, r, "/accounts?connected=1", http.StatusFound)
}

func (s *Server) handlePauseAccount(w http.ResponseWriter, r *http.Request) {
	email, err := s.accountEmail(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	res, err := engine.PauseAccount(s.Store.DB, email)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pause_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) handleResumeAccount(w http.ResponseWriter, r *http.Request) {
	email, err := s.accountEmail(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	res, err := engine.ResumeAccount(s.Store.DB, email)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "resume_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	email, err := s.accountEmail(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	acct, _ := engine.GetAccountByEmail(s.Store.DB, email)
	res, err := engine.RemoveAccount(s.Store.DB, email)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "remove_failed", err.Error())
		return
	}
	if s.Creds != nil && acct.ID != 0 {
		_ = s.Creds.DeleteGoogleCredentialByAccountID(r.Context(), acct.ID)
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) accountEmail(r *http.Request) (string, error) {
	ws := s.workspaceFromRequest(r)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid account id")
	}
	var email, workspaceID string
	err = queryRow(s.Store.DB, `SELECT email, workspace_id FROM accounts WHERE id = ?`, id).Scan(&email, &workspaceID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("account not found")
	}
	if err != nil {
		return "", err
	}
	if workspaceID != ws {
		return "", fmt.Errorf("account not found")
	}
	return email, nil
}

func (s *Server) resolveCampaign(r *http.Request) (string, error) {
	ws := s.workspaceFromRequest(r)
	return internal.ResolveCampaignNameInWorkspace(s.Store.DB, ws, r.PathValue("id"))
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	rows, err := query(s.Store.DB, `
		SELECT c.id, c.name, c.status, c.created_at,
			(SELECT COUNT(*) FROM campaign_leads cl WHERE cl.campaign_id = c.id) AS leads,
			(SELECT COUNT(*) FROM events e WHERE e.campaign_id = c.id AND e.type = 'sent') AS sent,
			(SELECT COUNT(*) FROM events e WHERE e.campaign_id = c.id AND e.type = 'reply') AS replies,
			(SELECT COUNT(*) FROM events e WHERE e.campaign_id = c.id AND e.type = 'bounce') AS bounces,
			(SELECT COUNT(*) FROM events e WHERE e.campaign_id = c.id AND e.type = 'unique_open') AS opens,
			(SELECT MIN(ss.send_at) FROM scheduled_sends ss WHERE ss.campaign_id = c.id AND ss.status = 'pending') AS next_send
		FROM campaigns c
		WHERE c.workspace_id = ?
		ORDER BY c.created_at DESC`, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	type row struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		Leads       int     `json:"leads"`
		Sent        int     `json:"sent"`
		Replies     int     `json:"replies"`
		Bounces     int     `json:"bounces"`
		ApproxOpens int     `json:"approx_opens"`
		ReplyRate   float64 `json:"reply_rate"`
		NextSend    *string `json:"next_send,omitempty"`
		CreatedAt   string  `json:"created_at"`
	}
	var list []row
	for rows.Next() {
		var item row
		var created time.Time
		var next sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &created, &item.Leads, &item.Sent, &item.Replies, &item.Bounces, &item.ApproxOpens, &next); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		if item.Sent > 0 {
			item.ReplyRate = float64(item.Replies) / float64(item.Sent) * 100
		}
		if next.Valid {
			v := next.Time.UTC().Format(time.RFC3339)
			item.NextSend = &v
		}
		list = append(list, item)
	}
	if list == nil {
		list = []row{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"campaigns": list, "workspace_id": ws}})
}

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Name            string   `json:"name"`
		SequenceYAML    string   `json:"sequence_yaml"`
		LeadsCSV        string   `json:"leads_csv"`
		Accounts        []string `json:"accounts"`
		SendWindowStart string   `json:"send_window_start"`
		SendWindowEnd   string   `json:"send_window_end"`
		SendDays        string   `json:"send_days"`
		Timezone        string   `json:"timezone"`
		OpenTracking    bool     `json:"open_tracking"`
		DraftOnly       bool     `json:"draft_only"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	var skipped int
	if req.LeadsCSV != "" {
		filtered, n, ferr := s.filterSuppressedCSV(ws, req.LeadsCSV)
		if ferr != nil {
			writeErr(w, http.StatusBadRequest, "create_failed", ferr.Error())
			return
		}
		req.LeadsCSV = filtered
		skipped = n
	}
	if req.DraftOnly || (req.SequenceYAML == "" && req.LeadsCSV == "") {
		res, err := engine.CreateDraftCampaign(s.Store.DB, engine.CreateDraftCampaignOpts{
			WorkspaceID: ws, Name: req.Name, AccountEmails: req.Accounts,
			SendWindowStart: req.SendWindowStart, SendWindowEnd: req.SendWindowEnd,
			SendDays: req.SendDays, Timezone: req.Timezone,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create_failed", err.Error())
			return
		}
		if req.OpenTracking {
			_ = SetHostedKV(s.Store.DB, fmt.Sprintf("campaign_open_tracking:%d", res.ID), "1")
		}
		warnings := append([]string{}, res.Warnings...)
		if skipped > 0 {
			warnings = append(warnings, fmt.Sprintf("%d suppressed leads skipped", skipped))
		}
		writeJSON(w, http.StatusCreated, envelope{Data: map[string]any{
			"campaign_id": res.ID, "status": "draft", "name": res.Name,
			"lead_count": 0, "warnings": warnings,
			"next_actions": []string{"add_leads", "preview_campaign", "activate_campaign"},
		}, Warnings: warnings})
		return
	}
	res, err := engine.CreateCampaign(s.Store.DB, engine.CreateCampaignOpts{
		WorkspaceID: ws, Name: req.Name, SequenceInline: req.SequenceYAML, LeadsInline: req.LeadsCSV,
		AccountEmails: req.Accounts, SendWindowStart: req.SendWindowStart, SendWindowEnd: req.SendWindowEnd,
		SendDays: req.SendDays, Timezone: req.Timezone,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}
	if req.OpenTracking {
		_ = SetHostedKV(s.Store.DB, fmt.Sprintf("campaign_open_tracking:%d", res.ID), "1")
	}
	warnings := append([]string{}, res.Warnings...)
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("%d suppressed leads skipped", skipped))
	}
	writeJSON(w, http.StatusCreated, envelope{Data: map[string]any{
		"campaign_id":        res.ID,
		"status":             "draft",
		"name":               res.Name,
		"lead_count":         res.Leads,
		"scheduled_messages": res.ScheduledSends,
		"warnings":           warnings,
		"next_actions":       []string{"preview_campaign", "activate_campaign"},
	}, Warnings: warnings})
}

func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	info, err := internal.GetCampaignStatus(s.Store.DB, name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "status_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: info})
}

func (s *Server) handleActivateCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = json.Unmarshal(body, &req)
	if !req.Confirm {
		writeErr(w, http.StatusBadRequest, "activation_not_confirmed",
			"Set confirm=true only after explicit approval. Creating a campaign does not send mail.")
		return
	}
	if err := engine.CampaignStateTransition(s.Store.DB, name, "activate", "draft", "active"); err != nil {
		writeErr(w, http.StatusBadRequest, "activate_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"name": name, "status": "active",
		"message":      "Campaign activated. Cron/tick will send due messages. This action is consequential.",
		"next_actions": []string{"get_campaign_stats", "list_replies"},
	}})
}

func (s *Server) handlePauseCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if err := engine.CampaignStateTransition(s.Store.DB, name, "pause", "active", "paused"); err != nil {
		writeErr(w, http.StatusBadRequest, "pause_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"name": name, "status": "paused"}})
}

func (s *Server) handleResumeCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if err := engine.CampaignStateTransition(s.Store.DB, name, "resume", "paused", "active"); err != nil {
		writeErr(w, http.StatusBadRequest, "resume_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"name": name, "status": "active"}})
}

func (s *Server) handlePreviewCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	id, status, preview, err := internal.GetCampaignPreview(s.Store.DB, name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "preview_failed", err.Error())
		return
	}
	lead := r.URL.Query().Get("lead")
	var rendered any
	if r.URL.Query().Get("render") == "1" {
		rendered, err = internal.GetCampaignRenderedPreview(s.Store.DB, name, lead)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "render_failed", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": id, "status": status, "schedule": preview, "rendered": rendered,
		"next_actions": []string{"activate_campaign"},
	}})
}

func (s *Server) handleCampaignStats(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	var id int64
	_ = queryRow(s.Store.DB, `SELECT id FROM campaigns WHERE name = ?`, name).Scan(&id)
	steps, _ := internal.GetCampaignStepStats(s.Store.DB, id)
	variants, _ := internal.GetCampaignVariantStats(s.Store.DB, id)
	leads, _ := internal.GetCampaignLeadStats(s.Store.DB, id)
	var sent, replies, bounces, opens int
	_ = queryRow(s.Store.DB, `
		SELECT
			COALESCE(SUM(CASE WHEN type = 'sent' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'reply' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'bounce' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'unique_open' THEN 1 ELSE 0 END), 0)
		FROM events WHERE campaign_id = ?`, id).Scan(&sent, &replies, &bounces, &opens)
	replyRate := 0.0
	if sent > 0 {
		replyRate = float64(replies) / float64(sent) * 100
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign": name, "sent": sent, "replies": replies, "bounces": bounces,
		"approx_opens": opens, "reply_rate": replyRate,
		"steps": steps, "variants": variants, "leads": leads,
		"note": "Approx. opens are unique pixel loads; privacy proxies can skew this metric.",
	}})
}

func (s *Server) handleAddLeads(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		CSV     string `json:"csv"`
		DryRun  bool   `json:"dry_run"`
		Confirm bool   `json:"confirm"`
	}
	_ = json.Unmarshal(body, &req)
	if req.CSV == "" {
		req.CSV = string(body)
	}
	ws := s.workspaceFromRequest(r)
	filtered, skipped, ferr := s.filterSuppressedCSV(ws, req.CSV)
	if ferr != nil && !req.DryRun {
		writeErr(w, http.StatusBadRequest, "add_leads_failed", ferr.Error())
		return
	}
	if ferr == nil {
		req.CSV = filtered
	}
	if req.DryRun {
		records, _, err := internal.ParseLeadsCSVFromReader(strings.NewReader(req.CSV))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "parse_failed", err.Error())
			return
		}
		n := len(records)
		if n > 5 {
			n = 5
		}
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
			"dry_run": true, "count": len(records), "sample": records[:n],
		}})
		return
	}
	var status string
	_ = queryRow(s.Store.DB, `SELECT status FROM campaigns WHERE name = ?`, name).Scan(&status)
	if status == "active" && !req.Confirm {
		writeErr(w, http.StatusBadRequest, "confirm_required", "importing into an active campaign requires confirm=true")
		return
	}
	res, err := internal.AddLeadsToCampaign(s.Store.DB, name, "", req.CSV)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "add_leads_failed", err.Error())
		return
	}
	out := map[string]any{
		"result": res, "next_actions": []string{"preview_campaign", "activate_campaign"},
	}
	var warnings []string
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("%d suppressed leads skipped", skipped))
		out["suppressed_skipped"] = skipped
	}
	writeJSON(w, http.StatusOK, envelope{Data: out, Warnings: warnings})
}

func (s *Server) handleRemoveLead(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	leadID := r.PathValue("leadId")
	var email string
	if id, err := strconv.ParseInt(leadID, 10, 64); err == nil {
		_ = queryRow(s.Store.DB, `SELECT email FROM leads WHERE id = ?`, id).Scan(&email)
	} else {
		email = leadID
	}
	if _, err := internal.RemoveLeadFromCampaign(s.Store.DB, name, email); err != nil {
		writeErr(w, http.StatusBadRequest, "remove_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"removed": email}})
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	rows, err := query(s.Store.DB, `
		SELECT em.campaign_id, em.lead_id, l.email, l.company, c.name, a.email,
			em.subject, em.snippet, em.occurred_at, em.type,
			COALESCE(rc.classification, '')
		FROM email_messages em
		JOIN campaigns c ON c.id = em.campaign_id
		JOIN leads l ON l.id = em.lead_id
		JOIN accounts a ON a.id = em.account_id
		LEFT JOIN reply_classifications rc ON rc.campaign_id = em.campaign_id AND rc.lead_id = em.lead_id
		WHERE c.workspace_id = ? AND em.direction = 'inbound'
		ORDER BY em.occurred_at DESC
		LIMIT 100`, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		CampaignID     int64  `json:"campaign_id"`
		LeadID         int64  `json:"lead_id"`
		Contact        string `json:"contact"`
		Company        string `json:"company"`
		Campaign       string `json:"campaign"`
		Sender         string `json:"sender"`
		Subject        string `json:"subject"`
		LatestMessage  string `json:"latest_message"`
		Classification string `json:"classification"`
		Type           string `json:"type"`
		Timestamp      string `json:"timestamp"`
	}
	var list []item
	for rows.Next() {
		var it item
		var occurred time.Time
		if err := rows.Scan(&it.CampaignID, &it.LeadID, &it.Contact, &it.Company, &it.Campaign, &it.Sender,
			&it.Subject, &it.LatestMessage, &occurred, &it.Type, &it.Classification); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		it.Timestamp = occurred.UTC().Format(time.RFC3339)
		list = append(list, it)
	}
	if list == nil {
		list = []item{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"threads": list}})
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	cid, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	lid, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var campaignWS string
	err := queryRow(s.Store.DB, `SELECT workspace_id FROM campaigns WHERE id = ?`, cid).Scan(&campaignWS)
	if err != nil || campaignWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "thread not found")
		return
	}
	msgs, err := engine.ListEmailThreadMessages(s.Store.DB, engine.ListEmailThreadMessagesOpts{
		CampaignID: cid, LeadID: lid,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "thread_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"messages": msgs}})
}

func (s *Server) handleThreadReply(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	cid, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	lid, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var campaignWS string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM campaigns WHERE id = ?`, cid).Scan(&campaignWS); err != nil || campaignWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "thread not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Body      string `json:"body"`
		ConfirmTo string `json:"confirm_to"`
		Send      bool   `json:"send"`
		Confirm   bool   `json:"confirm"`
	}
	_ = json.Unmarshal(body, &req)
	if req.Confirm {
		req.Send = true
	}
	var globalStatus string
	_ = queryRow(s.Store.DB, `SELECT global_status FROM leads WHERE id = ?`, lid).Scan(&globalStatus)
	if req.Send && (globalStatus == "blacklisted" || globalStatus == "bounced") {
		writeErr(w, http.StatusBadRequest, "suppressed", "lead is "+globalStatus+"; reply not sent")
		return
	}
	if !req.Send {
		preview, err := engine.PreviewInboxReply(engine.PreviewInboxReplyConfig{
			DB: s.Store.DB, CampaignID: cid, LeadID: lid, Body: req.Body, WorkspaceID: ws,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "preview_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"preview": preview, "next_actions": []string{"send with confirm_to"}}})
		return
	}
	if req.ConfirmTo == "" {
		writeErr(w, http.StatusBadRequest, "confirm_required", "confirm_to is required when send=true")
		return
	}
	preview, err := engine.PreviewInboxReply(engine.PreviewInboxReplyConfig{
		DB: s.Store.DB, CampaignID: cid, LeadID: lid, Body: req.Body, WorkspaceID: ws,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "preview_failed", err.Error())
		return
	}
	if !strings.EqualFold(preview.ToEmail, req.ConfirmTo) {
		writeErr(w, http.StatusBadRequest, "confirm_mismatch", "confirm_to does not match recipient")
		return
	}
	res, err := engine.SendInboxReply(engine.SendInboxReplyConfig{
		DB: s.Store.DB, GWS: s.GWS, CampaignID: cid, LeadID: lid, Body: req.Body, WorkspaceID: ws,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "reply_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) handleClassify(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	cid, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	lid, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var campaignWS string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM campaigns WHERE id = ?`, cid).Scan(&campaignWS); err != nil || campaignWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "thread not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Classification string  `json:"classification"`
		Confidence     float64 `json:"confidence"`
		Reason         string  `json:"reason"`
	}
	_ = json.Unmarshal(body, &req)
	_, err := exec(s.Store.DB, `
		INSERT INTO reply_classifications (workspace_id, campaign_id, lead_id, classification, confidence, reason)
		VALUES (?, ?, ?, ?, ?, ?)`, ws, cid, lid, req.Classification, req.Confidence, req.Reason)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "classify_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"status": "classified", "classification": req.Classification}})
}

func (s *Server) handleValidateLeads(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		CSV string `json:"csv"`
	}
	_ = json.Unmarshal(body, &req)
	if req.CSV == "" {
		req.CSV = string(body)
	}
	lines := strings.Split(req.CSV, "\n")
	total, valid, invalid, dup := 0, 0, 0, 0
	seen := map[string]bool{}
	var warnings []string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || (i == 0 && strings.Contains(strings.ToLower(line), "email")) {
			continue
		}
		total++
		cols := strings.Split(line, ",")
		email := strings.TrimSpace(strings.ToLower(cols[0]))
		if email == "" || !strings.Contains(email, "@") {
			invalid++
			continue
		}
		if seen[email] {
			dup++
			continue
		}
		seen[email] = true
		valid++
	}
	if dup > 0 {
		warnings = append(warnings, fmt.Sprintf("%d duplicate emails", dup))
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"total": total, "valid": valid, "invalid": invalid, "duplicate": dup, "warnings": warnings,
	}, Warnings: warnings})
}

func (s *Server) handleListLeads(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	domain := r.URL.Query().Get("domain")
	status := r.URL.Query().Get("status")
	limit := 100
	leads, err := searchLeads(s.Store.DB, q, domain, status, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if leads == nil {
		leads = []internal.LeadListRow{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"leads": leads}})
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"workspace_id": s.workspaceFromRequest(r)}})
}

func (s *Server) handleBlacklistLead(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("id")
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		_ = queryRow(s.Store.DB, `SELECT email FROM leads WHERE id = ?`, id).Scan(&target)
	}
	res, err := internal.BlacklistLead(s.Store.DB, target)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "blacklist_failed", err.Error())
		return
	}
	kind := "email"
	value := strings.ToLower(strings.TrimSpace(target))
	if !strings.Contains(value, "@") {
		kind = "domain"
	}
	_, _ = s.upsertSuppression(s.workspaceFromRequest(r), kind, value)
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) handlePauseLead(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("id")
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		_ = queryRow(s.Store.DB, `SELECT email FROM leads WHERE id = ?`, id).Scan(&target)
	}
	res, err := internal.PauseLead(s.Store.DB, target)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pause_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	rangeKey := r.URL.Query().Get("range")
	since := time.Time{}
	now := time.Now().UTC()
	switch rangeKey {
	case "today":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
	}
	where := `c.workspace_id = ?`
	args := []any{ws}
	if !since.IsZero() {
		where += ` AND e.timestamp >= ?`
		args = append(args, since.Format(time.RFC3339))
	}
	q := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN e.type = 'sent' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.type = 'reply' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.type = 'bounce' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.type = 'unique_open' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.type = 'unsubscribe' THEN 1 ELSE 0 END), 0)
		FROM events e
		JOIN campaigns c ON c.id = e.campaign_id
		WHERE %s`, where)
	var sent, replies, bounces, opens, unsubs int
	if err := queryRow(s.Store.DB, q, args...).Scan(&sent, &replies, &bounces, &opens, &unsubs); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var positive int
	_ = queryRow(s.Store.DB, `
		SELECT COUNT(*) FROM reply_classifications rc
		JOIN campaigns c ON c.id = rc.campaign_id
		WHERE c.workspace_id = ? AND rc.classification = 'positive'`, ws).Scan(&positive)
	replyRate := 0.0
	if sent > 0 {
		replyRate = float64(replies) / float64(sent) * 100
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"sent": sent, "replies": replies, "reply_rate": replyRate,
		"positive_replies": positive, "bounces": bounces,
		"approx_opens": opens, "unsubscribes": unsubs,
		"range": rangeKey,
		"note":  "Approx. opens may be affected by image proxies and privacy protections.",
	}})
}

func (s *Server) handleSetOpenTracking(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	var id int64
	_ = queryRow(s.Store.DB, `SELECT id FROM campaigns WHERE name = ?`, name).Scan(&id)
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(body, &req)
	val := "0"
	if req.Enabled {
		val = "1"
	}
	_ = SetHostedKV(s.Store.DB, fmt.Sprintf("campaign_open_tracking:%d", id), val)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"campaign_id": id, "open_tracking": req.Enabled}})
}

func (s *Server) handleTrackOpen(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Token     string `json:"token"`
		UserAgent string `json:"user_agent"`
		Country   string `json:"country"`
	}
	_ = json.Unmarshal(body, &req)
	t, err := GetTrackingToken(s.Store.DB, req.Token)
	if err != nil || t == nil || t.Kind != "open" {
		writeErr(w, http.StatusNotFound, "not_found", "token not found")
		return
	}
	_ = RecordOpenEvent(s.Store.DB, t, req.UserAgent, req.Country)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"recorded": true}})
}

func (s *Server) handleTrackClick(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Token     string `json:"token"`
		UserAgent string `json:"user_agent"`
		Country   string `json:"country"`
	}
	_ = json.Unmarshal(body, &req)
	t, err := GetTrackingToken(s.Store.DB, req.Token)
	if err != nil || t == nil || t.Kind != "click" {
		writeErr(w, http.StatusNotFound, "not_found", "token not found")
		return
	}
	_ = RecordClickEvent(s.Store.DB, t, req.UserAgent, req.Country)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"destination_url": t.DestinationURL}})
}
