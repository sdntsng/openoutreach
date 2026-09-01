package hosted

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
)

// handleSheetsImport fetches a public Google Sheets CSV export URL (or any CSV URL)
// and optionally appends leads to a campaign. Without campaign_id, returns preview only.
func (s *Server) handleSheetsImport(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Integrations["sheets"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_SHEETS is disabled")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	var req struct {
		URL        string `json:"url"`
		CampaignID int64  `json:"campaign_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	csvURL, err := normalizeSheetsCSVURL(strings.TrimSpace(req.URL))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_url", err.Error())
		return
	}
	csvData, err := fetchCSVURL(csvURL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "fetch_failed", err.Error())
		return
	}
	records, _, err := internal.ParseLeadsCSVFromReader(strings.NewReader(csvData))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "parse_failed", err.Error())
		return
	}
	ws := s.workspaceFromRequest(r)
	if req.CampaignID == 0 {
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
			"preview": true, "count": len(records), "csv": csvData,
		}, Warnings: []string{"pass campaign_id to append; preview only"}})
		return
	}
	var campaignName string
	if err := queryRow(s.Store.DB, `SELECT name FROM campaigns WHERE id = ?`, req.CampaignID).Scan(&campaignName); err != nil {
		writeErr(w, http.StatusBadRequest, "campaign_not_found", "campaign_id not found")
		return
	}
	res, err := internal.AddLeadsToCampaign(s.Store.DB, campaignName, "", csvData)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "import_failed", err.Error())
		return
	}
	_ = LogEnrichmentCall(s.Store.DB, ws, "sheets", "import", fmt.Sprintf("campaign=%d n=%d", req.CampaignID, len(records)), float64(len(records)))
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": req.CampaignID, "imported": res, "count": len(records),
	}})
}

// SyncScheduledSheets imports CSV from workspace sheets credentials whose metadata
// includes {"url":"...","campaign_id":N}. Invoked from hosted tick (Worker cron).
func (s *Server) SyncScheduledSheets() {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Integrations["sheets"] {
		return
	}
	key := s.encKey()
	if key == nil {
		return
	}
	list, err := ListIntegrationCredentials(s.Store.DB, key, s.WorkspaceID)
	if err != nil {
		return
	}
	for _, cred := range list {
		if cred.Provider != "sheets" || cred.Status != "active" {
			continue
		}
		var meta struct {
			URL        string `json:"url"`
			CampaignID int64  `json:"campaign_id"`
		}
		_ = json.Unmarshal([]byte(cred.Metadata), &meta)
		if strings.TrimSpace(meta.URL) == "" || meta.CampaignID == 0 {
			continue
		}
		csvURL, err := normalizeSheetsCSVURL(strings.TrimSpace(meta.URL))
		if err != nil {
			continue
		}
		csvData, err := fetchCSVURL(csvURL)
		if err != nil {
			continue
		}
		var campaignName string
		if err := queryRow(s.Store.DB, `SELECT name FROM campaigns WHERE id = ?`, meta.CampaignID).Scan(&campaignName); err != nil {
			continue
		}
		_, _ = internal.AddLeadsToCampaign(s.Store.DB, campaignName, "", csvData)
		_ = LogEnrichmentCall(s.Store.DB, s.WorkspaceID, "sheets", "cron_sync", fmt.Sprintf("campaign=%d", meta.CampaignID), 1)
	}
}

func normalizeSheetsCSVURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url must be http(s)")
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "docs.google.com") && strings.Contains(u.Path, "/spreadsheets/") {
		// Convert edit/view links to CSV export when possible.
		parts := strings.Split(u.Path, "/")
		var id string
		for i, p := range parts {
			if p == "d" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
		if id != "" {
			gid := u.Query().Get("gid")
			out := "https://docs.google.com/spreadsheets/d/" + id + "/export?format=csv"
			if gid != "" {
				out += "&gid=" + gid
			}
			return out, nil
		}
	}
	return raw, nil
}

func fetchCSVURL(raw string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Get(raw)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
