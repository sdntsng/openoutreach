package hosted

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andersmyrmel/cold-cli/internal"
)

type shortlistRow struct {
	ID               int64  `json:"id"`
	CampaignID       int64  `json:"campaign_id"`
	Email            string `json:"email"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	Company          string `json:"company"`
	Title            string `json:"title"`
	Domain           string `json:"domain"`
	Source           string `json:"source"`
	SourceAt         string `json:"source_at"`
	FitReason        string `json:"fit_reason"`
	EmailCheck       string `json:"email_check"`
	EmailCheckReason string `json:"email_check_reason"`
	PreviousOutreach string `json:"previous_outreach"`
	Status           string `json:"status"`
}

type shortlistIn struct {
	CampaignID int64  `json:"campaign_id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Company    string `json:"company"`
	Title      string `json:"title"`
	Domain     string `json:"domain"`
	Source     string `json:"source"`
	FitReason  string `json:"fit_reason"`
	CSV        string `json:"csv"`
}

func (s *Server) handleListShortlist(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	campaignID, _ := strconv.ParseInt(r.URL.Query().Get("campaign_id"), 10, 64)
	q := `
		SELECT id, campaign_id, email, first_name, last_name, company, title, domain,
			source, CAST(source_at AS TEXT), fit_reason, email_check, email_check_reason, previous_outreach, status
		FROM shortlist_candidates WHERE workspace_id = ?`
	args := []any{ws}
	if campaignID > 0 {
		q += ` AND campaign_id = ?`
		args = append(args, campaignID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY CASE status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 WHEN 'enrolled' THEN 2 ELSE 3 END, id DESC LIMIT 200`
	rows, err := query(s.Store.DB, q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	list := []shortlistRow{}
	pending, approved, excluded, enrolled := 0, 0, 0, 0
	for rows.Next() {
		var row shortlistRow
		if err := rows.Scan(&row.ID, &row.CampaignID, &row.Email, &row.FirstName, &row.LastName, &row.Company, &row.Title, &row.Domain,
			&row.Source, &row.SourceAt, &row.FitReason, &row.EmailCheck, &row.EmailCheckReason, &row.PreviousOutreach, &row.Status); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		list = append(list, row)
		switch row.Status {
		case "pending":
			pending++
		case "approved":
			approved++
		case "excluded":
			excluded++
		case "enrolled":
			enrolled++
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"candidates":   list,
		"counts":       map[string]int{"pending": pending, "approved": approved, "excluded": excluded, "enrolled": enrolled},
		"playbook":     s.loadPlaybook(ws),
		"next_actions": []string{"approve or exclude", "enroll approved into a draft campaign"},
	}})
}

func (s *Server) handleAddShortlist(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	var req shortlistIn
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	n, err := s.upsertShortlist(ws, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "shortlist_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"added": n, "campaign_id": req.CampaignID,
		"next_actions": []string{"review shortlist", "enroll approved", "do not activate yet"},
	}})
}

func (s *Server) upsertShortlist(ws string, req shortlistIn) (int, error) {
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "csv"
	}
	type rec struct {
		email, first, last, company, title, domain, fit string
	}
	var recs []rec
	if strings.TrimSpace(req.CSV) != "" {
		records, _, err := internal.ParseLeadsCSVFromReader(strings.NewReader(req.CSV))
		if err != nil {
			return 0, err
		}
		for _, r := range records {
			recs = append(recs, rec{
				email: r.Fields["email"], first: r.Fields["first_name"], last: r.Fields["last_name"],
				company: r.Fields["company"], title: r.Fields["title"], domain: r.Fields["domain"],
				fit: firstNonEmpty(r.Fields["fit_reason"], r.Fields["why"], req.FitReason),
			})
		}
	} else if strings.TrimSpace(req.Email) != "" {
		recs = append(recs, rec{
			email: req.Email, first: req.FirstName, last: req.LastName, company: req.Company,
			title: req.Title, domain: req.Domain, fit: req.FitReason,
		})
	} else {
		return 0, fmt.Errorf("email or csv is required")
	}
	n := 0
	for _, rec := range recs {
		email := strings.ToLower(strings.TrimSpace(rec.email))
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		domain := strings.TrimSpace(rec.domain)
		if domain == "" {
			domain = internal.ExtractDomain(email)
		}
		check, reason := syntaxEmailCheck(email)
		prev := previousOutreachSummary(s.Store.DB, email)
		_, err := exec(s.Store.DB, `
			INSERT INTO shortlist_candidates (
				workspace_id, campaign_id, email, first_name, last_name, company, title, domain,
				source, fit_reason, email_check, email_check_reason, previous_outreach, status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
			ON CONFLICT(workspace_id, campaign_id, email) DO UPDATE SET
				first_name = excluded.first_name,
				last_name = excluded.last_name,
				company = excluded.company,
				title = excluded.title,
				domain = excluded.domain,
				source = excluded.source,
				fit_reason = CASE WHEN excluded.fit_reason = '' THEN shortlist_candidates.fit_reason ELSE excluded.fit_reason END,
				email_check = excluded.email_check,
				email_check_reason = excluded.email_check_reason,
				previous_outreach = excluded.previous_outreach,
				status = CASE WHEN shortlist_candidates.status IN ('enrolled','excluded') THEN shortlist_candidates.status ELSE 'pending' END`,
			ws, req.CampaignID, email, rec.first, rec.last, rec.company, rec.title, domain,
			source, rec.fit, check, reason, prev)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Server) handlePatchShortlist(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "invalid shortlist id")
		return
	}
	var ownerWS string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM shortlist_candidates WHERE id = ?`, id).Scan(&ownerWS); err != nil || ownerWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "shortlist row not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Status    string `json:"status"`
		FitReason string `json:"fit_reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "" && status != "pending" && status != "approved" && status != "excluded" {
		writeErr(w, http.StatusBadRequest, "invalid_status", "status must be pending, approved, or excluded")
		return
	}
	if status != "" {
		if _, err := exec(s.Store.DB, `UPDATE shortlist_candidates SET status = ? WHERE id = ? AND status != 'enrolled'`, status, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	if req.FitReason != "" || status == "" {
		_, _ = exec(s.Store.DB, `UPDATE shortlist_candidates SET fit_reason = ? WHERE id = ?`, strings.TrimSpace(req.FitReason), id)
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"id": id, "status": status}})
}

func syntaxEmailCheck(email string) (check, reason string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return "fail", "invalid_syntax"
	}
	domain := internal.ExtractDomain(email)
	if _, disp := disposableDomains[domain]; disp {
		return "fail", "disposable_domain"
	}
	local, _, _ := strings.Cut(email, "@")
	switch local {
	case "info", "support", "admin", "sales", "hello", "contact":
		return "unknown", "role_address"
	}
	return "unknown", "syntax_ok"
}

func previousOutreachSummary(db *sql.DB, email string) string {
	var n int
	var last sql.NullString
	_ = queryRow(db, `
		SELECT COUNT(*), MAX(CAST(e.timestamp AS TEXT))
		FROM events e JOIN leads l ON l.id = e.lead_id
		WHERE LOWER(l.email) = ? AND e.type IN ('sent', 'reply')`, strings.ToLower(email)).Scan(&n, &last)
	if n == 0 {
		return "None"
	}
	when := ""
	if last.Valid && last.String != "" {
		when = "; last " + formatWhen(last.String, "")
	}
	return fmt.Sprintf("%d prior send/reply events%s", n, when)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
