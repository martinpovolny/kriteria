package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/martinpovolny/kriteria/internal/api"
	"github.com/martinpovolny/kriteria/internal/store"
)

var (
	commit    = "dev"
	buildTime = "unknown"
)

func main() {
	// Load .env file (ignore error — file is optional, env vars may be set directly)
	_ = godotenv.Load()

	addr := flag.String("addr", getenv("ADDR", ":8088"), "listen address")
	dbPath := flag.String("db", getenv("DB_PATH", "data/kriteria.db"), "path to SQLite database file")
	jsonPath := flag.String("json", getenv("JSON_PATH", "data/kriteria.json"), "path to kriteria.json (serves /api/criteria)")
	devMode := flag.Bool("dev", getenv("DEV_MODE", "true") == "true", "dev mode: mock teacher, no OAuth")
	oauthClientID := flag.String("oauth-client-id", getenv("OAUTH_CLIENT_ID", ""), "Google OAuth2 client ID")
	oauthSecret := flag.String("oauth-secret", getenv("OAUTH_CLIENT_SECRET", ""), "Google OAuth2 client secret")
	oauthRedirect := flag.String("oauth-redirect", getenv("OAUTH_REDIRECT_URL", ""), "OAuth2 redirect URL")
	sessionSecret := flag.String("session-secret", getenv("SESSION_SECRET", ""), "HMAC secret for session cookies")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting kriteria", "commit", commit, "build_time", buildTime, "addr", *addr, "db", *dbPath, "dev", *devMode)

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("failed to open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Ensure the current school year exists in the DB
	if id, label := api.EnsureCurrentSchoolYear(st.DB(), ctx); id > 0 {
		logger.Info("current school year", "label", label, "id", id)
	}

	// Build auth config
	secretKey := *sessionSecret
	if secretKey == "" {
		secretKey = api.GenerateRandomKey()
		logger.Info("generated random session secret (set --session-secret for stable sessions)")
	}
	authCfg := api.AuthConfig{
		ClientID:     *oauthClientID,
		ClientSecret: *oauthSecret,
		RedirectURL:  *oauthRedirect,
		SecretKey:    secretKey,
	}
	if *oauthClientID != "" {
		logger.Info("OAuth enabled", "redirect", *oauthRedirect)
	}

	handler := api.NewMux(logger, st, api.BuildInfo{Commit: commit, BuildTime: buildTime}, *jsonPath, *devMode, authCfg)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	logger.Info("listening", "addr", *addr)

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
