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
	rev, err := s.buildCampaignReview(r, "")
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	var pending int
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM scheduled_sends WHERE campaign_id = ? AND status = 'pending'`, rev.CampaignID).Scan(&pending)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": rev.CampaignID, "name": rev.Name, "status": rev.Status,
		"ready": rev.Ready, "lead_count": rev.Enrolled, "pending_sends": pending,
		"active_accounts": len(rev.Accounts), "checklist": rev.Checklist,
		"next_send": rev.NextSend, "next_send_note": rev.NextSendNote,
		"warnings": rev.Warnings,
	}, Warnings: rev.Warnings})
}

func (s *Server) handleDraftSequence(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ICP        string `json:"icp"`
		Offer      string `json:"offer"`
		Tone       string `json:"tone"`
		StepCount  int    `json:"step_count"`
		FromName   string `json:"from_name"`
		CampaignID int64  `json:"campaign_id"`
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
	ws := s.workspaceFromRequest(r)
	pb := s.loadPlaybook(ws)
	usedPlaybook := false
	if icp == "" {
		if a := strings.TrimSpace(pb.Audience); a != "" {
			icp = a
			usedPlaybook = true
		}
	}
	if offer == "" {
		if o := strings.TrimSpace(pb.Offer); o != "" {
			offer = o
			usedPlaybook = true
		}
	}
	if icp == "" {
		icp = "your ICP"
	}
	if offer == "" {
		offer = "your offer"
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
	seq, err := internal.ParseSequenceFromBytes([]byte(yamlOut))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "invalid_draft", err.Error())
		return
	}
	samples := []map[string]string{
		{"first_name": "Ada", "last_name": "Lovelace", "company": "Acme", "email": "ada@acme.com", "domain": "acme.com"},
		{"first_name": "Alan", "last_name": "Turing", "company": "Bletchley", "email": "alan@bletchley.example", "domain": "bletchley.example"},
	}
	var preview []map[string]any
	for _, fields := range samples {
		steps := make([]map[string]string, 0, len(seq.Steps))
		for _, st := range seq.Steps {
			steps = append(steps, map[string]string{
				"step":    fmt.Sprintf("%d", st.Step),
				"subject": internal.RenderTemplate(st.Subject, fields),
				"body":    internal.RenderTemplate(st.Body, fields),
			})
		}
		preview = append(preview, map[string]any{"lead": fields, "steps": steps})
	}

	var warnings []string
	warnings = append(warnings, "draft only — never auto-activates")
	if req.CampaignID > 0 {
		var status string
		if err := queryRow(s.Store.DB, `SELECT status FROM campaigns WHERE id = ?`, req.CampaignID).Scan(&status); err != nil {
			writeErr(w, http.StatusBadRequest, "campaign_not_found", "campaign_id not found")
			return
		}
		if status != "draft" && status != "paused" {
			writeErr(w, http.StatusBadRequest, "not_draft", "refusing to overwrite sequence on "+status+" campaign")
			return
		}
		_, _ = exec(s.Store.DB, `UPDATE campaigns SET sequence_content = ? WHERE id = ?`, yamlOut, req.CampaignID)
		warnings = append(warnings, "sequence stored on draft campaign; preview then human activate")
	}

	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"sequence_yaml": yamlOut,
		"draft_only":    true,
		"valid":         true,
		"step_count":    len(seq.Steps),
		"preview":       preview,
		"used_playbook": usedPlaybook,
		"next_actions":  []string{"review the emails", "preview campaign", "human activate"},
	}, Warnings: warnings})
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
	source := "classification"
	usedPlaybook := false
	pb := s.loadPlaybook(s.workspaceFromRequest(r))
	switch strings.ToLower(classification) {
	case "positive", "interested", "hot":
		suggestion = "Appreciate the interest — what does your calendar look like next week for a 15-min call?"
		if pb.Company != "" || pb.Offer != "" {
			usedPlaybook = true
			source = "classification+playbook"
			who := pb.Company
			if who == "" {
				who = "us"
			}
			what := pb.Offer
			if what == "" {
				what = "what we sent"
			}
			suggestion = fmt.Sprintf("Appreciate the interest — happy to walk through how %s helps with %s. What does your calendar look like next week?", who, what)
		}
	case "objection":
		suggestion = "Totally fair — curious what would make this worth revisiting later?"
	case "ooo", "out_of_office":
		suggestion = ""
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
		"used_playbook":  usedPlaybook,
		"source":         source,
		"next_actions":   []string{"human edits", "POST reply with confirm:true to send"},
	}})
}
