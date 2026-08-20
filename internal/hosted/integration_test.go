package hosted_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
	"github.com/andersmyrmel/cold-cli/internal/hosted"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

func setupHosted(t *testing.T) (*hosted.Server, *hosted.MockGmail) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("COLD_CLI_DATABASE_URL", "")
	// force sqlite via empty DATABASE_URL and custom data dir through HOME
	_ = os.MkdirAll(filepath.Join(dir, ".cold-cli"), 0o700)

	store, err := engine.OpenStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := hosted.BootstrapHostedSchema(store.DB); err != nil {
		t.Fatal(err)
	}
	key, _ := hosted.DeriveKey("test-encryption-key")
	srv, err := hosted.NewServer(store, hosted.ServerOpts{
		WorkspaceID:   "default",
		InternalToken: "test-token",
		PublicBaseURL: "http://localhost:8080",
		UseMockGmail:  true,
		EncryptionKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, srv.GWS.Mock
}

func TestHealthAndTickLock(t *testing.T) {
	srv, _ := setupHosted(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("health status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestCampaignActivateGateAndReplyCancel(t *testing.T) {
	srv, mock := setupHosted(t)
	db := srv.Store.DB

	acct, err := internal.AddAccountInWorkspace(db, "default", "sender@example.com", 30, "hosted-mock")
	if err != nil {
		t.Fatal(err)
	}
	mock.RevokedAccounts = map[string]bool{}

	seq := `name: test
defaults:
  from_name: "Test"
steps:
  - step: 1
    delay: 0
    subject: "Hello {{first_name}}"
    body: |
      Hi {{first_name}},
  - step: 2
    delay: 0
    body: |
      Bump {{first_name}}
`
	leads := "email,first_name,company\nlead@acme.com,Ada,Acme\n"
	res, err := engine.CreateCampaign(db, engine.CreateCampaignOpts{
		WorkspaceID: "default", Name: "test-camp", SequenceInline: seq, LeadsInline: leads,
		AccountEmails: []string{"sender@example.com"},
		SendWindowStart: "00:00", SendWindowEnd: "23:59", SendDays: "0,1,2,3,4,5,6", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "draft" {
		t.Fatalf("expected draft, got %s", res.Status)
	}

	// Force pending sends due now
	_, _ = db.Exec(`UPDATE scheduled_sends SET send_at = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))

	// Tick while draft — should not send
	cfg := engine.TickConfig{DB: db, GWS: mock, NoSleep: true, MaxSendsPerTick: 1, Now: time.Now().UTC()}
	result, err := engine.Tick(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 0 {
		t.Fatalf("draft should not send, sent=%d", result.Sent)
	}

	if err := engine.CampaignStateTransition(db, "test-camp", "activate", "draft", "active"); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE scheduled_sends SET send_at = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))

	result, err = engine.Tick(engine.TickConfig{DB: db, GWS: mock, NoSleep: true, MaxSendsPerTick: 1, Now: time.Now().UTC(), SendNow: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 {
		t.Fatalf("expected 1 send, got %d failed=%d", result.Sent, result.Failed)
	}
	if len(mock.Sent) != 1 {
		t.Fatalf("mock sent %d", len(mock.Sent))
	}
	threadID := mock.Sent[0].ThreadID

	var storedMsgID string
	_ = db.QueryRow(`SELECT message_id FROM scheduled_sends WHERE step_number = 1 AND status = 'sent'`).Scan(&storedMsgID)
	mock.InjectReply("lead@acme.com", "sender@example.com", "Re: Hello Ada", storedMsgID, threadID, "Interested!")

	result, err = engine.Tick(engine.TickConfig{DB: db, GWS: mock, NoSleep: true, MaxSendsPerTick: 1, Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if result.RepliesDetected < 1 {
		t.Fatalf("expected reply detection, got %+v", result)
	}
	var pending int
	_ = db.QueryRow(`SELECT COUNT(*) FROM scheduled_sends WHERE status = 'pending'`).Scan(&pending)
	if pending != 0 {
		t.Fatalf("expected remaining sends cancelled, pending=%d", pending)
	}
	_ = acct
	_ = context.Background()
}

func TestWorkspaceIsolation(t *testing.T) {
	srv, _ := setupHosted(t)
	db := srv.Store.DB
	_, err := internal.AddAccountInWorkspace(db, "ws-a", "a@example.com", 30, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.CreateDraftCampaign(db, engine.CreateDraftCampaignOpts{
		WorkspaceID: "ws-a", Name: "secret-camp", AccountEmails: []string{"a@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = internal.ResolveCampaignNameInWorkspace(db, "ws-b", "secret-camp")
	if err == nil {
		t.Fatal("expected cross-workspace miss")
	}
}

func TestMaxSendsPerTick(t *testing.T) {
	srv, mock := setupHosted(t)
	db := srv.Store.DB
	_, err := internal.AddAccountInWorkspace(db, "default", "sender2@example.com", 50, "hosted-mock")
	if err != nil {
		t.Fatal(err)
	}
	seq := `name: t
defaults:
  from_name: "T"
steps:
  - step: 1
    delay: 0
    subject: "Hi {{first_name}}"
    body: "Hello {{first_name}}"
`
	leads := "email,first_name\none@x.com,One\ntwo@x.com,Two\n"
	_, err = engine.CreateCampaign(db, engine.CreateCampaignOpts{
		WorkspaceID: "default", Name: "burst", SequenceInline: seq, LeadsInline: leads,
		AccountEmails: []string{"sender2@example.com"},
		SendWindowStart: "00:00", SendWindowEnd: "23:59", SendDays: "0,1,2,3,4,5,6", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = engine.CampaignStateTransition(db, "burst", "activate", "draft", "active")
	_, _ = db.Exec(`UPDATE scheduled_sends SET send_at = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
	result, err := engine.Tick(engine.TickConfig{DB: db, GWS: mock, NoSleep: true, MaxSendsPerTick: 1, Now: time.Now().UTC(), SendNow: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 {
		t.Fatalf("MaxSendsPerTick=1 expected 1 sent, got %d", result.Sent)
	}
}

func TestActivateRequiresConfirm(t *testing.T) {
	srv, _ := setupHosted(t)
	db := srv.Store.DB
	_, err := internal.AddAccountInWorkspace(db, "default", "act@example.com", 30, "hosted-mock")
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.CreateDraftCampaign(db, engine.CreateDraftCampaignOpts{
		WorkspaceID: "default", Name: "needs-confirm", AccountEmails: []string{"act@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/needs-confirm/activate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm, got %d %s", rr.Code, rr.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/needs-confirm/activate", strings.NewReader(`{"confirm":true}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 with confirm, got %d %s", rr2.Code, rr2.Body.String())
	}
}
