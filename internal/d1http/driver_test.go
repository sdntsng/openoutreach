package d1http

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDriverQueryAndExec(t *testing.T) {
	var lastMode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/d1" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("X-Internal-Token") != "tok" {
			t.Errorf("missing token")
		}
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		lastMode, _ = req["mode"].(string)
		switch lastMode {
		case "exec":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta": map[string]any{"changes": 1, "last_row_id": 7},
			})
		case "query":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"columns": []string{"id", "email"},
				"rows":    [][]any{{float64(7), "a@b.com"}},
				"meta":    map[string]any{"changes": 0, "last_row_id": 7},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad mode"})
		}
	}))
	defer srv.Close()

	db, err := sql.Open("d1http", FormatDSN(srv.URL, "tok"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	res, err := db.Exec("INSERT INTO leads (email) VALUES (?)", "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if id != 7 {
		t.Fatalf("last insert id %d", id)
	}

	var gotID int64
	var email string
	if err := db.QueryRow("SELECT id, email FROM leads WHERE id = ?", 7).Scan(&gotID, &email); err != nil {
		t.Fatal(err)
	}
	if gotID != 7 || email != "a@b.com" {
		t.Fatalf("got %d %s", gotID, email)
	}
}

func TestExecWhileRowsOpenRequiresMultipleConns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		mode, _ := req["mode"].(string)
		switch mode {
		case "query":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"columns": []string{"id"},
				"rows":    [][]any{{float64(1)}},
			})
		case "exec":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta": map[string]any{"changes": 1},
			})
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()

	db, err := sql.Open("d1http", FormatDSN(srv.URL, "tok"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	rows, err := db.Query("SELECT id FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("expected row")
	}

	done := make(chan error, 1)
	go func() {
		_, err := db.Exec("UPDATE t SET v = 1")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("expected deadlock with MaxOpenConns(1), got %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exec after closing rows: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exec still blocked after closing rows")
	}

	db.SetMaxOpenConns(2)
	rows2, err := db.Query("SELECT id FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if !rows2.Next() {
		t.Fatal("expected row")
	}

	done2 := make(chan error, 1)
	go func() {
		_, err := db.Exec("UPDATE t SET v = 2")
		done2 <- err
	}()

	select {
	case err := <-done2:
		if err != nil {
			t.Fatalf("exec with MaxOpenConns(2): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exec blocked with MaxOpenConns(2) while rows open")
	}
	if err := rows2.Close(); err != nil {
		t.Fatal(err)
	}
}
