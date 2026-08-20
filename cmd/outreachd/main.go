package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/andersmyrmel/cold-cli/internal/hosted"
	"github.com/andersmyrmel/cold-cli/pkg/engine"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if os.Getenv("COLD_CLI_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") != "" {
		_ = os.Setenv("COLD_CLI_DATABASE_URL", os.Getenv("DATABASE_URL"))
	}
	if os.Getenv("COLD_CLI_DATABASE_URL") == "" {
		log.Fatal("COLD_CLI_DATABASE_URL (or DATABASE_URL) is required for outreachd")
	}

	mockGmail := os.Getenv("OPENOUTREACH_MOCK_GMAIL") == "1"
	var encKey []byte
	var err error
	if raw := strings.TrimSpace(os.Getenv("CREDENTIAL_ENCRYPTION_KEY")); raw != "" {
		encKey, err = hosted.DeriveKey(raw)
		if err != nil {
			log.Fatalf("encryption key: %v", err)
		}
	} else if mockGmail {
		encKey, _ = hosted.DeriveKey("dev-only-not-for-production-openoutreach-key")
	} else {
		log.Fatal("CREDENTIAL_ENCRYPTION_KEY is required when OPENOUTREACH_MOCK_GMAIL is not set")
	}

	store, err := engine.OpenStore()
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := hosted.BootstrapHostedSchema(store.DB); err != nil {
		log.Fatalf("hosted schema: %v", err)
	}

	workspace := os.Getenv("OPENOUTREACH_WORKSPACE_ID")
	if workspace == "" {
		workspace = os.Getenv("COLD_CLI_WORKSPACE_ID")
	}

	srv, err := hosted.NewServer(store, hosted.ServerOpts{
		WorkspaceID:        workspace,
		InternalToken:      os.Getenv("INTERNAL_CONTAINER_TOKEN"),
		PublicBaseURL:      os.Getenv("PUBLIC_BASE_URL"),
		UseMockGmail:       mockGmail,
		ListenAddr:         envOr("LISTEN_ADDR", ":8080"),
		EncryptionKey:      encKey,
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:    srv.ListenAddr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("outreachd listening", "addr", srv.ListenAddr, "workspace", srv.WorkspaceID, "mock_gmail", srv.UseMockGmail)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	slog.Info("outreachd shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
