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
)

func Run() {
    _ = godotenv.Load(".env")

    zerolog.TimeFieldFormat = time.RFC3339
    if os.Getenv("LOG_PRETTY") == "1" {
        log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen})
    }

    for _, name := range config.DBNames() {
        cfg := config.DBConfigFromPrefix(name)
        conn, err := db.Connect(cfg)
        if err != nil { log.Fatal().Err(err).Str("db", name).Msg("db connect failed") }
        db.Register(name, conn)
    }
    defer db.CloseAll()

    log.Info().Msg("worker started (skeleton)")

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    log.Info().Msg("worker stopping")
}
