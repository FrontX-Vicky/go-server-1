package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"server_1/internal/core/config"
	"server_1/internal/core/db"
	"server_1/internal/router"
)

func Run() {
	_ = godotenv.Load(".env")

	cfg := config.Load()

	zerolog.TimeFieldFormat = time.RFC3339
	if cfg.AppEnv == "dev" || os.Getenv("LOG_PRETTY") == "1" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen})
	}

	if names := config.DBNames(); len(names) > 0 {
		for _, name := range names {
			pfx := strings.ToUpper(strings.TrimSpace(name))
			dbc := config.DBConfigFromPrefix(pfx)
			conn, err := db.Connect(dbc)
			if err != nil {
				log.Fatal().Err(err).Str("db", pfx).Msg("failed to connect named db")
			}
			db.Register(pfx, conn)
			log.Info().Str("db", pfx).Msg("connected named db")
		}
		defer db.CloseAll()
	}

	r := router.Build(cfg)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Addr, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Create channel for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", addr).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	<-quit
	log.Info().Msg("shutting down server...")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited gracefully")
}
