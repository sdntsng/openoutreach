package hosted_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/internal/hosted"
)

func TestCapabilitiesAndIntegrationsVault(t *testing.T) {
	srv, _ := setupHosted(t)

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/capabilities", nil))
	if rr.Code != 200 {
		t.Fatalf("capabilities %d %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Data hosted.Capabilities `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Data.Sending["smtp_imap"] {
		t.Fatal("expected smtp_imap enabled by default")
	}
	if !env.Data.Integrations["apollo"] {
		t.Fatal("expected apollo feature on")
	}
	if !env.Data.Integrations["warmup"] {
		t.Fatal("warmup should be on by default (set FEATURE_WARMUP=0 to hide)")
	}
	if !env.Data.Integrations["outbound"] {
		t.Fatal("outbound webhook feature should be on by default")
	}

	body := `{"provider":"apollo","name":"default","secret":"test-apollo-key-12345678"}`
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put integration %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "test-apollo-key-12345678") {
		t.Fatal("secret leaked in response")
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil))
	if rr.Code != 200 {
		t.Fatalf("list %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "apollo") {
		t.Fatalf("expected apollo in list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/agent/draft-sequence", strings.NewReader(`{"icp":"CMOs","offer":"analytics"}`)))
	if rr.Code != 200 {
		t.Fatalf("draft sequence %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sequence_yaml") {
		t.Fatalf("missing yaml: %s", rr.Body.String())
	}
}

func TestWebhookIngestPreview(t *testing.T) {
	srv, _ := setupHosted(t)
	payload := `{"email":"ada@acme.com","first_name":"Ada","company":"Acme"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/generic/ingest", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("expected lead preview: %s", rr.Body.String())
	}
}

func TestSheetsImportPreview(t *testing.T) {
	csvSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("email,first_name,company\nada@acme.com,Ada,Acme\n"))
	}))
	t.Cleanup(csvSrv.Close)

	srv, _ := setupHosted(t)
	body := `{"url":"` + csvSrv.URL + `"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/sheets/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("sheets import %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"preview":true`) {
		t.Fatalf("expected preview: %s", rr.Body.String())
	}
}

func TestWebhookHMACAndIdempotencyPreview(t *testing.T) {
	srv, _ := setupHosted(t)
	put := `{"provider":"generic","name":"default","hmac_secret":"whsec-test"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/webhooks", strings.NewReader(put))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put webhook %d %s", rr.Code, rr.Body.String())
	}

	payload := `{"email":"ada@acme.com","first_name":"Ada"}`
	rr = httptest.NewRecorder()
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/generic/ingest", strings.NewReader(payload))
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("X-OpenOutreach-Signature", "deadbeef")
	srv.Handler().ServeHTTP(rr, bad)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 bad hmac, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSMTPAccountRequiresVaultPassword(t *testing.T) {
	srv, _ := setupHosted(t)
	body := `{
		"email":"smtp@example.com",
		"smtp_host":"smtp.example.com",
		"smtp_port":587,
		"smtp_username":"smtp@example.com",
		"smtp_password":"s3cret-pass",
		"imap_host":"imap.example.com",
		"imap_port":993
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/smtp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("add smtp %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "s3cret-pass") {
		t.Fatal("smtp password leaked")
	}
}

func TestIntegrationCRUDTestAndRotation(t *testing.T) {
	srv, _ := setupHosted(t)
	put := func(secret string) *httptest.ResponseRecorder {
		body := `{"provider":"apollo","name":"seat-a","secret":"` + secret + `"}`
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}
	rr := put("first-secret-value-aaaa")
	if rr.Code != 200 {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "first-secret-value-aaaa") {
		t.Fatal("secret leaked on put")
	}
	var env struct {
		Data hosted.IntegrationCredential `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	id := env.Data.ID
	if id == 0 {
		t.Fatal("missing id")
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/"+strconv.FormatInt(id, 10)+"/test", strings.NewReader("{}"))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("test %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "first-secret-value-aaaa") {
		t.Fatal("secret leaked on test")
	}

	rr = put("rotated-secret-value-bbbb")
	if rr.Code != 200 {
		t.Fatalf("rotate %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "rotated-secret-value-bbbb") || strings.Contains(rr.Body.String(), "first-secret-value-aaaa") {
		t.Fatal("secret leaked on rotate")
	}

	rr = httptest.NewRecorder()
	del := httptest.NewRequest(http.MethodDelete, "/api/v1/integrations/"+strconv.FormatInt(id, 10), nil)
	srv.Handler().ServeHTTP(rr, del)
	if rr.Code != 200 {
		t.Fatalf("delete %d %s", rr.Code, rr.Body.String())
	}
}

func TestCapabilitiesHonorFeatureFlags(t *testing.T) {
	t.Setenv("FEATURE_SMTP_IMAP", "0")
	t.Setenv("FEATURE_APOLLO", "0")
	t.Setenv("FEATURE_SHEETS", "0")
	t.Setenv("AUTH_MODE", "hosted")
	t.Setenv("MCP_BEARER_TOKEN", "mcp-test-token")
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/capabilities", nil))
	if rr.Code != 200 {
		t.Fatalf("capabilities %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "mcp-test-token") {
		t.Fatal("mcp bearer leaked")
	}
	var env struct {
		Data hosted.Capabilities `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Sending["smtp_imap"] {
		t.Fatal("smtp should be disabled")
	}
	if env.Data.Integrations["apollo"] || env.Data.Integrations["sheets"] {
		t.Fatal("apollo/sheets should be disabled")
	}
	if env.Data.AuthMode != "hosted" {
		t.Fatalf("auth_mode=%s", env.Data.AuthMode)
	}
	if !env.Data.MCPConfigured {
		t.Fatal("expected mcp_configured true")
	}
	if env.Data.FeatureFlags["FEATURE_SHEETS"] != "0" {
		t.Fatalf("FEATURE_SHEETS flag=%s", env.Data.FeatureFlags["FEATURE_SHEETS"])
	}
}

func TestMicrosoftOAuthStartRequiresConfig(t *testing.T) {
	t.Setenv("MICROSOFT_CLIENT_ID", "")
	t.Setenv("MICROSOFT_CLIENT_SECRET", "")
	t.Setenv("FEATURE_MICROSOFT", "1")
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/microsoft/oauth/start", strings.NewReader("{}"))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without microsoft client, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestResendDisabledAndBounceWebhook(t *testing.T) {
	t.Setenv("FEATURE_RESEND", "0")
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/resend", strings.NewReader(`{"email":"from@acme.com","api_key":"re_test"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without FEATURE_RESEND, got %d %s", rr.Code, rr.Body.String())
	}

	t.Setenv("FEATURE_RESEND", "1")
	srv, _ = setupHosted(t)
	if _, err := srv.Store.DB.Exec(`INSERT INTO leads (email, first_name) VALUES ('bounce@acme.com', 'Bo')`); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	ev := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/resend/events", strings.NewReader(`{"type":"email.bounced","data":{"to":["bounce@acme.com"],"email_id":"msg_1"}}`))
	ev.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, ev)
	if rr.Code != 200 {
		t.Fatalf("bounce webhook %d %s", rr.Code, rr.Body.String())
	}
	var status string
	if err := srv.Store.DB.QueryRow(`SELECT global_status FROM leads WHERE email = 'bounce@acme.com'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "bounced" {
		t.Fatalf("status=%s", status)
	}
}

func TestWebhookCreateDraftFromCampaignName(t *testing.T) {
	srv, _ := setupHosted(t)
	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "sender@example.com", 30, "hosted-mock"); err != nil {
		t.Fatal(err)
	}
	payload := `{"email":"ada@acme.com","first_name":"Ada","campaign_name":"clay-inbound","create_campaign":true}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/clay/ingest", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"status":"active"`) {
		t.Fatal("ingest must not activate")
	}
	if !strings.Contains(rr.Body.String(), `"campaign_created":true`) {
		t.Fatalf("expected campaign_created: %s", rr.Body.String())
	}
	var status string
	if err := srv.Store.DB.QueryRow(`SELECT status FROM campaigns WHERE name = 'clay-inbound'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "draft" {
		t.Fatalf("status=%s", status)
	}

	rr = httptest.NewRecorder()
	again := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/clay/ingest", strings.NewReader(payload))
	again.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, again)
	if rr.Code != 200 {
		t.Fatalf("append %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"campaign_created":true`) {
		t.Fatalf("second ingest should append existing draft: %s", rr.Body.String())
	}
}

func TestAccountsWarmupAndReplyMode(t *testing.T) {
	t.Setenv("FEATURE_WARMUP", "1")
	srv, _ := setupHosted(t)
	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "sender@example.com", 30, "hosted-mock"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rr.Code != 200 {
		t.Fatalf("list %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"warmup_status":"unset"`) {
		t.Fatalf("expected unset warmup: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"reply_mode":"oauth"`) {
		t.Fatalf("expected oauth reply_mode: %s", rr.Body.String())
	}

	put := `{"provider":"warmup","name":"default","secret":"warmup-key-12345678"}`
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", strings.NewReader(put))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put warmup %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "warmup-key-12345678") {
		t.Fatal("warmup secret leaked")
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/capabilities", nil))
	if !strings.Contains(rr.Body.String(), `"warmup":true`) {
		t.Fatalf("expected FEATURE_WARMUP in capabilities: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rr.Code != 200 {
		t.Fatalf("list after warmup %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"warmup_status":"healthy"`) {
		t.Fatalf("expected healthy warmup badge: %s", rr.Body.String())
	}

	t.Setenv("FEATURE_RESEND", "1")
	rr = httptest.NewRecorder()
	resend := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/resend", strings.NewReader(`{"email":"from@acme.com","api_key":"re_test_key_xxxx"}`))
	resend.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, resend)
	if rr.Code != 200 {
		t.Fatalf("add resend %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if !strings.Contains(rr.Body.String(), `"reply_mode":"send_only"`) || !strings.Contains(rr.Body.String(), `"domain_verification":"dns_at_provider"`) {
		t.Fatalf("expected resend send_only/dns_at_provider: %s", rr.Body.String())
	}
}

func TestCFEmailDisabledAddSendAndInbound(t *testing.T) {
	t.Setenv("FEATURE_CF_EMAIL", "0")
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/cf-email", strings.NewReader(`{"email":"from@acme.com","api_token":"cf-secret-token-xxxx","account_id":"acc123"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without FEATURE_CF_EMAIL, got %d %s", rr.Code, rr.Body.String())
	}

	t.Setenv("FEATURE_CF_EMAIL", "1")
	srv, _ = setupHosted(t)

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/capabilities", nil))
	if !strings.Contains(rr.Body.String(), `"cf_email":true`) {
		t.Fatalf("expected cf_email capability: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	add := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/cf-email", strings.NewReader(`{"email":"from@acme.com","api_token":"cf-secret-token-xxxx","account_id":"acc123","daily_limit":40}`))
	add.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, add)
	if rr.Code != 200 {
		t.Fatalf("add cf-email %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "cf-secret-token-xxxx") {
		t.Fatal("api token leaked")
	}
	var added struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Data.ID == 0 {
		t.Fatalf("missing account id: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if !strings.Contains(rr.Body.String(), `"reply_mode":"email_routing"`) || !strings.Contains(rr.Body.String(), `"domain_verification":"dns_at_cloudflare"`) {
		t.Fatalf("expected cf_email email_routing/dns_at_cloudflare: %s", rr.Body.String())
	}

	var sawMessageID bool
	cfAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cf-secret-token-xxxx" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.URL.Path, "/accounts/acc123/email/sending/send") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if headers, ok := payload["headers"].(map[string]any); ok {
			if _, ok := headers["Message-ID"]; ok {
				sawMessageID = true
			}
		}
		if payload["to"] == "bounce@acme.com" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]any{"permanent_bounces": []string{"bounce@acme.com"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]any{"delivered": []string{"lead@acme.com"}},
		})
	}))
	t.Cleanup(cfAPI.Close)
	t.Setenv("CF_EMAIL_API_BASE", cfAPI.URL)

	raw := "From: from@acme.com\r\nTo: lead@acme.com\r\nSubject: Hi\r\nMessage-ID: <sent-1@acme.com>\r\n\r\nHello\r\n"
	p := &hosted.APIMailerProvider{
		Provider: hosted.AccountProviderCFEmail, APIKey: "cf-secret-token-xxxx",
		From: "from@acme.com", AccountID: "acc123", Client: cfAPI.Client(),
	}
	id, thread, err := p.SendEmail("from@acme.com", "lead@acme.com", raw, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "<sent-1@acme.com>" || thread != "<sent-1@acme.com>" {
		t.Fatalf("id=%q thread=%q", id, thread)
	}
	if sawMessageID {
		t.Fatal("Message-ID must not be sent as a Cloudflare custom header")
	}
	if _, _, err := p.SendEmail("from@acme.com", "bounce@acme.com", raw, ""); err == nil {
		t.Fatal("expected permanent bounce error")
	}

	if _, err := srv.Store.DB.Exec(`INSERT INTO campaigns (name, sequence_file) VALUES ('cf-in', 'seq.yml')`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`INSERT INTO leads (email, first_name) VALUES ('lead@acme.com', 'Ada')`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`INSERT INTO campaign_leads (campaign_id, lead_id, status) VALUES (1, 1, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, thread_id)
		VALUES (1, 1, ?, 'sent', 1, '<sent-1@acme.com>', '<sent-1@acme.com>')`, added.Data.ID); err != nil {
		t.Fatal(err)
	}

	noTok := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/cf-email/inbound", strings.NewReader(`{"from":"lead@acme.com","to":"from@acme.com","raw":"x"}`))
	noTok.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, noTok)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("inbound without token: %d %s", rr.Code, rr.Body.String())
	}

	mime := "From: Ada <lead@acme.com>\r\nTo: from@acme.com\r\nSubject: Re: Hi\r\nMessage-ID: <reply-1@acme.com>\r\nIn-Reply-To: <sent-1@acme.com>\r\n\r\nInterested\r\n"
	in := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/cf-email/inbound", strings.NewReader(`{"from":"Ada <lead@acme.com>","to":"from@acme.com","raw":`+strconv.Quote(mime)+`}`))
	in.Header.Set("Content-Type", "application/json")
	in.Header.Set("X-Internal-Token", "test-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, in)
	if rr.Code != 200 {
		t.Fatalf("inbound %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"replies":1`) {
		t.Fatalf("expected replies:1 %s", rr.Body.String())
	}

	if _, err := srv.Store.DB.Exec(`INSERT INTO leads (email, first_name) VALUES ('other@acme.com', 'Bo')`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`INSERT INTO campaign_leads (campaign_id, lead_id, status) VALUES (1, 2, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, thread_id)
		VALUES (1, 2, ?, 'sent', 1, '<sent-2@acme.com>', '<sent-2@acme.com>')`, added.Data.ID); err != nil {
		t.Fatal(err)
	}
	cfMIME := "From: other@acme.com\r\nTo: from@acme.com\r\nSubject: Re: Hi\r\nMessage-ID: <cf-gen@cloudflare>\r\nIn-Reply-To: <cf-provider-id@cloudflare>\r\n\r\nYes\r\n"
	in2 := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/cf-email/inbound", strings.NewReader(`{"from":"other@acme.com","to":"from@acme.com","raw":`+strconv.Quote(cfMIME)+`}`))
	in2.Header.Set("Content-Type", "application/json")
	in2.Header.Set("X-Internal-Token", "test-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, in2)
	if rr.Code != 200 {
		t.Fatalf("cf-generated inbound %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"replies":1`) {
		t.Fatalf("expected lead-thread fallback replies:1 %s", rr.Body.String())
	}
}
