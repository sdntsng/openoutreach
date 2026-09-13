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
)

type checklistItem struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Label string `json:"label"`
	Fix   string `json:"fix,omitempty"`
	Count int    `json:"count,omitempty"`
}

type recipientPreview struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	Company   string `json:"company,omitempty"`
	Title     string `json:"title,omitempty"`
	Source    string `json:"source,omitempty"`
}

type campaignReview struct {
	CampaignID     int64              `json:"campaign_id"`
	Name           string             `json:"name"`
	Status         string             `json:"status"`
	Sequence       SequenceView       `json:"sequence"`
	Rendered       []RenderedStep     `json:"rendered"`
	PreviewLead    recipientPreview   `json:"preview_lead"`
	Recipients     []recipientPreview `json:"recipients"`
	PreviewOptions []recipientPreview `json:"preview_options"`
	Exclusions     []string           `json:"exclusions"`
	Accounts       []string           `json:"accounts"`
	SendWindow     string             `json:"send_window"`
	Timezone       string             `json:"timezone"`
	SendDays       string             `json:"send_days"`
	NextSend       string             `json:"next_send,omitempty"`
	NextSendNote   string             `json:"next_send_note,omitempty"`
	Enrolled       int                `json:"enrolled"`
	PendingReview  int                `json:"pending_review"`
	Checklist      []checklistItem    `json:"checklist"`
	Ready          bool               `json:"ready"`
	Warnings       []string           `json:"warnings,omitempty"`
	NextActions    []string           `json:"next_actions"`
}

func (s *Server) handleSequencePreview(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		SequenceYAML string             `json:"sequence_yaml"`
		FromName     string             `json:"from_name"`
		Steps        []SequenceStepView `json:"steps"`
		Fields       map[string]string  `json:"fields"`
		LeadEmail    string             `json:"lead_email"`
		CampaignID   int64              `json:"campaign_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil && len(body) > 0 {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	yamlIn := strings.TrimSpace(req.SequenceYAML)
	if yamlIn == "" && len(req.Steps) > 0 {
		yamlIn = SequenceToYAML(sequenceFromView(SequenceView{
			FromName: req.FromName, Steps: req.Steps,
		}))
	}
	seq, yamlOut, err := parseSequenceYAML(yamlIn)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_sequence", err.Error())
		return
	}
	fields := req.Fields
	preview := recipientPreview{Email: "ada@acme.com", FirstName: "Ada", Company: "Acme"}
	if req.CampaignID > 0 && strings.TrimSpace(req.LeadEmail) != "" {
		if f, rec, ok := s.loadPreviewLead(s.workspaceFromRequest(r), req.CampaignID, req.LeadEmail); ok {
			fields = f
			preview = rec
		}
	}
	if fields == nil {
		fields = samplePreviewFields
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"sequence":     sequenceViewFrom(seq, yamlOut),
		"rendered":     renderSequence(seq, fields),
		"preview_lead": preview,
	}})
}

func (s *Server) handleCampaignReview(w http.ResponseWriter, r *http.Request) {
	rev, err := s.buildCampaignReview(r, r.URL.Query().Get("lead"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: rev, Warnings: rev.Warnings})
}

func (s *Server) buildCampaignReview(r *http.Request, leadEmail string) (*campaignReview, error) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		return nil, err
	}
	ws := s.workspaceFromRequest(r)
	var id int64
	var status, seqContent, windowStart, windowEnd, sendDays, tz string
	if err := queryRow(s.Store.DB, `
		SELECT id, status, COALESCE(sequence_content, ''), send_window_start, send_window_end, send_days, timezone
		FROM campaigns WHERE name = ?`, name).Scan(&id, &status, &seqContent, &windowStart, &windowEnd, &sendDays, &tz); err != nil {
		return nil, fmt.Errorf("campaign not found")
	}

	rev := &campaignReview{
		CampaignID: id, Name: name, Status: status,
		SendWindow: strings.TrimSpace(windowStart + "–" + windowEnd),
		Timezone:   tz, SendDays: sendDays,
		Recipients: []recipientPreview{}, PreviewOptions: []recipientPreview{},
		Exclusions: []string{}, Accounts: []string{},
		Checklist: []checklistItem{}, NextActions: []string{},
	}

	var seq *internal.Sequence
	if strings.TrimSpace(seqContent) != "" {
		seq, _, err = parseSequenceYAML(seqContent)
		if err != nil {
			rev.Warnings = append(rev.Warnings, "sequence YAML: "+err.Error())
		} else {
			rev.Sequence = sequenceViewFrom(seq, seqContent)
		}
	} else {
		rev.Sequence = SequenceView{Name: "outreach", FromName: "You", Steps: []SequenceStepView{}, YAML: ""}
	}

	accRows, err := query(s.Store.DB, `
		SELECT a.email FROM campaign_accounts ca
		JOIN accounts a ON a.id = ca.account_id
		WHERE ca.campaign_id = ? ORDER BY a.email`, id)
	if err == nil {
		defer accRows.Close()
		for accRows.Next() {
			var email string
			if accRows.Scan(&email) == nil {
				rev.Accounts = append(rev.Accounts, email)
			}
		}
	}

	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM campaign_leads WHERE campaign_id = ?`, id).Scan(&rev.Enrolled)
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM shortlist_candidates WHERE workspace_id = ? AND campaign_id = ? AND status = 'pending'`, ws, id).Scan(&rev.PendingReview)

	seenPreview := map[string]bool{}
	leadRows, err := query(s.Store.DB, `
		SELECT l.email, COALESCE(l.first_name,''), COALESCE(l.company,'')
		FROM campaign_leads cl JOIN leads l ON l.id = cl.lead_id
		WHERE cl.campaign_id = ? ORDER BY l.email LIMIT 40`, id)
	if err == nil {
		defer leadRows.Close()
		for leadRows.Next() {
			var rec recipientPreview
			if leadRows.Scan(&rec.Email, &rec.FirstName, &rec.Company) == nil {
				rec.Source = "enrolled"
				rev.Recipients = append(rev.Recipients, rec)
				seenPreview[strings.ToLower(rec.Email)] = true
				rev.PreviewOptions = append(rev.PreviewOptions, rec)
			}
		}
		leadRows.Close()
	}
	slRows, err := query(s.Store.DB, `
		SELECT email, first_name, company, title, source, status
		FROM shortlist_candidates
		WHERE workspace_id = ? AND campaign_id = ? AND status != 'excluded'
		ORDER BY CASE status WHEN 'approved' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END, id
		LIMIT 40`, ws, id)
	if err == nil {
		defer slRows.Close()
		for slRows.Next() {
			var rec recipientPreview
			var status string
			if slRows.Scan(&rec.Email, &rec.FirstName, &rec.Company, &rec.Title, &rec.Source, &status) == nil {
				if seenPreview[strings.ToLower(rec.Email)] {
					continue
				}
				if rec.Source == "" {
					rec.Source = status
				} else {
					rec.Source = status + " · " + rec.Source
				}
				seenPreview[strings.ToLower(rec.Email)] = true
				rev.PreviewOptions = append(rev.PreviewOptions, rec)
			}
		}
		slRows.Close()
	}

	supRows, err := query(s.Store.DB, `SELECT kind, value FROM suppressions WHERE workspace_id = ? ORDER BY kind, value LIMIT 50`, ws)
	if err == nil {
		defer supRows.Close()
		for supRows.Next() {
			var kind, value string
			if supRows.Scan(&kind, &value) == nil {
				rev.Exclusions = append(rev.Exclusions, kind+": "+value)
			}
		}
	}

	fields := samplePreviewFields
	rev.PreviewLead = recipientPreview{Email: "ada@acme.com", FirstName: "Ada", Company: "Acme", Source: "sample"}
	if f, rec, ok := s.loadPreviewLead(ws, id, leadEmail); ok {
		fields = f
		rev.PreviewLead = rec
	}
	if seq != nil {
		rev.Rendered = renderSequence(seq, fields)
		s.attachScheduledTimes(id, rev.PreviewLead.Email, rev.Rendered)
	}

	var next sql.NullString
	_ = queryRow(s.Store.DB, `SELECT MIN(send_at) FROM scheduled_sends WHERE campaign_id = ? AND status = 'pending'`, id).Scan(&next)
	if next.Valid && strings.TrimSpace(next.String) != "" {
		rev.NextSend = next.String
		rev.NextSendNote = "Next queued send: " + formatWhen(next.String, tz)
	} else if status == "active" {
		rev.NextSendNote = "No pending sends in the window."
	} else if status == "draft" {
		rev.NextSendNote = "Nothing sends until you activate."
	}

	rev.Checklist, rev.Ready, rev.Warnings = s.campaignChecklist(ws, id, name, status, seqContent, rev)
	if !rev.Ready {
		rev.NextActions = []string{"fix readiness items", "review recipients", "activate only with confirm"}
	} else if status == "draft" {
		rev.NextActions = []string{"activate_campaign"}
	}
	return rev, nil
}

func (s *Server) loadPreviewLead(ws string, campaignID int64, leadEmail string) (map[string]string, recipientPreview, bool) {
	leadEmail = strings.ToLower(strings.TrimSpace(leadEmail))
	type row struct {
		email, first, last, company, domain, title, source string
	}
	try := func(email string) (row, bool) {
		var r row
		err := queryRow(s.Store.DB, `
			SELECT l.email, COALESCE(l.first_name,''), COALESCE(l.last_name,''), COALESCE(l.company,''), COALESCE(l.domain,'')
			FROM campaign_leads cl JOIN leads l ON l.id = cl.lead_id
			WHERE cl.campaign_id = ? AND (? = '' OR LOWER(l.email) = ?)
			ORDER BY l.id LIMIT 1`, campaignID, email, email).Scan(&r.email, &r.first, &r.last, &r.company, &r.domain)
		if err == nil {
			r.source = "enrolled"
			return r, true
		}
		err = queryRow(s.Store.DB, `
			SELECT email, first_name, last_name, company, domain, title, source
			FROM shortlist_candidates
			WHERE workspace_id = ? AND campaign_id = ? AND (? = '' OR LOWER(email) = ?)
			AND status != 'excluded'
			ORDER BY CASE status WHEN 'approved' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END, id
			LIMIT 1`, ws, campaignID, email, email).Scan(&r.email, &r.first, &r.last, &r.company, &r.domain, &r.title, &r.source)
		if err == nil {
			return r, true
		}
		return r, false
	}
	r, ok := try(leadEmail)
	if !ok && leadEmail != "" {
		r, ok = try("")
	}
	if !ok {
		return nil, recipientPreview{}, false
	}
	fields := mergeLeadFields(r.email, r.first, r.last, r.company, r.domain, map[string]string{"title": r.title})
	return fields, recipientPreview{
		Email: r.email, FirstName: r.first, Company: r.company, Title: r.title, Source: r.source,
	}, true
}

func (s *Server) attachScheduledTimes(campaignID int64, leadEmail string, steps []RenderedStep) {
	if len(steps) == 0 || strings.TrimSpace(leadEmail) == "" {
		return
	}
	rows, err := query(s.Store.DB, `
		SELECT ss.step_number, CAST(ss.send_at AS TEXT)
		FROM scheduled_sends ss JOIN leads l ON l.id = ss.lead_id
		WHERE ss.campaign_id = ? AND LOWER(l.email) = ?
		ORDER BY ss.step_number`, campaignID, strings.ToLower(leadEmail))
	if err != nil {
		return
	}
	defer rows.Close()
	times := map[int]string{}
	for rows.Next() {
		var step int
		var at string
		if rows.Scan(&step, &at) == nil {
			times[step] = at
		}
	}
	for i := range steps {
		if at, ok := times[steps[i].Step]; ok {
			steps[i].SendAt = at
			steps[i].SendNote = "Queued for " + formatWhen(at, "")
		}
	}
}

func formatWhen(raw, tz string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		if parsed, err2 := time.Parse("2006-01-02 15:04:05", raw); err2 == nil {
			t = parsed
			err = nil
		}
	}
	if err != nil {
		return raw
	}
	if strings.TrimSpace(tz) != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return t.In(loc).Format("Mon 2 Jan 15:04 MST")
		}
	}
	return t.UTC().Format("Mon 2 Jan 15:04 MST")
}

func (s *Server) campaignChecklist(ws string, id int64, name, status, seqContent string, rev *campaignReview) ([]checklistItem, bool, []string) {
	var items []checklistItem
	var warnings []string
	ready := true

	seqOK := strings.TrimSpace(seqContent) != "" && len(rev.Sequence.Steps) > 0
	items = append(items, checklistItem{
		ID: "sequence", OK: seqOK, Label: "Sequence has at least one email",
		Fix: "/campaigns/" + strconv.FormatInt(id, 10),
	})
	if !seqOK {
		ready = false
		warnings = append(warnings, "Add the emails this person will receive")
	}

	items = append(items, checklistItem{
		ID: "recipients", OK: rev.Enrolled > 0, Count: rev.Enrolled,
		Label: fmt.Sprintf("%d enrolled recipients", rev.Enrolled),
		Fix:   "/campaigns/" + strconv.FormatInt(id, 10) + "?tab=recipients",
	})
	if rev.Enrolled == 0 {
		ready = false
		if rev.PendingReview > 0 {
			warnings = append(warnings, fmt.Sprintf("%d people are waiting on shortlist review", rev.PendingReview))
		} else {
			warnings = append(warnings, "Approve people onto this campaign before sending")
		}
	}

	accOK := len(rev.Accounts) > 0
	items = append(items, checklistItem{
		ID: "sender", OK: accOK, Count: len(rev.Accounts),
		Label: "Sending account assigned", Fix: "/integrations?kind=send",
	})
	if !accOK {
		ready = false
		warnings = append(warnings, "Connect a mailbox, then assign it on this campaign")
	}

	windowOK := strings.TrimSpace(rev.Timezone) != "" && strings.Contains(rev.SendWindow, "–")
	items = append(items, checklistItem{
		ID: "window", OK: windowOK,
		Label: "Send window " + rev.SendWindow + " " + rev.Timezone,
		Fix:   "/schedule",
	})

	var reconnect int
	_ = queryRow(s.Store.DB, `
		SELECT COUNT(*) FROM campaign_accounts ca
		JOIN accounts a ON a.id = ca.account_id
		WHERE ca.campaign_id = ? AND a.status != 'active'`, id).Scan(&reconnect)
	var oauthBad int
	if accOK {
		for _, email := range rev.Accounts {
			h, _ := GetHostedKV(s.Store.DB, "account_oauth:"+strings.ToLower(email))
			if h == "reconnect_required" {
				oauthBad++
			}
		}
	}
	senderHealth := accOK && reconnect == 0 && oauthBad == 0
	items = append(items, checklistItem{
		ID: "sender_health", OK: senderHealth,
		Label: "Mailbox can send (active, credentials not asking to reconnect)",
		Fix:   "/accounts",
	})
	if reconnect > 0 {
		ready = false
		warnings = append(warnings, "A sending account is paused")
	}
	if oauthBad > 0 {
		ready = false
		warnings = append(warnings, "A sending account needs to reconnect")
	}

	if status != "draft" && status != "paused" {
		warnings = append(warnings, "campaign status is "+status)
	}

	_ = name
	_ = ws
	return items, ready, warnings
}

func (s *Server) activateIfReady(w http.ResponseWriter, r *http.Request, name string) bool {
	rev, err := s.buildCampaignReview(r, "")
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return false
	}
	if !rev.Ready {
		writeJSON(w, http.StatusBadRequest, envelope{
			Error:    &apiErr{Code: "not_ready", Message: "Campaign is not ready to send. Fix the checklist, then activate with confirm."},
			Data:     rev,
			Warnings: rev.Warnings,
		})
		return false
	}
	return true
}

func storeCampaignSequence(db *sql.DB, campaignID int64, yamlIn string) error {
	seq, yamlOut, err := parseSequenceYAML(yamlIn)
	if err != nil {
		return err
	}
	_ = seq
	_, err = exec(db, `UPDATE campaigns SET sequence_content = ?, sequence_file = '(inline)' WHERE id = ?`, yamlOut, campaignID)
	return err
}

func (s *Server) assignCampaignAccounts(ws string, campaignID int64, emails []string) error {
	if _, err := exec(s.Store.DB, `DELETE FROM campaign_accounts WHERE campaign_id = ?`, campaignID); err != nil {
		return err
	}
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		var id int64
		err := queryRow(s.Store.DB, `SELECT id FROM accounts WHERE workspace_id = ? AND email = ? AND status = 'active'`, ws, email).Scan(&id)
		if err != nil {
			return fmt.Errorf("account %s not found or not active", email)
		}
		if _, err := exec(s.Store.DB, `INSERT INTO campaign_accounts (campaign_id, account_id) VALUES (?, ?)`, campaignID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleEnrollShortlist(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	ws := s.workspaceFromRequest(r)
	var campaignID int64
	var status, seqContent string
	if err := queryRow(s.Store.DB, `SELECT id, status, COALESCE(sequence_content,'') FROM campaigns WHERE name = ?`, name).Scan(&campaignID, &status, &seqContent); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	}
	if status == "active" {
		writeErr(w, http.StatusBadRequest, "confirm_required", "enrolling into an active campaign requires using add-leads with confirm")
		return
	}
	if strings.TrimSpace(seqContent) == "" {
		writeErr(w, http.StatusBadRequest, "no_sequence", "save the sequence before enrolling people")
		return
	}
	var accounts int
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM campaign_accounts WHERE campaign_id = ?`, campaignID).Scan(&accounts)
	if accounts == 0 {
		writeErr(w, http.StatusBadRequest, "no_sender", "assign a sending account before enrolling — scheduling needs a mailbox")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		IDs         []int64 `json:"ids"`
		AllApproved bool    `json:"all_approved"`
	}
	_ = json.Unmarshal(body, &req)

	q := `
		SELECT id, email, first_name, last_name, company, title
		FROM shortlist_candidates
		WHERE workspace_id = ? AND campaign_id = ? AND status != 'enrolled' AND status != 'excluded'`
	args := []any{ws, campaignID}
	if len(req.IDs) > 0 {
		placeholders := make([]string, 0, len(req.IDs))
		for _, id := range req.IDs {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		q += ` AND id IN (` + strings.Join(placeholders, ",") + `)`
	} else if req.AllApproved {
		q += ` AND status = 'approved'`
	} else {
		writeErr(w, http.StatusBadRequest, "empty", "pass ids or all_approved=true")
		return
	}
	rows, err := query(s.Store.DB, q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	var csv strings.Builder
	csv.WriteString("email,first_name,last_name,company,title\n")
	var ids []int64
	for rows.Next() {
		var id int64
		var email, first, last, company, title string
		if err := rows.Scan(&id, &email, &first, &last, &company, &title); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		fmt.Fprintf(&csv, "%s,%s,%s,%s,%s\n", csvCell(email), csvCell(first), csvCell(last), csvCell(company), csvCell(title))
		ids = append(ids, id)
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", scanErr.Error())
		return
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "empty", "no shortlist rows to enroll — approve people first")
		return
	}
	res, err := engine.AddLeadsToCampaign(s.Store.DB, name, "", csv.String())
	if err != nil {
		writeErr(w, http.StatusBadRequest, "enroll_failed", err.Error())
		return
	}
	for _, id := range ids {
		_, _ = exec(s.Store.DB, `UPDATE shortlist_candidates SET status = 'enrolled' WHERE id = ?`, id)
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": campaignID, "enrolled": res, "count": len(ids),
		"next_actions": []string{"preview_campaign", "activate_campaign"},
	}})
}

func csvCell(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	if strings.ContainsAny(s, ",\n") {
		return `"` + s + `"`
	}
	return s
}
