package worker

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go communications.RunWorker(ctx, cfg)
	<-ctx.Done()
	log.Info().Msg("worker stopping")
}
