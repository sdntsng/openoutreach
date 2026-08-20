package hosted

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const (
	GmailSendScope     = gmail.GmailSendScope
	GmailReadonlyScope = gmail.GmailReadonlyScope
)

var GoogleOAuthScopes = []string{
	"openid",
	"email",
	GmailSendScope,
	GmailReadonlyScope,
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func (c GoogleOAuthConfig) OAuth2() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes:       GoogleOAuthScopes,
		Endpoint:     google.Endpoint,
	}
}

// GoogleAPIProvider implements internal.GWSClient via Gmail REST API.
type GoogleAPIProvider struct {
	Store          CredentialStore
	OAuth          *oauth2.Config
	OnAuthFailure  func(accountEmail string, err error)
	HTTPClient     *http.Client
	accountEmailToID map[string]int64
}

func NewGoogleAPIProvider(store CredentialStore, oauth *oauth2.Config) *GoogleAPIProvider {
	return &GoogleAPIProvider{
		Store:            store,
		OAuth:            oauth,
		HTTPClient:       http.DefaultClient,
		accountEmailToID: map[string]int64{},
	}
}

func (p *GoogleAPIProvider) RegisterAccount(email string, accountID int64) {
	p.accountEmailToID[strings.ToLower(email)] = accountID
}

func (p *GoogleAPIProvider) tokenSource(ctx context.Context, accountEmail string) (oauth2.TokenSource, error) {
	id, ok := p.accountEmailToID[strings.ToLower(accountEmail)]
	if !ok {
		return nil, fmt.Errorf("no hosted google credential mapping for %s", accountEmail)
	}
	cred, err := p.Store.GetGoogleCredentialByAccountID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, fmt.Errorf("no google credential for account %d", id)
	}
	tok := &oauth2.Token{
		AccessToken:  cred.AccessToken,
		RefreshToken: cred.RefreshToken,
		Expiry:       cred.TokenExpiry,
		TokenType:    "Bearer",
	}
	base := p.OAuth.TokenSource(ctx, tok)
	return oauth2.ReuseTokenSource(tok, &persistingTokenSource{
		base:    base,
		store:   p.Store,
		cred:    cred,
		account: accountEmail,
		onFail:  p.OnAuthFailure,
	}), nil
}

type persistingTokenSource struct {
	base    oauth2.TokenSource
	store   CredentialStore
	cred    *GoogleCredential
	account string
	onFail  func(string, error)
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		if p.onFail != nil {
			p.onFail(p.account, err)
		}
		return nil, err
	}
	if tok.AccessToken != "" && tok.AccessToken != p.cred.AccessToken {
		p.cred.AccessToken = tok.AccessToken
		p.cred.RefreshToken = tok.RefreshToken
		if p.cred.RefreshToken == "" {
			// keep existing refresh
		}
		p.cred.TokenExpiry = tok.Expiry
		_ = p.store.PutGoogleCredential(context.Background(), p.cred)
	}
	return tok, nil
}

func (p *GoogleAPIProvider) gmailService(ctx context.Context, account string) (*gmail.Service, error) {
	ts, err := p.tokenSource(ctx, account)
	if err != nil {
		return nil, err
	}
	return gmail.NewService(ctx, option.WithTokenSource(ts))
}

func (p *GoogleAPIProvider) SendEmail(account, to, rawMsg, threadID string) (string, string, error) {
	ctx := context.Background()
	svc, err := p.gmailService(ctx, account)
	if err != nil {
		return "", "", err
	}
	msg := &gmail.Message{Raw: rawMsg}
	if threadID != "" {
		msg.ThreadId = threadID
	}
	sent, err := svc.Users.Messages.Send("me", msg).Do()
	if err != nil {
		if p.OnAuthFailure != nil && isAuthError(err) {
			p.OnAuthFailure(account, err)
		}
		return "", "", err
	}
	if sent.Id == "" || sent.ThreadId == "" {
		return "", "", fmt.Errorf("gmail send returned empty ids")
	}
	return sent.Id, sent.ThreadId, nil
}

func (p *GoogleAPIProvider) ListMessages(account, query string, includeSpamTrash ...bool) ([]internal.GWSMessage, error) {
	ctx := context.Background()
	svc, err := p.gmailService(ctx, account)
	if err != nil {
		return nil, err
	}
	call := svc.Users.Messages.List("me").Q(query).MaxResults(100)
	if len(includeSpamTrash) > 0 && includeSpamTrash[0] {
		call = call.IncludeSpamTrash(true)
	}
	var out []internal.GWSMessage
	for {
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, m := range resp.Messages {
			full, err := p.GetMessage(account, m.Id)
			if err != nil {
				return nil, err
			}
			out = append(out, *full)
		}
		if resp.NextPageToken == "" {
			break
		}
		call.PageToken(resp.NextPageToken)
	}
	return out, nil
}

func (p *GoogleAPIProvider) GetMessage(account, msgID string) (*internal.GWSMessage, error) {
	ctx := context.Background()
	svc, err := p.gmailService(ctx, account)
	if err != nil {
		return nil, err
	}
	raw, err := svc.Users.Messages.Get("me", msgID).Format("full").Do()
	if err != nil {
		return nil, err
	}
	msg := gmailMessageFromAPI(raw)
	return &msg, nil
}

func (p *GoogleAPIProvider) GetThreadMessages(account, threadID string) ([]internal.GWSMessage, error) {
	ctx := context.Background()
	svc, err := p.gmailService(ctx, account)
	if err != nil {
		return nil, err
	}
	th, err := svc.Users.Threads.Get("me", threadID).Format("full").Do()
	if err != nil {
		return nil, err
	}
	out := make([]internal.GWSMessage, 0, len(th.Messages))
	for _, raw := range th.Messages {
		msg := gmailMessageFromAPI(raw)
		if msg.ThreadID == "" {
			msg.ThreadID = th.Id
		}
		out = append(out, msg)
	}
	return out, nil
}

func gmailMessageFromAPI(raw *gmail.Message) internal.GWSMessage {
	msg := internal.GWSMessage{
		ID:       raw.Id,
		ThreadID: raw.ThreadId,
		Snippet:  raw.Snippet,
		LabelIDs: raw.LabelIds,
		Headers:  map[string]string{},
	}
	if raw.InternalDate > 0 {
		msg.Date = time.UnixMilli(raw.InternalDate).UTC()
	}
	if raw.Payload != nil {
		msg.MimeType = raw.Payload.MimeType
		for _, h := range raw.Payload.Headers {
			msg.Headers[h.Name] = h.Value
			switch h.Name {
			case "From":
				msg.From = h.Value
			case "To":
				msg.To = h.Value
			case "Subject":
				msg.Subject = h.Value
			case "In-Reply-To":
				msg.InReplyTo = h.Value
			}
		}
		msg.TextBody, msg.HTMLBody = extractBodies(raw.Payload)
	}
	return msg
}

func extractBodies(part *gmail.MessagePart) (textBody, htmlBody string) {
	if part == nil {
		return "", ""
	}
	if part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			decoded, _ = base64.RawURLEncoding.DecodeString(part.Body.Data)
		}
		body := string(decoded)
		switch strings.ToLower(part.MimeType) {
		case "text/plain":
			textBody = body
		case "text/html":
			htmlBody = body
		}
	}
	for _, child := range part.Parts {
		t, h := extractBodies(child)
		if textBody == "" {
			textBody = t
		}
		if htmlBody == "" {
			htmlBody = h
		}
	}
	return textBody, htmlBody
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "invalid_grant") ||
		strings.Contains(s, "401") ||
		strings.Contains(s, "Unauthorized") ||
		strings.Contains(s, "Token has been expired or revoked")
}

// FetchGoogleUserEmail returns the primary email for an OAuth token.
func FetchGoogleUserEmail(ctx context.Context, client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var info struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", "", err
	}
	return info.Email, info.ID, nil
}

// RoutingGWS prefers GoogleAPIProvider when a mapping exists, else falls back to CLI gws.
type RoutingGWS struct {
	API  *GoogleAPIProvider
	CLI  internal.GWSClient
	Mock *MockGmail
}

func (r *RoutingGWS) SendEmail(account, to, rawMsg, threadID string) (string, string, error) {
	if r.Mock != nil {
		return r.Mock.SendEmail(account, to, rawMsg, threadID)
	}
	if r.API != nil {
		if _, ok := r.API.accountEmailToID[strings.ToLower(account)]; ok {
			return r.API.SendEmail(account, to, rawMsg, threadID)
		}
	}
	if r.CLI != nil {
		return r.CLI.SendEmail(account, to, rawMsg, threadID)
	}
	return "", "", fmt.Errorf("no gmail provider for %s", account)
}

func (r *RoutingGWS) ListMessages(account, query string, includeSpamTrash ...bool) ([]internal.GWSMessage, error) {
	if r.Mock != nil {
		return r.Mock.ListMessages(account, query, includeSpamTrash...)
	}
	if r.API != nil {
		if _, ok := r.API.accountEmailToID[strings.ToLower(account)]; ok {
			return r.API.ListMessages(account, query, includeSpamTrash...)
		}
	}
	if r.CLI != nil {
		return r.CLI.ListMessages(account, query, includeSpamTrash...)
	}
	return nil, fmt.Errorf("no gmail provider for %s", account)
}

func (r *RoutingGWS) GetMessage(account, msgID string) (*internal.GWSMessage, error) {
	if r.Mock != nil {
		return r.Mock.GetMessage(account, msgID)
	}
	if r.API != nil {
		if _, ok := r.API.accountEmailToID[strings.ToLower(account)]; ok {
			return r.API.GetMessage(account, msgID)
		}
	}
	if r.CLI != nil {
		return r.CLI.GetMessage(account, msgID)
	}
	return nil, fmt.Errorf("no gmail provider for %s", account)
}

func (r *RoutingGWS) GetThreadMessages(account, threadID string) ([]internal.GWSMessage, error) {
	if r.Mock != nil {
		return r.Mock.GetThreadMessages(account, threadID)
	}
	if r.API != nil {
		if _, ok := r.API.accountEmailToID[strings.ToLower(account)]; ok {
			return r.API.GetThreadMessages(account, threadID)
		}
	}
	if r.CLI != nil {
		return r.CLI.GetThreadMessages(account, threadID)
	}
	return nil, fmt.Errorf("no gmail provider for %s", account)
}
