package worker

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"server_1/internal/core/config"
	"server_1/internal/core/db"
	"server_1/internal/modules/communications"
)

func Run() {
	_ = godotenv.Load(".env")

	cfg := config.Load()

	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("LOG_PRETTY") == "1" || cfg.AppEnv == "dev" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen})
	}

	for _, name := range config.DBNames() {
		dbCfg := config.DBConfigFromPrefix(name)
		conn, err := db.Connect(dbCfg)
		if err != nil {
			log.Fatal().Err(err).Str("db", name).Msg("db connect failed")
		}
		db.Register(name, conn)
	}
	defer db.CloseAll()

	log.Info().Msg("worker started")

	// Expose Prometheus metrics on port 2112
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Info().Msg("Starting Prometheus metrics server on :2112")
		if err := http.ListenAndServe(":2112", nil); err != nil {
			log.Error().Err(err).Msg("Prometheus metrics server failed")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go communications.RunWorker(ctx, cfg)
	<-ctx.Done()
	log.Info().Msg("worker stopping")
}
