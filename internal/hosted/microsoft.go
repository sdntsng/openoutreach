package hosted

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
	"golang.org/x/oauth2"
)

const (
	AccountProviderMicrosoft = "microsoft"
	microsoftGraphScope      = "openid email offline_access Mail.Send Mail.Read User.Read"
)

// MicrosoftCredential mirrors google_credentials for M365.
type MicrosoftCredential struct {
	ID                 int64
	WorkspaceID        string
	AccountID          int64
	MicrosoftAccountID string
	RefreshToken       string
	AccessToken        string
	TokenExpiry        time.Time
	Scopes             string
}

func (s *DBCredentialStore) GetMicrosoftCredentialByAccountID(accountID int64) (*MicrosoftCredential, error) {
	var c MicrosoftCredential
	var encRefresh, encAccess string
	var expiryNull sql.NullTime
	err := queryRow(s.DB, `
		SELECT id, workspace_id, account_id, microsoft_account_id,
			encrypted_refresh_token, encrypted_access_token, token_expiry, scopes
		FROM microsoft_credentials WHERE account_id = ?`, accountID).Scan(
		&c.ID, &c.WorkspaceID, &c.AccountID, &c.MicrosoftAccountID,
		&encRefresh, &encAccess, &expiryNull, &c.Scopes,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refresh, err := Decrypt(s.Key, encRefresh)
	if err != nil {
		return nil, err
	}
	c.RefreshToken = string(refresh)
	if encAccess != "" {
		access, err := Decrypt(s.Key, encAccess)
		if err != nil {
			return nil, err
		}
		c.AccessToken = string(access)
	}
	if expiryNull.Valid {
		c.TokenExpiry = expiryNull.Time
	}
	return &c, nil
}

func (s *DBCredentialStore) PutMicrosoftCredential(c *MicrosoftCredential) error {
	encRefresh, err := Encrypt(s.Key, []byte(c.RefreshToken))
	if err != nil {
		return err
	}
	encAccess := ""
	if c.AccessToken != "" {
		encAccess, err = Encrypt(s.Key, []byte(c.AccessToken))
		if err != nil {
			return err
		}
	}
	ws := c.WorkspaceID
	if ws == "" {
		ws = "default"
	}
	var expiry any
	if !c.TokenExpiry.IsZero() {
		expiry = c.TokenExpiry.UTC()
	}
	_, err = exec(s.DB, `
		INSERT INTO microsoft_credentials (
			workspace_id, account_id, microsoft_account_id,
			encrypted_refresh_token, encrypted_access_token, token_expiry, scopes, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			microsoft_account_id = excluded.microsoft_account_id,
			encrypted_refresh_token = excluded.encrypted_refresh_token,
			encrypted_access_token = excluded.encrypted_access_token,
			token_expiry = excluded.token_expiry,
			scopes = excluded.scopes,
			updated_at = CURRENT_TIMESTAMP`,
		ws, c.AccountID, c.MicrosoftAccountID, encRefresh, encAccess, expiry, c.Scopes)
	return err
}

// MicrosoftOAuthConfig builds the Azure AD oauth2 config.
func MicrosoftOAuthConfig(redirectURL string) *oauth2.Config {
	clientID := strings.TrimSpace(os.Getenv("MICROSOFT_CLIENT_ID"))
	secret := strings.TrimSpace(os.Getenv("MICROSOFT_CLIENT_SECRET"))
	tenant := strings.TrimSpace(os.Getenv("MICROSOFT_TENANT_ID"))
	if tenant == "" {
		tenant = "common"
	}
	if redirectURL == "" {
		redirectURL = strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/") + "/api/v1/accounts/microsoft/oauth/callback"
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		RedirectURL:  redirectURL,
		Scopes:       strings.Split(microsoftGraphScope, " "),
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token",
		},
	}
}

// MicrosoftGraphProvider implements internal.GWSClient via Microsoft Graph.
type MicrosoftGraphProvider struct {
	Store  *DBCredentialStore
	OAuth  *oauth2.Config
	mu     sync.Mutex
	emails map[string]int64
	client *http.Client
}

func NewMicrosoftGraphProvider(store *DBCredentialStore, cfg *oauth2.Config) *MicrosoftGraphProvider {
	return &MicrosoftGraphProvider{
		Store:  store,
		OAuth:  cfg,
		emails: map[string]int64{},
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *MicrosoftGraphProvider) RegisterAccount(email string, accountID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emails[strings.ToLower(email)] = accountID
}

func (p *MicrosoftGraphProvider) tokenFor(email string) (string, error) {
	p.mu.Lock()
	id, ok := p.emails[strings.ToLower(email)]
	p.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("microsoft account %s not registered", email)
	}
	cred, err := p.Store.GetMicrosoftCredentialByAccountID(id)
	if err != nil || cred == nil {
		return "", fmt.Errorf("microsoft credentials missing for %s", email)
	}
	tok := &oauth2.Token{AccessToken: cred.AccessToken, RefreshToken: cred.RefreshToken, Expiry: cred.TokenExpiry}
	src := p.OAuth.TokenSource(context.Background(), tok)
	fresh, err := src.Token()
	if err != nil {
		return "", err
	}
	if fresh.AccessToken != cred.AccessToken || fresh.RefreshToken != cred.RefreshToken {
		cred.AccessToken = fresh.AccessToken
		if fresh.RefreshToken != "" {
			cred.RefreshToken = fresh.RefreshToken
		}
		cred.TokenExpiry = fresh.Expiry
		_ = p.Store.PutMicrosoftCredential(cred)
	}
	return fresh.AccessToken, nil
}

func (p *MicrosoftGraphProvider) SendEmail(account, to, rawMsg, threadID string) (string, string, error) {
	token, err := p.tokenFor(account)
	if err != nil {
		return "", "", err
	}
	subject, bodyText := parseRawMessage(rawMsg)
	message := map[string]any{
		"message": map[string]any{
			"subject": subject,
			"body": map[string]any{
				"contentType": "Text",
				"content":     bodyText,
			},
			"toRecipients": []map[string]any{
				{"emailAddress": map[string]string{"address": to}},
			},
		},
		"saveToSentItems": true,
	}
	raw, _ := json.Marshal(message)
	req, err := http.NewRequest(http.MethodPost, "https://graph.microsoft.com/v1.0/me/sendMail", bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return "", "", fmt.Errorf("graph sendMail HTTP %d: %s", res.StatusCode, string(b))
	}
	msgID := fmt.Sprintf("msgraph-%d", time.Now().UnixNano())
	if threadID == "" {
		threadID = msgID
	}
	return msgID, threadID, nil
}

func parseRawMessage(rawMsg string) (subject, body string) {
	msg, err := mail.ReadMessage(strings.NewReader(rawMsg))
	if err != nil {
		return "(no subject)", rawMsg
	}
	subject = msg.Header.Get("Subject")
	b, _ := io.ReadAll(msg.Body)
	body = string(b)
	if subject == "" {
		subject = "(no subject)"
	}
	return subject, body
}

func (p *MicrosoftGraphProvider) ListMessages(account, query string, includeSpamTrash ...bool) ([]internal.GWSMessage, error) {
	token, err := p.tokenFor(account)
	if err != nil {
		return nil, err
	}
	u := "https://graph.microsoft.com/v1.0/me/messages?$top=25&$orderby=receivedDateTime%20desc"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("graph list messages HTTP %d: %s", res.StatusCode, truncateBytes(body, 200))
	}
	var parsed struct {
		Value []struct {
			ID               string `json:"id"`
			Subject          string `json:"subject"`
			BodyPreview      string `json:"bodyPreview"`
			ConversationID   string `json:"conversationId"`
			ReceivedDateTime string `json:"receivedDateTime"`
			From             struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
			} `json:"from"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]internal.GWSMessage, 0, len(parsed.Value))
	for _, m := range parsed.Value {
		out = append(out, internal.GWSMessage{
			ID: m.ID, ThreadID: m.ConversationID, Subject: m.Subject,
			Snippet: m.BodyPreview, From: m.From.EmailAddress.Address,
			Headers: map[string]string{"Subject": m.Subject},
		})
	}
	return out, nil
}

func (p *MicrosoftGraphProvider) GetMessage(account, msgID string) (*internal.GWSMessage, error) {
	msgs, err := p.ListMessages(account, "")
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		if msgs[i].ID == msgID {
			return &msgs[i], nil
		}
	}
	return nil, fmt.Errorf("message not found")
}

func (p *MicrosoftGraphProvider) GetThreadMessages(account, threadID string) ([]internal.GWSMessage, error) {
	all, err := p.ListMessages(account, "")
	if err != nil {
		return nil, err
	}
	var out []internal.GWSMessage
	for _, m := range all {
		if m.ThreadID == threadID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *Server) handleMicrosoftOAuthStart(w http.ResponseWriter, r *http.Request) {
	caps := BuildCapabilities(s.WorkspaceID, s.PublicBaseURL, s.encKey() != nil, s.OAuth != nil)
	if !caps.Sending["microsoft"] || !caps.MicrosoftReady {
		writeErr(w, http.StatusForbidden, "feature_disabled", "Microsoft OAuth is not configured (MICROSOFT_CLIENT_ID/SECRET + FEATURE_MICROSOFT)")
		return
	}
	cfg := MicrosoftOAuthConfig(s.PublicBaseURL + "/api/v1/accounts/microsoft/oauth/callback")
	state := randomState()
	_, _ = exec(s.Store.DB, `INSERT INTO oauth_states (state, workspace_id) VALUES (?, ?)`, state, s.workspaceFromRequest(r))
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"authorize_url": cfg.AuthCodeURL(state, oauth2.AccessTypeOffline),
	}})
}

func (s *Server) handleMicrosoftOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	var ws string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM oauth_states WHERE state = ?`, state).Scan(&ws); err != nil {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	_, _ = exec(s.Store.DB, `DELETE FROM oauth_states WHERE state = ?`, state)
	cfg := MicrosoftOAuthConfig(s.PublicBaseURL + "/api/v1/accounts/microsoft/oauth/callback")
	tok, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}
	email, msID, err := fetchMicrosoftProfile(tok.AccessToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	acct, err := engine.AddAccountInWorkspace(s.Store.DB, ws, email, 50, "microsoft-graph")
	if err != nil {
		var id int64
		if qerr := queryRow(s.Store.DB, `SELECT id FROM accounts WHERE email = ?`, email).Scan(&id); qerr != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		acct = &internal.AddAccountResult{ID: id, Email: email, WorkspaceID: ws}
		_, _ = exec(s.Store.DB, `UPDATE accounts SET provider = ?, status = 'active' WHERE id = ?`, AccountProviderMicrosoft, id)
	} else {
		_, _ = exec(s.Store.DB, `UPDATE accounts SET provider = ? WHERE id = ?`, AccountProviderMicrosoft, acct.ID)
	}
	store, ok := s.Creds.(*DBCredentialStore)
	if !ok || store == nil {
		http.Error(w, "vault unconfigured", http.StatusServiceUnavailable)
		return
	}
	cred := &MicrosoftCredential{
		WorkspaceID: ws, AccountID: acct.ID, MicrosoftAccountID: msID,
		RefreshToken: tok.RefreshToken, AccessToken: tok.AccessToken,
		TokenExpiry: tok.Expiry, Scopes: microsoftGraphScope,
	}
	if err := store.PutMicrosoftCredential(cred); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.GWS != nil && s.GWS.Microsoft != nil {
		s.GWS.Microsoft.RegisterAccount(email, acct.ID)
	}
	http.Redirect(w, r, "/accounts?connected=1", http.StatusFound)
}

func fetchMicrosoftProfile(accessToken string) (email, id string, err error) {
	req, err := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", "", fmt.Errorf("graph /me HTTP %d", res.StatusCode)
	}
	var profile struct {
		ID                string `json:"id"`
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", "", err
	}
	email = profile.Mail
	if email == "" {
		email = profile.UserPrincipalName
	}
	return email, profile.ID, nil
}

// APIMailerProvider sends via Resend (send-only). SES uses SMTP_IMAP in practice.
type APIMailerProvider struct {
	Provider string
	APIKey   string
	From     string
	Client   *http.Client
}

func (p *APIMailerProvider) SendEmail(account, to, rawMsg, threadID string) (string, string, error) {
	subject, bodyText := parseRawMessage(rawMsg)
	from := account
	if from == "" {
		from = p.From
	}
	switch p.Provider {
	case "resend":
		payload := map[string]any{
			"from": from, "to": []string{to}, "subject": subject, "text": bodyText,
		}
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(raw))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
		req.Header.Set("Content-Type", "application/json")
		client := p.Client
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}
		res, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			return "", "", fmt.Errorf("resend HTTP %d: %s", res.StatusCode, truncateBytes(body, 200))
		}
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &out)
		if out.ID == "" {
			out.ID = fmt.Sprintf("resend-%d", time.Now().UnixNano())
		}
		if threadID == "" {
			threadID = out.ID
		}
		return out.ID, threadID, nil
	case "ses":
		return "", "", fmt.Errorf("SES: use SMTP/IMAP account against the SES SMTP endpoint (SigV4 HTTP not wired); see docs/INTEGRATIONS.md")
	default:
		return "", "", fmt.Errorf("unknown api mailer %s", p.Provider)
	}
}

func (p *APIMailerProvider) ListMessages(string, string, ...bool) ([]internal.GWSMessage, error) {
	return nil, fmt.Errorf("api mailer is send-only; configure bounce webhook")
}
func (p *APIMailerProvider) GetMessage(string, string) (*internal.GWSMessage, error) {
	return nil, fmt.Errorf("api mailer is send-only")
}
func (p *APIMailerProvider) GetThreadMessages(string, string) ([]internal.GWSMessage, error) {
	return nil, fmt.Errorf("api mailer is send-only")
}
