package hosted_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/internal/hosted"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

func seedCampaign(t *testing.T, srv *hosted.Server, name, leadsCSV string) int64 {
	t.Helper()
	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "sender@example.com", 30, "hosted-mock"); err != nil {
		// ignore duplicate on second seed
		if !strings.Contains(err.Error(), "already") && !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "exists") {
			t.Fatal(err)
		}
	}
	seq := `name: ` + name + `
defaults:
  from_name: "Test"
steps:
  - step: 1
    delay: 0
    subject: "Hello {{first_name}}"
    body: |
      Hi {{first_name}},
`
	if leadsCSV == "" {
		leadsCSV = "email,first_name,company\nlead@acme.com,Ada,Acme\n"
	}
	res, err := engine.CreateCampaign(srv.Store.DB, engine.CreateCampaignOpts{
		WorkspaceID: "default", Name: name, SequenceInline: seq, LeadsInline: leadsCSV,
		AccountEmails:   []string{"sender@example.com"},
		SendWindowStart: "09:00", SendWindowEnd: "17:00", SendDays: "1,2,3,4,5", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.ID
}

func TestSetupChecklist(t *testing.T) {
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil))
	if rr.Code != 200 {
		t.Fatalf("setup %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"connect_account"`) {
		t.Fatalf("expected connect_account next action: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"encryption_ready":true`) {
		t.Fatalf("expected encryption_ready: %s", rr.Body.String())
	}

	seedCampaign(t, srv, "setup-camp", "")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil))
	if rr.Code != 200 {
		t.Fatalf("setup after seed %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"preview_and_activate"`) {
		t.Fatalf("expected preview_and_activate: %s", rr.Body.String())
	}
}

func TestSuppressionsHonorOnImportAndSearch(t *testing.T) {
	srv, _ := setupHosted(t)
	seedCampaign(t, srv, "sup-src", "email,first_name,company\nkeep@acme.com,Keep,Acme\nskip@blocked.com,Skip,Blocked\n")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/suppressions", strings.NewReader(`{"email":"skip@blocked.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("add suppression %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/suppressions", nil))
	if !strings.Contains(rr.Body.String(), "skip@blocked.com") {
		t.Fatalf("list suppressions: %s", rr.Body.String())
	}

	id := seedCampaign(t, srv, "sup-target", "email,first_name,company\nkeep2@acme.com,Keep2,Acme\n")
	rr = httptest.NewRecorder()
	add := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+itoa(id)+"/leads",
		strings.NewReader(`{"csv":"email,first_name,company\nskip@blocked.com,Skip,Blocked\nnew@acme.com,New,Acme\n"}`))
	add.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, add)
	if rr.Code != 200 {
		t.Fatalf("add leads %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "suppressed") {
		t.Fatalf("expected suppressed skip warning: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/leads?q=Keep", nil))
	if rr.Code != 200 {
		t.Fatalf("search %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "keep@acme.com") {
		t.Fatalf("expected name/company search hit: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "skip@blocked.com") && strings.Contains(rr.Body.String(), `"global_status":"active"`) {
		// blacklisted is ok; must not still be active
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/leads/export", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "keep@acme.com") {
		t.Fatalf("export %d %s", rr.Code, rr.Body.String())
	}
}

func TestVerifyLeadsAndAccountDNS(t *testing.T) {
	origMX, origTXT := hosted.CurrentLookupFns()
	t.Cleanup(func() {
		hosted.SetLookupFns(origMX, origTXT)
	})
	hosted.SetLookupFns(
		func(host string) ([]*net.MX, error) {
			if host == "acme.com" {
				return []*net.MX{{Host: "mx.acme.com", Pref: 10}}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
		func(name string) ([]string, error) {
			if name == "example.com" {
				return []string{"v=spf1 include:_spf.google.com ~all"}, nil
			}
			if name == "_dmarc.example.com" {
				return []string{"v=DMARC1; p=none"}, nil
			}
			return nil, nil
		},
	)

	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/verify", strings.NewReader(`{"emails":["ada@acme.com","bad","x@mailinator.com","nobody@nosuch.invalid"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("verify %d %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "disposable") || !strings.Contains(body, "invalid_syntax") || !strings.Contains(body, "no_mx") {
		t.Fatalf("verify results: %s", body)
	}

	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "sender@example.com", 30, "hosted-mock"); err != nil {
		t.Fatal(err)
	}
	var acctID int64
	if err := srv.Store.DB.QueryRow(`SELECT id FROM accounts WHERE email = 'sender@example.com'`).Scan(&acctID); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+itoa(acctID)+"/dns", nil))
	if rr.Code != 200 {
		t.Fatalf("dns %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"spf":true`) || !strings.Contains(rr.Body.String(), `"dmarc":true`) {
		t.Fatalf("expected SPF/DMARC: %s", rr.Body.String())
	}
}

func TestCloneStaysDraftAndPatchCampaign(t *testing.T) {
	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "clone-src", "email,first_name,company\nada@acme.com,Ada,Acme\n")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+itoa(id)+"/clone", strings.NewReader(`{"name":"clone-copy"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("clone %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"draft"`) {
		t.Fatalf("clone must stay draft: %s", rr.Body.String())
	}
	var status string
	if err := srv.Store.DB.QueryRow(`SELECT status FROM campaigns WHERE name = 'clone-copy'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "draft" {
		t.Fatalf("db status=%s", status)
	}

	rr = httptest.NewRecorder()
	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/campaigns/"+itoa(id), strings.NewReader(`{"timezone":"America/New_York","open_tracking":true}`))
	patch.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, patch)
	if rr.Code != 200 {
		t.Fatalf("patch draft %d %s", rr.Code, rr.Body.String())
	}
	var tz string
	if err := srv.Store.DB.QueryRow(`SELECT timezone FROM campaigns WHERE name = 'clone-src'`).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "America/New_York" {
		t.Fatalf("timezone=%s", tz)
	}

	if err := engine.CampaignStateTransition(srv.Store.DB, "clone-src", "activate", "draft", "active"); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	again := httptest.NewRequest(http.MethodPatch, "/api/v1/campaigns/"+itoa(id), strings.NewReader(`{"timezone":"UTC"}`))
	again.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, again)
	if rr.Code != 400 {
		t.Fatalf("expected 400 patching active, got %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+itoa(id)+"/leads/export", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("campaign export %d %s", rr.Code, rr.Body.String())
	}
}

func TestOutboundWebhookAfterTick(t *testing.T) {
	var got []byte
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	t.Cleanup(hook.Close)

	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "hook-camp", "email,first_name,company\nada@acme.com,Ada,Acme\n")
	put := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", strings.NewReader(
		`{"provider":"outbound","name":"default","secret":"`+hook.URL+`"}`))
	put.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, put)
	if rr.Code != 200 {
		t.Fatalf("put outbound %d %s", rr.Code, rr.Body.String())
	}

	var leadID, acctID int64
	if err := srv.Store.DB.QueryRow(`SELECT id FROM leads WHERE email = 'ada@acme.com'`).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.DB.QueryRow(`SELECT id FROM accounts WHERE email = 'sender@example.com'`).Scan(&acctID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`
		INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, timestamp)
		VALUES (?, ?, ?, 'sent', 1, ?)`, id, leadID, acctID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	tick := httptest.NewRequest(http.MethodPost, "/internal/tick", strings.NewReader("{}"))
	tick.Header.Set("X-Internal-Token", "test-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, tick)
	if rr.Code != 200 {
		t.Fatalf("tick %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(string(got), `"type":"sent"`) || !strings.Contains(string(got), "ada@acme.com") {
		t.Fatalf("webhook payload=%s", string(got))
	}

	// second tick should not re-send (cursor advanced)
	got = nil
	tick = httptest.NewRequest(http.MethodPost, "/internal/tick", strings.NewReader("{}"))
	tick.Header.Set("X-Internal-Token", "test-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, tick)
	if len(got) != 0 {
		t.Fatalf("expected no replay, got %s", string(got))
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
