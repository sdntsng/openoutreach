package hosted

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// WorkspacePlaybook is workspace-level context for compose and setup pages.
// Stored in hosted_kv. Never activates a campaign.
type WorkspacePlaybook struct {
	Company              string   `json:"company"`
	Website              string   `json:"website"`
	Location             string   `json:"location"`
	Description          string   `json:"description"`
	Competitors          []string `json:"competitors"`
	Offer                string   `json:"offer"`
	Problem              string   `json:"problem"`
	ExampleClients       []string `json:"example_clients"`
	Keywords             []string `json:"keywords"`
	Audience             string   `json:"audience"`
	Geography            string   `json:"geography"`
	CompanySize          string   `json:"company_size"`
	TemplateInstructions string   `json:"template_instructions"`
	DefaultSequenceYAML  string   `json:"default_sequence_yaml"`
	SendWindowStart      string   `json:"send_window_start"`
	SendWindowEnd        string   `json:"send_window_end"`
	SendDays             string   `json:"send_days"`
	Timezone             string   `json:"timezone"`
}

func playbookKey(ws string) string {
	return "playbook:" + ws
}

func emptyPlaybook() WorkspacePlaybook {
	return WorkspacePlaybook{
		Competitors:     []string{},
		ExampleClients:  []string{},
		Keywords:        []string{},
		SendDays:        "1,2,3,4,5",
		Timezone:        "UTC",
		SendWindowStart: "09:00",
		SendWindowEnd:   "17:00",
	}
}

func (s *Server) loadPlaybook(ws string) WorkspacePlaybook {
	pb := emptyPlaybook()
	raw, err := GetHostedKV(s.Store.DB, playbookKey(ws))
	if err != nil || strings.TrimSpace(raw) == "" {
		return pb
	}
	_ = json.Unmarshal([]byte(raw), &pb)
	if pb.Competitors == nil {
		pb.Competitors = []string{}
	}
	if pb.ExampleClients == nil {
		pb.ExampleClients = []string{}
	}
	if pb.Keywords == nil {
		pb.Keywords = []string{}
	}
	return pb
}

func (s *Server) handleGetPlaybook(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	writeJSON(w, http.StatusOK, envelope{Data: s.loadPlaybook(ws)})
}

func (s *Server) handlePutPlaybook(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	body, _ := io.ReadAll(r.Body)
	cur := s.loadPlaybook(ws)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &cur); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	cur.Company = strings.TrimSpace(cur.Company)
	cur.Website = strings.TrimSpace(cur.Website)
	cur.Location = strings.TrimSpace(cur.Location)
	if cur.Competitors == nil {
		cur.Competitors = []string{}
	}
	if cur.ExampleClients == nil {
		cur.ExampleClients = []string{}
	}
	if cur.Keywords == nil {
		cur.Keywords = []string{}
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}
	if err := SetHostedKV(s.Store.DB, playbookKey(ws), string(raw)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: cur})
}
