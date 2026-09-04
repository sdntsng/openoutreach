package hosted

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

// DNS lookups are injectable so tests never hit the network.
var lookupMX = net.LookupMX
var lookupTXT = net.LookupTXT

// SetLookupFns overrides public DNS lookups. Tests restore the previous pair.
func SetLookupFns(mx func(string) ([]*net.MX, error), txt func(string) ([]string, error)) {
	if mx != nil {
		lookupMX = mx
	}
	if txt != nil {
		lookupTXT = txt
	}
}

// CurrentLookupFns returns the active MX/TXT lookup functions.
func CurrentLookupFns() (func(string) ([]*net.MX, error), func(string) ([]string, error)) {
	return lookupMX, lookupTXT
}

var outboundHTTPClient = &http.Client{Timeout: 8 * time.Second}

var disposableDomains = map[string]struct{}{
	"mailinator.com": {}, "yopmail.com": {}, "guerrillamail.com": {},
	"tempmail.com": {}, "temp-mail.org": {}, "10minutemail.com": {},
	"throwaway.email": {}, "trashmail.com": {}, "sharklasers.com": {},
	"guerrillamail.info": {}, "getnada.com": {}, "maildrop.cc": {},
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	var accounts, campaigns, leads, suppressions int
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM accounts WHERE workspace_id = ?`, ws).Scan(&accounts)
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM campaigns WHERE workspace_id = ?`, ws).Scan(&campaigns)
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM leads`).Scan(&leads)
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM suppressions WHERE workspace_id = ?`, ws).Scan(&suppressions)

	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	var next []string
	if accounts == 0 {
		next = append(next, "connect_account")
	}
	if leads == 0 {
		next = append(next, "import_leads")
	}
	if campaigns == 0 {
		next = append(next, "create_draft")
	}
	if accounts > 0 && leads > 0 && campaigns > 0 {
		next = append(next, "preview_and_activate")
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"workspace_id":       ws,
		"accounts":           accounts,
		"campaigns":          campaigns,
		"leads":              leads,
		"suppressions":       suppressions,
		"encryption_ready":   caps.EncryptionReady,
		"google_oauth_ready": caps.GoogleOAuthReady,
		"next_actions":       next,
	}})
}

func (s *Server) handleListSuppressions(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	rows, err := query(s.Store.DB, `
		SELECT id, workspace_id, kind, value, created_at
		FROM suppressions WHERE workspace_id = ?
		ORDER BY created_at DESC, id DESC`, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	type row struct {
		ID          int64  `json:"id"`
		WorkspaceID string `json:"workspace_id"`
		Kind        string `json:"kind"`
		Value       string `json:"value"`
		CreatedAt   string `json:"created_at"`
	}
	var list []row
	for rows.Next() {
		var item row
		var created time.Time
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Kind, &item.Value, &created); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		list = append(list, item)
	}
	if list == nil {
		list = []row{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"suppressions": list}})
}

func (s *Server) handleAddSuppression(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Kind   string `json:"kind"`
		Value  string `json:"value"`
		Email  string `json:"email"`
		Domain string `json:"domain"`
		CSV    string `json:"csv"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	value := strings.ToLower(strings.TrimSpace(req.Value))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(req.Email))
		if value != "" {
			kind = "email"
		}
	}
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(req.Domain))
		if value != "" {
			kind = "domain"
		}
	}
	if req.CSV != "" && value == "" {
		added, skipped, err := s.addSuppressionsFromCSV(ws, req.CSV, kind)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "import_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"added": added, "skipped": skipped}})
		return
	}
	if kind == "" {
		if strings.Contains(value, "@") {
			kind = "email"
		} else {
			kind = "domain"
		}
	}
	if kind != "email" && kind != "domain" {
		writeErr(w, http.StatusBadRequest, "invalid_kind", "kind must be email or domain")
		return
	}
	if value == "" {
		writeErr(w, http.StatusBadRequest, "invalid_value", "email or domain is required")
		return
	}
	if kind == "email" {
		value = strings.ToLower(strings.TrimSpace(value))
	} else {
		value = strings.TrimPrefix(value, "@")
	}
	id, err := s.upsertSuppression(ws, kind, value)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	_, _ = engine.BlacklistLead(s.Store.DB, value)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"id": id, "kind": kind, "value": value}})
}

func (s *Server) handleDeleteSuppression(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "invalid suppression id")
		return
	}
	res, err := exec(s.Store.DB, `DELETE FROM suppressions WHERE id = ? AND workspace_id = ?`, id, ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "suppression not found")
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"deleted": true, "id": id}})
}

func (s *Server) handleVerifyLeads(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		CSV    string   `json:"csv"`
		Emails []string `json:"emails"`
		Email  string   `json:"email"`
	}
	_ = json.Unmarshal(body, &req)
	emails := append([]string{}, req.Emails...)
	if req.Email != "" {
		emails = append(emails, req.Email)
	}
	if req.CSV != "" {
		for i, line := range strings.Split(req.CSV, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || (i == 0 && strings.Contains(strings.ToLower(line), "email")) {
				continue
			}
			col := strings.Split(line, ",")[0]
			emails = append(emails, strings.TrimSpace(col))
		}
	}
	type result struct {
		Email      string `json:"email"`
		OK         bool   `json:"ok"`
		Reason     string `json:"reason,omitempty"`
		Disposable bool   `json:"disposable,omitempty"`
		MX         bool   `json:"mx"`
	}
	out := make([]result, 0, len(emails))
	okN, badN := 0, 0
	for _, raw := range emails {
		email := strings.ToLower(strings.TrimSpace(raw))
		item := result{Email: email}
		if email == "" {
			continue
		}
		if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
			item.Reason = "invalid_syntax"
			badN++
			out = append(out, item)
			continue
		}
		domain := internal.ExtractDomain(email)
		if _, disp := disposableDomains[domain]; disp {
			item.Disposable = true
			item.Reason = "disposable_domain"
			badN++
			out = append(out, item)
			continue
		}
		mxs, err := lookupMX(domain)
		if err != nil || len(mxs) == 0 {
			item.Reason = "no_mx"
			badN++
			out = append(out, item)
			continue
		}
		item.OK = true
		item.MX = true
		okN++
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"results": out, "valid": okN, "invalid": badN, "total": len(out),
	}})
}

func (s *Server) handleAccountDNS(w http.ResponseWriter, r *http.Request) {
	email, err := s.accountEmail(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	domain := internal.ExtractDomain(email)
	if domain == "" {
		writeErr(w, http.StatusBadRequest, "invalid_domain", "account email has no domain")
		return
	}
	mxOK, mxRecords := false, []string{}
	if mxs, err := lookupMX(domain); err == nil {
		for _, mx := range mxs {
			if mx != nil && mx.Host != "" {
				mxRecords = append(mxRecords, strings.TrimSuffix(mx.Host, "."))
			}
		}
		mxOK = len(mxRecords) > 0
	}
	spfOK, dmarcOK := false, false
	var spfRecord, dmarcRecord string
	if txts, err := lookupTXT(domain); err == nil {
		for _, t := range txts {
			if strings.Contains(strings.ToLower(t), "v=spf1") {
				spfOK = true
				spfRecord = t
				break
			}
		}
	}
	if txts, err := lookupTXT("_dmarc." + domain); err == nil {
		for _, t := range txts {
			if strings.Contains(strings.ToLower(t), "v=dmarc1") {
				dmarcOK = true
				dmarcRecord = t
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"email": email, "domain": domain,
		"mx": mxOK, "mx_records": mxRecords,
		"spf": spfOK, "spf_record": spfRecord,
		"dmarc": dmarcOK, "dmarc_record": dmarcRecord,
		"note": "Public DNS only. DKIM is at the mailbox provider and is not fetched here.",
	}})
}

func (s *Server) handleCloneCampaign(w http.ResponseWriter, r *http.Request) {
	srcName, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Name     string `json:"name"`
		LeadsCSV string `json:"leads_csv"`
	}
	_ = json.Unmarshal(body, &req)
	if strings.TrimSpace(req.Name) == "" {
		req.Name = srcName + "-copy"
	}
	csv := strings.TrimSpace(req.LeadsCSV)
	if csv == "" {
		csv, err = exportCampaignLeadsCSV(s.Store.DB, srcName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "export_failed", err.Error())
			return
		}
	}
	csv, skipped, err := s.filterSuppressedCSV(ws, csv)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "leads_failed", err.Error())
		return
	}
	res, err := engine.CloneCampaign(s.Store.DB, engine.CloneCampaignOpts{
		WorkspaceID: ws, SourceName: srcName, NewName: req.Name, LeadsInline: csv,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "clone_failed", err.Error())
		return
	}
	if res.Status != "draft" {
		writeErr(w, http.StatusInternalServerError, "clone_not_draft", "clone must stay draft")
		return
	}
	warnings := append([]string{}, res.Warnings...)
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("%d suppressed leads skipped", skipped))
	}
	writeJSON(w, http.StatusCreated, envelope{Data: map[string]any{
		"campaign_id": res.ID, "name": res.Name, "status": "draft",
		"lead_count": res.Leads, "scheduled_messages": res.ScheduledSends,
		"next_actions": []string{"preview_campaign", "activate_campaign"},
	}, Warnings: warnings})
}

func (s *Server) handlePatchCampaign(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	var status string
	var id int64
	if err := queryRow(s.Store.DB, `SELECT id, status FROM campaigns WHERE name = ?`, name).Scan(&id, &status); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	}
	if status != "draft" && status != "paused" {
		writeErr(w, http.StatusBadRequest, "not_editable", "only draft or paused campaigns can be patched")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		SequenceYAML    *string `json:"sequence_yaml"`
		SendWindowStart *string `json:"send_window_start"`
		SendWindowEnd   *string `json:"send_window_end"`
		SendDays        *string `json:"send_days"`
		Timezone        *string `json:"timezone"`
		MinGapSeconds   *int    `json:"min_gap_seconds"`
		MaxGapSeconds   *int    `json:"max_gap_seconds"`
		OpenTracking    *bool   `json:"open_tracking"`
		OpenTrackingEn  *bool   `json:"open_tracking_enabled"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	opts := engine.UpdateCampaignOpts{
		SendWindowStart: req.SendWindowStart,
		SendWindowEnd:   req.SendWindowEnd,
		SendDays:        req.SendDays,
		Timezone:        req.Timezone,
		MinGapSeconds:   req.MinGapSeconds,
		MaxGapSeconds:   req.MaxGapSeconds,
	}
	if req.SequenceYAML != nil {
		f, err := os.CreateTemp("", "openoutreach-seq-*.yml")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "temp_failed", err.Error())
			return
		}
		defer os.Remove(f.Name())
		if _, err := f.WriteString(*req.SequenceYAML); err != nil {
			_ = f.Close()
			writeErr(w, http.StatusInternalServerError, "temp_failed", err.Error())
			return
		}
		_ = f.Close()
		path := f.Name()
		opts.SequenceFile = &path
	}
	if err := engine.UpdateCampaign(s.Store.DB, name, opts); err != nil {
		writeErr(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	enabled := req.OpenTracking
	if enabled == nil {
		enabled = req.OpenTrackingEn
	}
	if enabled != nil {
		val := "0"
		if *enabled {
			val = "1"
		}
		_ = SetHostedKV(s.Store.DB, fmt.Sprintf("campaign_open_tracking:%d", id), val)
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"name": name, "status": status, "campaign_id": id,
		"next_actions": []string{"preview_campaign"},
	}})
}

func (s *Server) handleExportLeads(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if q != "" && domain == "" {
		domain = ""
	}
	csvBody, n, err := exportLeadsCSV(s.Store.DB, q, domain)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "export_failed", err.Error())
		return
	}
	if wantsCSV(r) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="leads.csv"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(csvBody))
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"csv": csvBody, "count": n}})
}

func (s *Server) handleExportCampaignLeads(w http.ResponseWriter, r *http.Request) {
	name, err := s.resolveCampaign(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	csvBody, err := exportCampaignLeadsCSV(s.Store.DB, name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "export_failed", err.Error())
		return
	}
	n := strings.Count(csvBody, "\n") - 1
	if n < 0 {
		n = 0
	}
	if wantsCSV(r) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-leads.csv"`, name))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(csvBody))
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"csv": csvBody, "count": n, "campaign": name}})
}

func wantsCSV(r *http.Request) bool {
	if r.URL.Query().Get("format") == "csv" {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "text/csv")
}

func (s *Server) upsertSuppression(ws, kind, value string) (int64, error) {
	_, err := exec(s.Store.DB, `
		INSERT INTO suppressions (workspace_id, kind, value) VALUES (?, ?, ?)
		ON CONFLICT (workspace_id, kind, value) DO NOTHING`, ws, kind, value)
	if err != nil {
		return 0, err
	}
	var id int64
	_ = queryRow(s.Store.DB, `
		SELECT id FROM suppressions WHERE workspace_id = ? AND kind = ? AND value = ?`,
		ws, kind, value).Scan(&id)
	return id, nil
}

func (s *Server) addSuppressionsFromCSV(ws, raw, kindHint string) (added, skipped int, err error) {
	kindHint = strings.ToLower(strings.TrimSpace(kindHint))
	if kindHint == "domain" {
		for _, line := range strings.Split(raw, "\n") {
			token := strings.ToLower(strings.TrimSpace(strings.Split(line, ",")[0]))
			token = strings.TrimPrefix(strings.ReplaceAll(token, "@", ""), "www.")
			if token == "" || token == "domain" || token == "domains" || strings.Contains(token, " ") {
				continue
			}
			if _, e := s.upsertSuppression(ws, "domain", token); e != nil {
				return added, skipped, e
			}
			added++
		}
		return added, skipped, nil
	}
	records, _, parseErr := internal.ParseLeadsCSVFromReader(strings.NewReader(raw))
	if parseErr == nil {
		for _, rec := range records {
			email := strings.ToLower(strings.TrimSpace(rec.Fields["email"]))
			domain := strings.ToLower(strings.TrimSpace(rec.Fields["domain"]))
			if kindHint == "domain" || (email == "" && domain != "") {
				value := strings.TrimPrefix(domain, "@")
				if value == "" {
					value = strings.TrimPrefix(email, "@")
				}
				if value == "" || strings.Contains(value, " ") {
					skipped++
					continue
				}
				if _, e := s.upsertSuppression(ws, "domain", value); e != nil {
					return added, skipped, e
				}
				added++
				continue
			}
			if email == "" {
				skipped++
				continue
			}
			if _, e := s.upsertSuppression(ws, "email", email); e != nil {
				return added, skipped, e
			}
			_, _ = engine.BlacklistLead(s.Store.DB, email)
			added++
		}
		return added, skipped, nil
	}
	for i, line := range strings.Split(raw, "\n") {
		token := strings.ToLower(strings.TrimSpace(strings.Split(line, ",")[0]))
		token = strings.TrimPrefix(token, "@")
		if token == "" || token == "email" || token == "domain" {
			continue
		}
		if i == 0 && (strings.Contains(token, "email") || strings.Contains(token, "domain")) {
			continue
		}
		kind := kindHint
		if kind == "" {
			if strings.Contains(token, "@") {
				kind = "email"
			} else {
				kind = "domain"
			}
		}
		if kind == "email" && !strings.Contains(token, "@") {
			skipped++
			continue
		}
		if kind == "domain" {
			token = strings.TrimPrefix(strings.ReplaceAll(token, "@", ""), "www.")
		}
		if _, e := s.upsertSuppression(ws, kind, token); e != nil {
			return added, skipped, e
		}
		if kind == "email" {
			_, _ = engine.BlacklistLead(s.Store.DB, token)
		}
		added++
	}
	return added, skipped, nil
}

func IsSuppressed(db *sql.DB, workspaceID, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}
	domain := internal.ExtractDomain(email)
	var n int
	err := queryRow(db, `
		SELECT COUNT(*) FROM suppressions
		WHERE workspace_id = ? AND (
			(kind = 'email' AND value = ?) OR
			(kind = 'domain' AND value = ?)
		)`, workspaceID, email, domain).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Server) filterSuppressedCSV(ws, raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, 0, nil
	}
	records, headers, err := internal.ParseLeadsCSVFromReader(strings.NewReader(raw))
	if err != nil {
		return raw, 0, err
	}
	kept := make([]internal.LeadRecord, 0, len(records))
	skipped := 0
	for _, rec := range records {
		email := rec.Fields["email"]
		sup, err := IsSuppressed(s.Store.DB, ws, email)
		if err != nil {
			return "", 0, err
		}
		if sup {
			skipped++
			continue
		}
		kept = append(kept, rec)
	}
	if skipped == 0 {
		return raw, 0, nil
	}
	if len(kept) == 0 {
		return "", skipped, fmt.Errorf("all %d leads are on the suppression list", skipped)
	}
	return encodeLeadCSV(headers, kept), skipped, nil
}

func encodeLeadCSV(headers []string, records []internal.LeadRecord) string {
	if len(headers) == 0 {
		headers = []string{"email", "first_name", "last_name", "company"}
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(headers)
	for _, rec := range records {
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = rec.Fields[h]
		}
		_ = w.Write(row)
	}
	w.Flush()
	return buf.String()
}

func exportCampaignLeadsCSV(db *sql.DB, campaignName string) (string, error) {
	rows, err := query(db, `
		SELECT l.email, COALESCE(l.first_name,''), COALESCE(l.last_name,''), COALESCE(l.company,''), COALESCE(l.domain,'')
		FROM leads l
		JOIN campaign_leads cl ON cl.lead_id = l.id
		JOIN campaigns c ON c.id = cl.campaign_id
		WHERE c.name = ?
		ORDER BY l.id`, campaignName)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"email", "first_name", "last_name", "company", "domain"})
	n := 0
	for rows.Next() {
		var email, first, last, company, domain string
		if err := rows.Scan(&email, &first, &last, &company, &domain); err != nil {
			return "", err
		}
		_ = w.Write([]string{email, first, last, company, domain})
		n++
	}
	w.Flush()
	if n == 0 {
		return "", fmt.Errorf("source campaign has no leads to clone")
	}
	return buf.String(), nil
}

func exportLeadsCSV(db *sql.DB, q, domain string) (string, int, error) {
	sqlStr := `
		SELECT l.email, COALESCE(l.first_name,''), COALESCE(l.last_name,''), COALESCE(l.company,''),
			COALESCE(l.domain,''), COALESCE(l.global_status,'')
		FROM leads l WHERE 1=1`
	var args []any
	if domain != "" {
		sqlStr += " AND l.domain = ?"
		args = append(args, strings.ToLower(domain))
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		sqlStr += " AND (LOWER(l.email) LIKE ? OR LOWER(COALESCE(l.first_name,'')) LIKE ? OR LOWER(COALESCE(l.last_name,'')) LIKE ? OR LOWER(COALESCE(l.company,'')) LIKE ? OR LOWER(COALESCE(l.domain,'')) LIKE ?)"
		args = append(args, like, like, like, like, like)
	}
	sqlStr += " ORDER BY l.id"
	rows, err := query(db, sqlStr, args...)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"email", "first_name", "last_name", "company", "domain", "global_status"})
	n := 0
	for rows.Next() {
		var email, first, last, company, dom, status string
		if err := rows.Scan(&email, &first, &last, &company, &dom, &status); err != nil {
			return "", 0, err
		}
		_ = w.Write([]string{email, first, last, company, dom, status})
		n++
	}
	w.Flush()
	return buf.String(), n, nil
}

func searchLeads(db *sql.DB, q, domain, status string, limit int) ([]internal.LeadListRow, error) {
	if q == "" {
		return internal.ListLeads(db, domain, status, limit)
	}
	if limit <= 0 {
		limit = 100
	}
	like := "%" + strings.ToLower(q) + "%"
	sqlStr := `
		SELECT l.id, l.email, l.first_name, l.company, l.domain, l.global_status,
			(SELECT COUNT(*) FROM campaign_leads WHERE lead_id = l.id) as campaigns
		FROM leads l
		WHERE (LOWER(l.email) LIKE ? OR LOWER(COALESCE(l.first_name,'')) LIKE ? OR LOWER(COALESCE(l.last_name,'')) LIKE ? OR LOWER(COALESCE(l.company,'')) LIKE ? OR LOWER(COALESCE(l.domain,'')) LIKE ?)`
	args := []any{like, like, like, like, like}
	if domain != "" {
		sqlStr += " AND l.domain = ?"
		args = append(args, strings.ToLower(domain))
	}
	if status != "" {
		sqlStr += " AND l.global_status = ?"
		args = append(args, status)
	}
	sqlStr += " ORDER BY l.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := query(db, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leads []internal.LeadListRow
	for rows.Next() {
		var l internal.LeadListRow
		if err := rows.Scan(&l.ID, &l.Email, &l.FirstName, &l.Company, &l.Domain, &l.GlobalStatus, &l.Campaigns); err != nil {
			return nil, err
		}
		leads = append(leads, l)
	}
	return leads, rows.Err()
}

func (s *Server) dispatchOutboundEvents(workspaceID string) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Integrations["outbound"] && !caps.Integrations["webhook"] {
		return
	}
	key := s.encKey()
	if key == nil {
		return
	}
	var enc, metadata string
	err := queryRow(s.Store.DB, `
		SELECT encrypted_secret, COALESCE(metadata, '')
		FROM integration_credentials
		WHERE workspace_id = ? AND provider = 'outbound' AND status = 'active'
		ORDER BY id DESC LIMIT 1`, workspaceID).Scan(&enc, &metadata)
	if err != nil {
		return
	}
	plain, err := Decrypt(key, enc)
	if err != nil || len(plain) == 0 {
		return
	}
	hookURL := strings.TrimSpace(string(plain))
	if !strings.HasPrefix(hookURL, "http://") && !strings.HasPrefix(hookURL, "https://") {
		if u := jsonStringField(metadata, "url"); u != "" {
			hookURL = u
		}
	}
	if !strings.HasPrefix(hookURL, "http://") && !strings.HasPrefix(hookURL, "https://") {
		return
	}
	hmacSecret := jsonStringField(metadata, "hmac_secret")

	cursorKey := "outbound_events_cursor:" + workspaceID
	cursor, _ := GetHostedKV(s.Store.DB, cursorKey)
	lastID, _ := strconv.ParseInt(strings.TrimSpace(cursor), 10, 64)

	rows, err := query(s.Store.DB, `
		SELECT e.id, e.type, e.campaign_id, e.lead_id, e.timestamp, COALESCE(l.email, '')
		FROM events e
		JOIN campaigns c ON c.id = e.campaign_id
		LEFT JOIN leads l ON l.id = e.lead_id
		WHERE c.workspace_id = ? AND e.id > ? AND e.type IN ('sent', 'reply', 'bounce')
		ORDER BY e.id ASC
		LIMIT 50`, workspaceID, lastID)
	if err != nil {
		return
	}
	type ev struct {
		ID         int64
		Type       string
		CampaignID int64
		LeadID     int64
		Timestamp  time.Time
		Email      string
	}
	var events []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.ID, &e.Type, &e.CampaignID, &e.LeadID, &e.Timestamp, &e.Email); err != nil {
			rows.Close()
			return
		}
		events = append(events, e)
	}
	rows.Close()

	var maxID int64 = lastID
	for _, e := range events {
		payload, _ := json.Marshal(map[string]any{
			"type":        e.Type,
			"campaign_id": e.CampaignID,
			"lead_id":     e.LeadID,
			"email":       e.Email,
			"timestamp":   e.Timestamp.UTC().Format(time.RFC3339),
		})
		req, err := http.NewRequest(http.MethodPost, hookURL, bytes.NewReader(payload))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "OpenOutreach-outbound")
		if hmacSecret != "" {
			mac := hmac.New(sha256.New, []byte(hmacSecret))
			mac.Write(payload)
			req.Header.Set("X-OpenOutreach-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := outboundHTTPClient.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if e.ID > maxID {
			maxID = e.ID
		}
	}
	if maxID > lastID {
		_ = SetHostedKV(s.Store.DB, cursorKey, strconv.FormatInt(maxID, 10))
	}
}
