package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	cfg := auth.Config{
		ClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		Origins:       strings.Split(env("APP_ORIGINS", "https://maceclubheemskerk.eu,https://www.maceclubheemskerk.eu,https://maceclubheemskerk.vdzonsoftware.nl"), ","),
		SecureCookies: env("COOKIE_SECURE", "true") != "false",
	}
	if allowed := os.Getenv("ALLOWED_EMAILS"); allowed != "" {
		for _, email := range strings.Split(allowed, ",") {
			if email = strings.TrimSpace(email); email != "" {
				cfg.AllowedEmails = append(cfg.AllowedEmails, email)
			}
		}
	}
	if err := auth.ValidateConfig(cfg); err != nil {
		return err
	}
	var store *auth.Store
	if cfg.ClientID != "" {
		path := os.Getenv("SESSION_FILE")
		if path == "" {
			return errors.New("SESSION_FILE is required when Google login is enabled")
		}
		var err error
		store, err = auth.OpenStore(path)
		if err != nil {
			return fmt.Errorf("open session store: %w", err)
		}
		defer store.Close()
	}
	verifier, err := auth.GoogleVerifier()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.Open(executable)
	if err != nil {
		return err
	}
	fingerprint := sha256.New()
	_, err = io.Copy(fingerprint, binary)
	binary.Close()
	if err != nil {
		return err
	}
	version := fmt.Sprintf("%x", fingerprint.Sum(nil))
	server := &http.Server{
		Addr: ":" + port, Handler: web.Handler(auth.New(cfg, store, verifier), version),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "address", server.Addr)
		errorsCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errorsCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errorsCh; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		slog.Info("server stopped gracefully")
		return nil
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
