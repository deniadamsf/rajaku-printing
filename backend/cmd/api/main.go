// Command api is the HTTP entrypoint for the Rajaku Printing backend.
// Startup order is deliberate and fail-fast: config → logger → DB → router →
// http.Server with graceful shutdown. Any failure before ListenAndServe aborts
// the process (per spec section 22 — no silent-degraded startup).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/config"
	"github.com/rajaku-printing/backend/internal/database"
	"github.com/rajaku-printing/backend/internal/logger"
	"github.com/rajaku-printing/backend/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	isDev := cfg.App.Env == config.EnvDevelopment
	logger.Init(cfg.App.LogLevel, isDev)

	log.Info().
		Str("env", string(cfg.App.Env)).
		Str("base_url", cfg.App.BaseURL).
		Int("port", cfg.App.Port).
		Msg("starting rajaku-printing api")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DB, isDev)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	log.Info().Str("host", cfg.DB.Host).Str("db", cfg.DB.Name).Msg("database connected")

	router, bg, err := server.NewRouter(server.Deps{Config: cfg, DB: db})
	if err != nil {
		return fmt.Errorf("wire router: %w", err)
	}

	// Job terjadwal (§19 retention). Dimulai setelah wiring sukses, dihentikan
	// duluan saat shutdown supaya tidak ada job yang jalan sementara koneksi DB
	// sedang ditutup.
	if bg != nil && bg.Jobs != nil {
		bg.Jobs.Start()
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.App.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if bg != nil && bg.Jobs != nil {
		bg.Jobs.Stop(shutdownCtx)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}

	log.Info().Msg("shutdown complete")
	return nil
}
