package hosted

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
	"golang.org/x/oauth2"
)

// Server is the OpenOutreach hosted HTTP control plane.
type Server struct {
	Store          *engine.Store
	Creds          CredentialStore
	GWS            *RoutingGWS
	OAuth          *oauth2.Config
	WorkspaceID    string
	InternalToken  string
	PublicBaseURL  string
	TrackingSecret string
	UseMockGmail   bool
	ListenAddr     string
	Mux            *http.ServeMux
}

type envelope struct {
	Data     any      `json:"data,omitempty"`
	Error    *apiErr  `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type apiErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ServerOpts struct {
	WorkspaceID        string
	InternalToken      string
	PublicBaseURL      string
	TrackingSecret     string
	UseMockGmail       bool
	ListenAddr         string
	EncryptionKey      []byte
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
}

func NewServer(store *engine.Store, opts ServerOpts) (*Server, error) {
	s := &Server{
		Store:          store,
		WorkspaceID:    internal.NormalizeWorkspaceID(opts.WorkspaceID),
		InternalToken:  opts.InternalToken,
		PublicBaseURL:  strings.TrimRight(opts.PublicBaseURL, "/"),
		TrackingSecret: opts.TrackingSecret,
		UseMockGmail:   opts.UseMockGmail,
		ListenAddr:     opts.ListenAddr,
		Mux:            http.NewServeMux(),
	}
	if s.ListenAddr == "" {
		s.ListenAddr = ":8080"
	}
	if s.PublicBaseURL == "" {
		s.PublicBaseURL = "http://localhost:8080"
	}
	if opts.EncryptionKey != nil {
		s.Creds = NewDBCredentialStore(store.DB, opts.EncryptionKey)
	}
	if opts.GoogleClientID != "" && opts.GoogleClientSecret != "" {
		cfg := GoogleOAuthConfig{
			ClientID:     opts.GoogleClientID,
			ClientSecret: opts.GoogleClientSecret,
			RedirectURL:  opts.GoogleRedirectURL,
		}
		if cfg.RedirectURL == "" {
			cfg.RedirectURL = s.PublicBaseURL + "/api/v1/accounts/google/oauth/callback"
		}
		s.OAuth = cfg.OAuth2()
	}
	routing := &RoutingGWS{}
	if s.UseMockGmail {
		routing.Mock = NewMockGmail()
	}
	if s.Creds != nil && s.OAuth != nil {
		api := NewGoogleAPIProvider(s.Creds, s.OAuth)
		api.OnAuthFailure = func(email string, err error) {
			slog.Warn("google oauth failure; pausing account", "account", email, "error", err.Error())
			_, _ = engine.PauseAccount(store.DB, email)
			_ = SetHostedKV(store.DB, "account_oauth:"+strings.ToLower(email), "reconnect_required")
		}
		_ = s.hydrateAPIAccounts(api)
		routing.API = api
	}
	if storeCreds, ok := s.Creds.(*DBCredentialStore); ok && storeCreds != nil {
		msCfg := MicrosoftOAuthConfig(s.PublicBaseURL + "/api/v1/accounts/microsoft/oauth/callback")
		if msCfg.ClientID != "" && msCfg.ClientSecret != "" {
			ms := NewMicrosoftGraphProvider(storeCreds, msCfg)
			_ = s.hydrateMicrosoftAccounts(ms)
			routing.Microsoft = ms
		}
	}
	if !s.UseMockGmail {
		routing.CLI = engine.ConfiguredGWSClient(store.DB)
	}
	s.GWS = routing
	s.routes()
	return s, nil
}

func (s *Server) hydrateAPIAccounts(api *GoogleAPIProvider) error {
	rows, err := query(s.Store.DB, `SELECT id, email FROM accounts WHERE status = 'active' AND provider = ?`, engine.AccountProviderGWS)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return err
		}
		cred, err := s.Creds.GetGoogleCredentialByAccountID(context.Background(), id)
		if err != nil {
			return err
		}
		if cred != nil {
			api.RegisterAccount(email, id)
		}
	}
	return nil
}

func (s *Server) hydrateMicrosoftAccounts(api *MicrosoftGraphProvider) error {
	rows, err := query(s.Store.DB, `SELECT id, email FROM accounts WHERE status = 'active' AND provider = ?`, AccountProviderMicrosoft)
	if err != nil {
		return err
	}
	defer rows.Close()
	store, ok := s.Creds.(*DBCredentialStore)
	if !ok || store == nil {
		return nil
	}
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return err
		}
		cred, err := store.GetMicrosoftCredentialByAccountID(id)
		if err != nil {
			return err
		}
		if cred != nil {
			api.RegisterAccount(email, id)
		}
	}
	return nil
}

func (s *Server) Handler() http.Handler { return s.Mux }

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /internal/health", s.handleHealth)
	s.Mux.HandleFunc("POST /internal/tick", s.requireInternal(s.handleTick))

	s.Mux.HandleFunc("GET /api/v1/accounts", s.handleListAccounts)
	s.Mux.HandleFunc("GET /api/v1/accounts/{id}/status", s.handleAccountStatus)
	s.Mux.HandleFunc("POST /api/v1/accounts/google/oauth/start", s.handleOAuthStart)
	s.Mux.HandleFunc("GET /api/v1/accounts/google/oauth/callback", s.handleOAuthCallback)
	s.Mux.HandleFunc("POST /api/v1/accounts/microsoft/oauth/start", s.handleMicrosoftOAuthStart)
	s.Mux.HandleFunc("GET /api/v1/accounts/microsoft/oauth/callback", s.handleMicrosoftOAuthCallback)
	s.Mux.HandleFunc("POST /api/v1/accounts/smtp", s.handleAddSMTPAccount)
	s.Mux.HandleFunc("POST /api/v1/accounts/{id}/pause", s.handlePauseAccount)
	s.Mux.HandleFunc("POST /api/v1/accounts/{id}/resume", s.handleResumeAccount)
	s.Mux.HandleFunc("POST /api/v1/accounts/{id}/remove", s.handleRemoveAccount)

	s.Mux.HandleFunc("GET /api/v1/settings/capabilities", s.handleCapabilities)
	s.Mux.HandleFunc("GET /api/v1/integrations", s.handleListIntegrations)
	s.Mux.HandleFunc("POST /api/v1/integrations", s.handlePutIntegration)
	s.Mux.HandleFunc("DELETE /api/v1/integrations/{id}", s.handleDeleteIntegration)
	s.Mux.HandleFunc("POST /api/v1/integrations/test", s.handleTestIntegration)
	s.Mux.HandleFunc("POST /api/v1/integrations/{id}/test", s.handleTestIntegration)
	s.Mux.HandleFunc("POST /api/v1/integrations/apollo/search", s.handleApolloSearch)
	s.Mux.HandleFunc("POST /api/v1/integrations/search", s.handleConnectorSearch)
	s.Mux.HandleFunc("POST /api/v1/integrations/enrich", s.handleEnrichLead)
	s.Mux.HandleFunc("POST /api/v1/integrations/sheets/import", s.handleSheetsImport)
	s.Mux.HandleFunc("POST /api/v1/integrations/webhooks", s.handlePutWebhookEndpoint)
	s.Mux.HandleFunc("POST /api/v1/integrations/{provider}/ingest", s.handleWebhookIngest)

	s.Mux.HandleFunc("POST /api/v1/agent/draft-sequence", s.handleDraftSequence)
	s.Mux.HandleFunc("GET /api/v1/campaigns/{id}/preflight", s.handlePreflightCampaign)

	s.Mux.HandleFunc("GET /api/v1/campaigns", s.handleListCampaigns)
	s.Mux.HandleFunc("POST /api/v1/campaigns", s.handleCreateCampaign)
	s.Mux.HandleFunc("GET /api/v1/campaigns/{id}", s.handleGetCampaign)
	s.Mux.HandleFunc("POST /api/v1/campaigns/{id}/activate", s.handleActivateCampaign)
	s.Mux.HandleFunc("POST /api/v1/campaigns/{id}/pause", s.handlePauseCampaign)
	s.Mux.HandleFunc("POST /api/v1/campaigns/{id}/resume", s.handleResumeCampaign)
	s.Mux.HandleFunc("GET /api/v1/campaigns/{id}/preview", s.handlePreviewCampaign)
	s.Mux.HandleFunc("GET /api/v1/campaigns/{id}/stats", s.handleCampaignStats)
	s.Mux.HandleFunc("POST /api/v1/campaigns/{id}/leads", s.handleAddLeads)
	s.Mux.HandleFunc("DELETE /api/v1/campaigns/{id}/leads/{leadId}", s.handleRemoveLead)

	s.Mux.HandleFunc("GET /api/v1/inbox", s.handleInbox)
	s.Mux.HandleFunc("GET /api/v1/threads/{campaignId}/{leadId}", s.handleGetThread)
	s.Mux.HandleFunc("POST /api/v1/threads/{campaignId}/{leadId}/reply", s.handleThreadReply)
	s.Mux.HandleFunc("POST /api/v1/threads/{campaignId}/{leadId}/classify", s.handleClassify)
	s.Mux.HandleFunc("GET /api/v1/threads/{campaignId}/{leadId}/suggest-reply", s.handleSuggestReply)

	s.Mux.HandleFunc("POST /api/v1/leads/validate", s.handleValidateLeads)
	s.Mux.HandleFunc("GET /api/v1/leads", s.handleListLeads)
	s.Mux.HandleFunc("POST /api/v1/leads/{id}/blacklist", s.handleBlacklistLead)
	s.Mux.HandleFunc("POST /api/v1/leads/{id}/pause", s.handlePauseLead)

	s.Mux.HandleFunc("GET /api/v1/overview", s.handleOverview)
	s.Mux.HandleFunc("GET /api/v1/workspace", s.handleWorkspace)
	s.Mux.HandleFunc("POST /api/v1/internal/track/open", s.requireInternal(s.handleTrackOpen))
	s.Mux.HandleFunc("POST /api/v1/internal/track/click", s.requireInternal(s.handleTrackClick))
	s.Mux.HandleFunc("POST /api/v1/campaigns/{id}/open-tracking", s.handleSetOpenTracking)
}

func (s *Server) requireInternal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.InternalToken != "" && r.Header.Get("X-Internal-Token") != s.InternalToken {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid internal token")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK := s.Store.DB.Ping() == nil
	lastTick, _ := GetHostedKV(s.Store.DB, "last_successful_tick")
	lastPoll, _ := GetHostedKV(s.Store.DB, "last_successful_gmail_poll")
	var accounts int
	_ = queryRow(s.Store.DB, `SELECT COUNT(*) FROM accounts WHERE status = 'active'`).Scan(&accounts)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"status":                     "ok",
		"database":                   dbOK,
		"container":                  true,
		"scheduler":                  true,
		"connected_accounts":         accounts,
		"last_successful_tick":       lastTick,
		"last_successful_gmail_poll": lastPoll,
		"workspace_id":               s.WorkspaceID,
		"mock_gmail":                 s.UseMockGmail,
	}})
}

func (s *Server) handleTick(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lock, err := s.Store.AcquireTickLock(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "already running") {
			writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"status": "locked", "message": "tick already running"}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "lock_failed", err.Error())
		return
	}
	defer lock.Close()

	cfg := engine.TickConfig{
		DB:              s.Store.DB,
		GWS:             s.GWS,
		NoSleep:         true,
		MaxSendsPerTick: 1,
		Timezone:        time.UTC,
		SecretResolver:  s.secretResolver(),
		TrackingPixelForSend: func(campaignID, leadID, accountID, sendID int64, stepNumber int, params *internal.EmailParams) {
			s.attachTrackingPixel(campaignID, leadID, accountID, sendID, params)
		},
	}
	result, err := engine.Tick(cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "tick_failed", err.Error())
		return
	}
	s.SyncScheduledSheets()
	_ = TouchLastTick(s.Store.DB)
	if result.RepliesDetected > 0 || result.BouncesDetected > 0 || result.Sent > 0 {
		_ = SetHostedKV(s.Store.DB, "last_successful_gmail_poll", time.Now().UTC().Format(time.RFC3339))
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"status": "ok", "result": result}})
}

func (s *Server) attachTrackingPixel(campaignID, leadID, accountID, sendID int64, params *internal.EmailParams) {
	flag, _ := GetHostedKV(s.Store.DB, fmt.Sprintf("campaign_open_tracking:%d", campaignID))
	if flag != "1" && flag != "true" {
		return
	}
	token, err := NewOpaqueToken()
	if err != nil {
		return
	}
	sid := sendID
	_ = PutTrackingToken(s.Store.DB, TrackingToken{
		Token: token, Kind: "open", WorkspaceID: s.WorkspaceID,
		CampaignID: campaignID, LeadID: leadID, AccountID: accountID,
		ScheduledSendID: &sid,
	})
	params.TrackingPixelURL = OpenPixelURL(s.PublicBaseURL, token)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, envelope{Error: &apiErr{Code: code, Message: msg}})
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
