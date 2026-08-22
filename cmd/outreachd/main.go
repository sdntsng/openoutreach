package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/andersmyrmel/cold-cli/internal/hosted"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	addr := envOr("LISTEN_ADDR", ":8080")

	var mu sync.Mutex
	var handler http.Handler
	var bootErr error

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		h, err := handler, bootErr
		mu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if h == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"data":{"status":"starting"}}`))
			return
		}
		h.ServeHTTP(w, r)
	})

	go func() {
		h, err := boot()
		mu.Lock()
		handler, bootErr = h, err
		mu.Unlock()
		if err != nil {
			slog.Error("outreachd bootstrap failed", "err", err)
			return
		}
		slog.Info("outreachd ready", "addr", addr)
	}()

	slog.Info("outreachd listening", "addr", addr)
	if err := http.ListenAndServe(addr, root); err != nil {
		log.Fatal(err)
	}
}

func boot() (http.Handler, error) {
	if os.Getenv("COLD_CLI_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") != "" {
		_ = os.Setenv("COLD_CLI_DATABASE_URL", os.Getenv("DATABASE_URL"))
	}
	d1 := strings.TrimSpace(os.Getenv("OPENOUTREACH_D1_PROXY")) != ""
	if os.Getenv("COLD_CLI_DATABASE_URL") == "" && !d1 {
		return nil, errString("COLD_CLI_DATABASE_URL (or DATABASE_URL) is required for outreachd, or set OPENOUTREACH_D1_PROXY for Cloudflare D1")
	}

	store, err := engine.OpenStore()
	if err != nil {
		return nil, err
	}
	if err := hosted.BootstrapHostedSchema(store.DB); err != nil {
		store.Close()
		return nil, err
	}

	var encKey []byte
	if raw := strings.TrimSpace(os.Getenv("CREDENTIAL_ENCRYPTION_KEY")); raw != "" {
		encKey, err = hosted.DeriveKey(raw)
		if err != nil {
			store.Close()
			return nil, err
		}
	} else if os.Getenv("OPENOUTREACH_MOCK_GMAIL") == "1" {
		encKey, _ = hosted.DeriveKey("dev-only-not-for-production-openoutreach-key")
	}

	workspace := os.Getenv("OPENOUTREACH_WORKSPACE_ID")
	if workspace == "" {
		workspace = os.Getenv("COLD_CLI_WORKSPACE_ID")
	}

	srv, err := hosted.NewServer(store, hosted.ServerOpts{
		WorkspaceID:        workspace,
		InternalToken:      os.Getenv("INTERNAL_CONTAINER_TOKEN"),
		PublicBaseURL:      os.Getenv("PUBLIC_BASE_URL"),
		TrackingSecret:     os.Getenv("TRACKING_HMAC_SECRET"),
		UseMockGmail:       os.Getenv("OPENOUTREACH_MOCK_GMAIL") == "1",
		ListenAddr:         envOr("LISTEN_ADDR", ":8080"),
		EncryptionKey:      encKey,
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
	})
	if err != nil {
		store.Close()
		return nil, err
	}
	return srv.Handler(), nil
}

type errString string

func (e errString) Error() string { return string(e) }

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
