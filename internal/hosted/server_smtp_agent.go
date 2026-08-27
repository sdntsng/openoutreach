package hosted

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

func (s *Server) handleAddSMTPAccount(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["smtp_imap"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_SMTP_IMAP is disabled")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Email           string `json:"email"`
		DailyLimit      int    `json:"daily_limit"`
		SMTPHost        string `json:"smtp_host"`
		SMTPPort        int    `json:"smtp_port"`
		SMTPUsername    string `json:"smtp_username"`
		SMTPPassword    string `json:"smtp_password"`
		SMTPPasswordRef string `json:"smtp_password_ref"`
		SMTPTLSMode     string `json:"smtp_tls_mode"`
		IMAPHost        string `json:"imap_host"`
		IMAPPort        int    `json:"imap_port"`
		IMAPUsername    string `json:"imap_username"`
		IMAPPassword    string `json:"imap_password"`
		IMAPPasswordRef string `json:"imap_password_ref"`
		IMAPTLSMode     string `json:"imap_tls_mode"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	ws := s.workspaceFromRequest(r)
	if req.DailyLimit <= 0 {
		req.DailyLimit = 50
	}
	smtpRef := strings.TrimSpace(req.SMTPPasswordRef)
	imapRef := strings.TrimSpace(req.IMAPPasswordRef)
	key := s.encKey()
	if smtpRef == "" && strings.TrimSpace(req.SMTPPassword) != "" {
		if key == nil {
			writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY required to store SMTP password")
			return
		}
		cred, err := PutIntegrationCredential(s.Store.DB, key, ws, IntegrationCredentialInput{
			Provider: "smtp_password",
			Name:     "smtp:" + strings.ToLower(strings.TrimSpace(req.Email)),
			Secret:   req.SMTPPassword,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "vault_failed", err.Error())
			return
		}
		smtpRef = fmt.Sprintf("secret:%d", cred.ID)
	}
	if imapRef == "" && strings.TrimSpace(req.IMAPPassword) != "" {
		if key == nil {
			writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY required to store IMAP password")
			return
		}
		cred, err := PutIntegrationCredential(s.Store.DB, key, ws, IntegrationCredentialInput{
			Provider: "smtp_password",
			Name:     "imap:" + strings.ToLower(strings.TrimSpace(req.Email)),
			Secret:   req.IMAPPassword,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "vault_failed", err.Error())
			return
		}
		imapRef = fmt.Sprintf("secret:%d", cred.ID)
	}
	if imapRef == "" {
		imapRef = smtpRef
	}
	res, err := engine.AddSMTPIMAPAccount(s.Store.DB, engine.AddSMTPIMAPAccountOpts{
		WorkspaceID:     ws,
		Email:           req.Email,
		DailyLimit:      req.DailyLimit,
		SMTPHost:        req.SMTPHost,
		SMTPPort:        req.SMTPPort,
		SMTPUsername:    req.SMTPUsername,
		SMTPPasswordRef: smtpRef,
		SMTPTLSMode:     req.SMTPTLSMode,
		IMAPHost:        req.IMAPHost,
		IMAPPort:        req.IMAPPort,
		IMAPUsername:    req.IMAPUsername,
		IMAPPasswordRef: imapRef,
		IMAPTLSMode:     req.IMAPTLSMode,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "add_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res, Warnings: []string{"passwords stored as secret refs; never returned"}})
}

func (s *Server) secretResolver() internal.SecretResolver {
	key := s.encKey()
	if key == nil {
		return internal.EnvSecretResolver{}
	}
	return HostedSecretResolver{DB: s.Store.DB, Key: key, WorkspaceID: s.WorkspaceID}
}

func (s *Server) handlePreflightCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "invalid campaign id")
		return
	}
	db := s.Store.DB
	var name, status, seqContent string
	var leadCount, pending, accounts int
	err = queryRow(db, `SELECT name, status, sequence_content FROM campaigns WHERE id = ?`, id).Scan(&name, &status, &seqContent)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	}
	_ = queryRow(db, `SELECT COUNT(*) FROM campaign_leads WHERE campaign_id = ?`, id).Scan(&leadCount)
	_ = queryRow(db, `SELECT COUNT(*) FROM scheduled_sends WHERE campaign_id = ? AND status = 'pending'`, id).Scan(&pending)
	_ = queryRow(db, `SELECT COUNT(*) FROM campaign_accounts ca JOIN accounts a ON a.id = ca.account_id WHERE ca.campaign_id = ? AND a.status = 'active'`, id).Scan(&accounts)

	var warnings []string
	ready := true
	if status != "draft" && status != "paused" {
		warnings = append(warnings, "campaign status is "+status)
	}
	if strings.TrimSpace(seqContent) == "" {
		warnings = append(warnings, "sequence_content is empty")
		ready = false
	}
	if leadCount == 0 {
		warnings = append(warnings, "no leads on campaign")
		ready = false
	}
	if accounts == 0 {
		warnings = append(warnings, "no active sending accounts assigned")
		ready = false
	}
	// lightweight lead quality
	rows, qerr := query(db, `
		SELECT l.email FROM leads l
		JOIN campaign_leads cl ON cl.lead_id = l.id
		WHERE cl.campaign_id = ? LIMIT 500`, id)
	invalid := 0
	if qerr == nil {
		defer rows.Close()
		for rows.Next() {
			var email string
			_ = rows.Scan(&email)
			email = strings.ToLower(strings.TrimSpace(email))
			if !strings.Contains(email, "@") || strings.HasPrefix(email, "info@") || strings.HasPrefix(email, "support@") {
				invalid++
			}
		}
	}
	if invalid > 0 {
		warnings = append(warnings, fmt.Sprintf("%d leads look invalid or role-based", invalid))
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": id, "name": name, "status": status,
		"ready": ready, "lead_count": leadCount, "pending_sends": pending,
		"active_accounts": accounts, "warnings": warnings,
	}, Warnings: warnings})
}

func (s *Server) handleDraftSequence(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ICP       string `json:"icp"`
		Offer     string `json:"offer"`
		Tone      string `json:"tone"`
		StepCount int    `json:"step_count"`
		FromName  string `json:"from_name"`
	}
	_ = json.Unmarshal(body, &req)
	if req.StepCount <= 0 {
		req.StepCount = 3
	}
	if req.StepCount > 6 {
		req.StepCount = 6
	}
	if req.FromName == "" {
		req.FromName = "You"
	}
	if req.Tone == "" {
		req.Tone = "direct"
	}
	icp := strings.TrimSpace(req.ICP)
	offer := strings.TrimSpace(req.Offer)
	if icp == "" {
		icp = "your ICP"
	}
	if offer == "" {
		offer = "our product"
	}
	var b strings.Builder
	b.WriteString("name: drafted\n")
	b.WriteString("defaults:\n")
	b.WriteString("  from_name: " + req.FromName + "\n")
	b.WriteString("steps:\n")
	for i := 1; i <= req.StepCount; i++ {
		delay := 0
		if i > 1 {
			delay = 3
		}
		subj := fmt.Sprintf("Quick question for {{first_name}}")
		bodyText := fmt.Sprintf("Hi {{first_name}},\n\nReaching out because you work with %s. We help with %s (%s tone).\n\nWorth a quick look?\n", icp, offer, req.Tone)
		if i > 1 {
			subj = fmt.Sprintf("Re: {{first_name}} — follow-up %d", i-1)
			bodyText = fmt.Sprintf("Hi {{first_name}},\n\nJust bumping this in case it got buried. Still relevant for %s?\n", icp)
		}
		b.WriteString(fmt.Sprintf("  - step: %d\n    delay: %d\n    subject: %q\n    body: |\n", i, delay, subj))
		for _, line := range strings.Split(bodyText, "\n") {
			b.WriteString("      " + line + "\n")
		}
	}
	yamlOut := b.String()
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"sequence_yaml": yamlOut,
		"draft_only":    true,
		"next_actions":  []string{"review YAML", "attach to draft campaign", "preview", "human activate"},
	}, Warnings: []string{"draft only — never auto-activates"}})
}

func (s *Server) handleSuggestReply(w http.ResponseWriter, r *http.Request) {
	campaignID, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	leadID, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var classification, reason string
	_ = queryRow(s.Store.DB, `
		SELECT classification, reason FROM reply_classifications
		WHERE campaign_id = ? AND lead_id = ?
		ORDER BY id DESC LIMIT 1`, campaignID, leadID).Scan(&classification, &reason)
	if classification == "" {
		classification = "unknown"
	}
	suggestion := "Thanks for the reply — happy to share more detail."
	switch strings.ToLower(classification) {
	case "positive", "interested":
		suggestion = "Appreciate the interest — what does your calendar look like next week for a 15-min call?"
	case "objection":
		suggestion = "Totally fair — curious what would make this worth revisiting later?"
	case "ooo", "out_of_office":
		suggestion = "" // do not suggest send
	case "unsubscribe", "not_interested":
		suggestion = ""
	case "bounce":
		suggestion = ""
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": campaignID, "lead_id": leadID,
		"classification": classification, "reason": reason,
		"suggested_body": suggestion,
		"send_allowed":   suggestion != "",
		"next_actions":   []string{"human edits", "POST reply with confirm:true to send"},
	}})
}
