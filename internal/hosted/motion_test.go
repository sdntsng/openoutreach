package hosted_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
)

func TestClassifyHotQueuesHandoff(t *testing.T) {
	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "mark-hot", "email,first_name,company\nada@acme.com,Ada,Acme\n")
	var leadID int64
	if err := srv.Store.DB.QueryRow(`SELECT id FROM leads WHERE email = 'ada@acme.com'`).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+itoa(id)+"/"+itoa(leadID)+"/classify",
		strings.NewReader(`{"classification":"hot"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("classify %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"failed"`) || !strings.Contains(rr.Body.String(), `"kind":"interested"`) {
		t.Fatalf("expected failed interested handoff on classify: %s", rr.Body.String())
	}
}

func TestCreateDraftWithoutMailboxAndReview(t *testing.T) {
	srv, _ := setupHosted(t)
	body := `{
		"name":"small-list",
		"sequence_yaml":"name: outreach\ndefaults:\n  from_name: You\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi {{first_name}}\n    body: |\n      Hello {{first_name}} at {{company}}\n",
		"leads_csv":"email,first_name,company,title\nada@acme.com,Ada,Acme,CTO\n"
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"shortlist_added":1`) {
		t.Fatalf("expected shortlist: %s", rr.Body.String())
	}

	var created struct {
		Data struct {
			CampaignID int64 `json:"campaign_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := itoa(created.Data.CampaignID)

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+id+"/review", nil))
	if rr.Code != 200 {
		t.Fatalf("review %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Hello Ada at Acme") {
		t.Fatalf("expected rendered body, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"preview_options"`) || !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("expected shortlist in preview options: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"ready":true`) {
		t.Fatalf("empty enrollment should not be ready: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	act := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+id+"/activate", strings.NewReader(`{"confirm":true}`))
	act.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, act)
	if rr.Code != 400 || !strings.Contains(rr.Body.String(), "not_ready") {
		t.Fatalf("activate empty should fail: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/shortlist?campaign_id="+id, nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("shortlist %d %s", rr.Code, rr.Body.String())
	}
}

func TestShortlistApproveEnrollActivate(t *testing.T) {
	srv, _ := setupHosted(t)
	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "sender@example.com", 30, "hosted-mock"); err != nil {
		t.Fatal(err)
	}
	body := `{
		"name":"enroll-camp",
		"accounts":["sender@example.com"],
		"sequence_yaml":"name: outreach\ndefaults:\n  from_name: You\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi {{first_name}}\n    body: |\n      Hello {{first_name}}\n",
		"leads_csv":"email,first_name,company\nada@acme.com,Ada,Acme\n"
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Data struct {
			CampaignID int64 `json:"campaign_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	id := itoa(created.Data.CampaignID)

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/shortlist?campaign_id="+id, nil))
	var listed struct {
		Data struct {
			Candidates []struct {
				ID int64 `json:"id"`
			} `json:"candidates"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &listed)
	if len(listed.Data.Candidates) != 1 {
		t.Fatalf("expected 1 candidate: %s", rr.Body.String())
	}
	sid := itoa(listed.Data.Candidates[0].ID)
	rr = httptest.NewRecorder()
	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/shortlist/"+sid, strings.NewReader(`{"status":"approved"}`))
	patch.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, patch)
	if rr.Code != 200 {
		t.Fatalf("approve %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	enroll := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+id+"/enroll", strings.NewReader(`{"all_approved":true}`))
	enroll.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, enroll)
	if rr.Code != 200 {
		t.Fatalf("enroll %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	act := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+id+"/activate", strings.NewReader(`{"confirm":true}`))
	act.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, act)
	if rr.Code != 200 {
		t.Fatalf("activate %d %s", rr.Code, rr.Body.String())
	}
}

func TestSequencePreviewRendersYAML(t *testing.T) {
	srv, _ := setupHosted(t)
	body := `{"sequence_yaml":"name: t\ndefaults:\n  from_name: You\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi {{first_name}}\n    body: |\n      Hello {{company}}\n"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sequences/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Hello Acme") {
		t.Fatalf("preview %d %s", rr.Code, rr.Body.String())
	}
}

func TestOutboundDeliveryDoesNotSkipFailures(t *testing.T) {
	var codes []int
	n := 0
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		b, _ := io.ReadAll(r.Body)
		codes = append(codes, n)
		if n == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write(b)
	}))
	t.Cleanup(hook.Close)

	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "fail-hook", "email,first_name,company\nada@acme.com,Ada,Acme\n")
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
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := srv.Store.DB.Exec(`
		INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, timestamp)
		VALUES (?, ?, ?, 'sent', 1, ?)`, id, leadID, acctID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`
		INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, timestamp)
		VALUES (?, ?, ?, 'reply', 1, ?)`, id, leadID, acctID, now); err != nil {
		t.Fatal(err)
	}

	tick := func() {
		req := httptest.NewRequest(http.MethodPost, "/internal/tick", strings.NewReader("{}"))
		req.Header.Set("X-Internal-Token", "test-token")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("tick %d %s", rr.Code, rr.Body.String())
		}
	}
	tick()
	var failed, success int
	_ = srv.Store.DB.QueryRow(`SELECT COUNT(*) FROM outbound_deliveries WHERE status = 'failed'`).Scan(&failed)
	_ = srv.Store.DB.QueryRow(`SELECT COUNT(*) FROM outbound_deliveries WHERE status = 'success'`).Scan(&success)
	if failed != 1 || success != 1 {
		t.Fatalf("after first tick failed=%d success=%d codes=%v", failed, success, codes)
	}
	tick()
	_ = srv.Store.DB.QueryRow(`SELECT COUNT(*) FROM outbound_deliveries WHERE status = 'failed'`).Scan(&failed)
	_ = srv.Store.DB.QueryRow(`SELECT COUNT(*) FROM outbound_deliveries WHERE status = 'success'`).Scan(&success)
	if failed != 0 || success != 2 {
		t.Fatalf("retry should deliver the failure failed=%d success=%d", failed, success)
	}
}

func TestHandoffPersistsFailureWithoutWebhook(t *testing.T) {
	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "hot-camp", "email,first_name,company\nada@acme.com,Ada,Acme\n")
	var leadID int64
	if err := srv.Store.DB.QueryRow(`SELECT id FROM leads WHERE email = 'ada@acme.com'`).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+itoa(id)+"/"+itoa(leadID)+"/handoff",
		strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("handoff %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"failed"`) {
		t.Fatalf("expected failed handoff: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"handoffs_failed":1`) {
		t.Fatalf("overview %d %s", rr.Code, rr.Body.String())
	}
}

func TestSuggestReplyDoesNotClaimPlaybookWhenEmpty(t *testing.T) {
	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "sug", "email,first_name,company\nada@acme.com,Ada,Acme\n")
	var leadID int64
	_ = srv.Store.DB.QueryRow(`SELECT id FROM leads WHERE email = 'ada@acme.com'`).Scan(&leadID)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+itoa(id)+"/"+itoa(leadID)+"/suggest-reply", nil))
	if rr.Code != 200 {
		t.Fatalf("suggest %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"used_playbook":false`) {
		t.Fatalf("empty playbook should not claim facts: %s", rr.Body.String())
	}
}

func TestWebhookCreateDraftWithoutMailbox(t *testing.T) {
	srv, _ := setupHosted(t)
	payload := `{"email":"ada@acme.com","first_name":"Ada","campaign_name":"no-mailbox","create_campaign":true}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/clay/ingest", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"campaign_created":true`) {
		t.Fatalf("expected campaign_created: %s", rr.Body.String())
	}
	var created struct {
		Data struct {
			CampaignID int64 `json:"campaign_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/shortlist?campaign_id="+itoa(created.Data.CampaignID), nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("shortlist %d %s", rr.Code, rr.Body.String())
	}
}

func TestAccountHealthSavedWhenUnverified(t *testing.T) {
	srv, _ := setupHosted(t)
	if _, err := internal.AddAccountInWorkspace(srv.Store.DB, "default", "plain@example.com", 30, "smtp"); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"oauth_health":"saved"`) {
		t.Fatalf("expected saved health: %s", rr.Body.String())
	}
}
