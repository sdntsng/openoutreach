package hosted_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkspacePlaybookRoundTrip(t *testing.T) {
	srv, _ := setupHosted(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/workspace/playbook", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"company":""`) {
		t.Fatalf("empty playbook %d %s", rr.Code, rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/api/v1/workspace/playbook", strings.NewReader(
		`{"company":"Vinci","offer":"ad variants","geography":"United States","default_sequence_yaml":"name: x\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi\n    body: |\n      Hello\n"}`))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, put)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"company":"Vinci"`) {
		t.Fatalf("put playbook %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/workspace/playbook", nil))
	if !strings.Contains(rr.Body.String(), `"offer":"ad variants"`) {
		t.Fatalf("get after put: %s", rr.Body.String())
	}
}

func TestInboxBoxesAndCampaignInterested(t *testing.T) {
	srv, _ := setupHosted(t)
	id := seedCampaign(t, srv, "box-camp", "email,first_name,company\nada@acme.com,Ada,Acme\n")
	var leadID, acctID int64
	if err := srv.Store.DB.QueryRow(`SELECT id FROM leads WHERE email = 'ada@acme.com'`).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.DB.QueryRow(`SELECT id FROM accounts WHERE email = 'sender@example.com'`).Scan(&acctID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := srv.Store.DB.Exec(`
		INSERT INTO email_messages (campaign_id, lead_id, account_id, direction, type, subject, snippet, occurred_at)
		VALUES (?, ?, ?, 'inbound', 'reply', 'Re: Hello', 'Interested', ?)`,
		id, leadID, acctID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB.Exec(`
		INSERT INTO reply_classifications (workspace_id, campaign_id, lead_id, classification, confidence, reason)
		VALUES ('default', ?, ?, 'positive', 0.9, 'test')`, id, leadID); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/inbox?box=needs", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"needs_reply":true`) {
		t.Fatalf("needs inbox %d %s", rr.Code, rr.Body.String())
	}

	if _, err := srv.Store.DB.Exec(`
		INSERT INTO email_messages (campaign_id, lead_id, account_id, direction, type, subject, snippet, occurred_at)
		VALUES (?, ?, ?, 'outbound', 'sent', 'Re: Hello', 'Thanks', ?)`,
		id, leadID, acctID, now.Add(time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/inbox?box=needs", nil))
	if rr.Code != 200 || strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("needs after reply should be empty: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/inbox?box=sent", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "ada@acme.com") {
		t.Fatalf("sent box %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"interested":1`) {
		t.Fatalf("campaign interested %d %s", rr.Code, rr.Body.String())
	}
}

func TestSuppressionDomainCSV(t *testing.T) {
	srv, _ := setupHosted(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/suppressions", strings.NewReader(
		`{"kind":"domain","csv":"acme.com\nblocked.io\n"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"added":2`) {
		t.Fatalf("domain csv %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/suppressions", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "acme.com") || !strings.Contains(body, `"kind":"domain"`) {
		t.Fatalf("list domains: %s", body)
	}
}
