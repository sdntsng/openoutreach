package hosted

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func testApolloKey(apiKey string) (bool, string, error) {
	req, err := http.NewRequest(http.MethodPost, "https://api.apollo.io/api/v1/auth/health", bytes.NewReader([]byte("{}")))
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		if len(apiKey) >= 16 {
			return true, "apollo key accepted locally (live health unreachable)", nil
		}
		return false, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return false, "", fmt.Errorf("apollo HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return true, "apollo health ok", nil
}

// ApolloSearchRequest is a minimal people search payload.
type ApolloSearchRequest struct {
	CredentialName string   `json:"credential_name"`
	QKeywords      string   `json:"q_keywords"`
	Titles         []string `json:"person_titles"`
	Locations      []string `json:"person_locations"`
	PerPage        int      `json:"per_page"`
	Page           int      `json:"page"`
}

type apolloPerson struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Title     string `json:"title"`
	LinkedIn  string `json:"linkedin_url"`
	OrgName   string `json:"organization_name"`
	Org       struct {
		Name   string `json:"name"`
		Domain string `json:"primary_domain"`
	} `json:"organization"`
}

// SearchApolloPeople searches Apollo and returns normalized lead rows (preview only).
func SearchApolloPeople(db *sql.DB, key []byte, workspaceID string, req ApolloSearchRequest) ([]map[string]string, error) {
	name := strings.TrimSpace(req.CredentialName)
	if name == "" {
		name = "default"
	}
	apiKey, err := ResolveIntegrationSecret(db, key, workspaceID, "apollo", name, 0)
	if err != nil {
		return nil, err
	}
	perPage := req.PerPage
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	payload := map[string]any{
		"q_keywords":       req.QKeywords,
		"person_titles":    req.Titles,
		"person_locations": req.Locations,
		"page":             page,
		"per_page":         perPage,
	}
	raw, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest(http.MethodPost, "https://api.apollo.io/api/v1/mixed_people/api_search", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("apollo search HTTP %d: %s", res.StatusCode, truncateBytes(body, 200))
	}
	var parsed struct {
		People []apolloPerson `json:"people"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	_ = LogEnrichmentCall(db, workspaceID, "apollo", "search", fmt.Sprintf("page=%d n=%d", page, len(parsed.People)), float64(len(parsed.People)))
	out := make([]map[string]string, 0, len(parsed.People))
	for _, p := range parsed.People {
		company := p.OrgName
		if company == "" {
			company = p.Org.Name
		}
		out = append(out, map[string]string{
			"email":        strings.TrimSpace(p.Email),
			"first_name":   p.FirstName,
			"last_name":    p.LastName,
			"company":      company,
			"domain":       p.Org.Domain,
			"title":        p.Title,
			"linkedin_url": p.LinkedIn,
		})
	}
	return out, nil
}

func leadsToCSV(rows []map[string]string) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"email", "first_name", "last_name", "company", "domain", "title", "linkedin_url"})
	for _, r := range rows {
		_ = w.Write([]string{r["email"], r["first_name"], r["last_name"], r["company"], r["domain"], r["title"], r["linkedin_url"]})
	}
	w.Flush()
	return buf.String()
}

func truncateBytes(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func (s *Server) handleApolloSearch(w http.ResponseWriter, r *http.Request) {
	key := s.encKey()
	if key == nil {
		writeErr(w, http.StatusServiceUnavailable, "vault_unconfigured", "CREDENTIAL_ENCRYPTION_KEY is required")
		return
	}
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, true, s.OAuth != nil)
	if !caps.Integrations["apollo"] {
		writeErr(w, http.StatusForbidden, "feature_disabled", "FEATURE_APOLLO is disabled")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req ApolloSearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	ws := s.workspaceFromRequest(r)
	people, err := SearchApolloPeople(s.Store.DB, key, ws, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "apollo_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"leads": people, "count": len(people), "csv": leadsToCSV(people),
		"next_actions": []string{"review preview", "POST /api/v1/campaigns/{id}/leads with csv", "do not activate without confirm"},
	}})
}
