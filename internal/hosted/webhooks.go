package hosted

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andersmyrmel/cold-cli/internal"
)

func (s *Server) handleWebhookIngest(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	provider := strings.ToLower(r.PathValue("provider"))
	if provider == "" {
		provider = "generic"
	}
	if provider == "clay" && !caps.Integrations["clay"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_CLAY is disabled")
		return
	}
	if provider != "clay" && !caps.Integrations["webhook"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_WEBHOOK is disabled")
		return
	}

	ws := s.workspaceFromRequest(r)
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "default"
	}
	campaignID, _ := strconv.ParseInt(r.URL.Query().Get("campaign_id"), 10, 64)

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read_failed", err.Error())
		return
	}

	key := s.encKey()
	if key != nil {
		var encSecret string
		var storedCampaign sql.NullInt64
		qerr := queryRow(s.Store.DB, `
			SELECT encrypted_hmac_secret, campaign_id FROM webhook_endpoints
			WHERE workspace_id = ? AND provider = ? AND name = ? AND status = 'active'`,
			ws, provider, name).Scan(&encSecret, &storedCampaign)
		if qerr == nil && encSecret != "" {
			plain, derr := Decrypt(key, encSecret)
			if derr == nil && len(plain) > 0 {
				sig := r.Header.Get("X-OpenOutreach-Signature")
				if sig == "" {
					sig = r.Header.Get("X-Clay-Signature")
				}
				mac := hmac.New(sha256.New, plain)
				mac.Write(body)
				expected := hex.EncodeToString(mac.Sum(nil))
				if !hmac.Equal([]byte(strings.TrimPrefix(strings.ToLower(sig), "sha256=")), []byte(expected)) {
					writeErr(w, http.StatusUnauthorized, "bad_signature", "HMAC signature mismatch")
					return
				}
			}
			if campaignID == 0 && storedCampaign.Valid {
				campaignID = storedCampaign.Int64
			}
		}
	}

	leads, warnings := normalizeWebhookLeads(body)
	if campaignID == 0 {
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
			"preview": true, "leads": leads, "count": len(leads), "csv": leadsToCSV(leads),
		}, Warnings: append(warnings, "pass campaign_id to append; preview only")})
		return
	}

	csvData := leadsToCSV(leads)
	var campaignName string
	if err := queryRow(s.Store.DB, `SELECT name FROM campaigns WHERE id = ?`, campaignID).Scan(&campaignName); err != nil {
		writeErr(w, http.StatusBadRequest, "campaign_not_found", "campaign_id not found")
		return
	}
	res, err := internal.AddLeadsToCampaign(s.Store.DB, campaignName, "", csvData)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "import_failed", err.Error())
		return
	}
	_ = LogEnrichmentCall(s.Store.DB, ws, provider, "ingest", fmt.Sprintf("campaign=%d n=%d", campaignID, len(leads)), float64(len(leads)))
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": campaignID, "imported": res, "count": len(leads),
	}, Warnings: warnings})
}

func normalizeWebhookLeads(body []byte) ([]map[string]string, []string) {
	var warnings []string
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, []string{"invalid json"}
	}
	var items []map[string]any
	switch t := raw.(type) {
	case map[string]any:
		if arr, ok := t["leads"].([]any); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]any); ok {
					items = append(items, m)
				}
			}
		} else if arr, ok := t["people"].([]any); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]any); ok {
					items = append(items, m)
				}
			}
		} else {
			items = append(items, t)
		}
	case []any:
		for _, v := range t {
			if m, ok := v.(map[string]any); ok {
				items = append(items, m)
			}
		}
	}
	out := make([]map[string]string, 0, len(items))
	for _, m := range items {
		email := firstString(m, "email", "Email", "work_email")
		if email == "" || !strings.Contains(email, "@") {
			warnings = append(warnings, "skipped row without valid email")
			continue
		}
		out = append(out, map[string]string{
			"email":        strings.ToLower(strings.TrimSpace(email)),
			"first_name":   firstString(m, "first_name", "firstName", "First Name"),
			"last_name":    firstString(m, "last_name", "lastName", "Last Name"),
			"company":      firstString(m, "company", "company_name", "organization"),
			"domain":       firstString(m, "domain", "company_domain"),
			"title":        firstString(m, "title", "job_title"),
			"linkedin_url": firstString(m, "linkedin_url", "linkedin"),
		})
	}
	return out, warnings
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				return strings.TrimSpace(t)
			case float64:
				return strconv.FormatInt(int64(t), 10)
			}
		}
	}
	return ""
}

func (s *Server) handlePutWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Provider   string `json:"provider"`
		Name       string `json:"name"`
		HMACSecret string `json:"hmac_secret"`
		CampaignID int64  `json:"campaign_id"`
		FieldMap   string `json:"field_map"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	if req.Provider == "" {
		req.Provider = "generic"
	}
	if req.Name == "" {
		req.Name = "default"
	}
	if req.FieldMap == "" {
		req.FieldMap = "{}"
	}
	ws := s.workspaceFromRequest(r)
	enc := ""
	var err error
	if strings.TrimSpace(req.HMACSecret) != "" {
		enc, err = Encrypt(key, []byte(strings.TrimSpace(req.HMACSecret)))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "encrypt_failed", err.Error())
			return
		}
	}
	var campaign any
	if req.CampaignID > 0 {
		campaign = req.CampaignID
	}
	_, err = exec(s.Store.DB, `
		INSERT INTO webhook_endpoints (workspace_id, provider, name, encrypted_hmac_secret, campaign_id, field_map)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, provider, name) DO UPDATE SET
			encrypted_hmac_secret = CASE WHEN excluded.encrypted_hmac_secret = '' THEN webhook_endpoints.encrypted_hmac_secret ELSE excluded.encrypted_hmac_secret END,
			campaign_id = COALESCE(excluded.campaign_id, webhook_endpoints.campaign_id),
			field_map = excluded.field_map`,
		ws, strings.ToLower(req.Provider), req.Name, enc, campaign, req.FieldMap)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "put_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"provider": req.Provider, "name": req.Name,
		"ingest_path": fmt.Sprintf("/api/v1/integrations/%s/ingest?name=%s", req.Provider, req.Name),
	}})
}