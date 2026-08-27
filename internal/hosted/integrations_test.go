package hosted_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
