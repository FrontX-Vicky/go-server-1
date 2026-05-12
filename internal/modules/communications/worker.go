package communications

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"server_1/internal/core/config"
)

func RunWorker(ctx context.Context, cfg config.Config) {
	service := NewEmailService(cfg.Email)
	pollEvery := time.Duration(cfg.Email.WorkerPollMS) * time.Millisecond
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processed, err := service.ProcessNextQueuedJob(ctx)
			if err != nil {
				log.Error().Err(err).Msg("email worker failed to process job")
			}
			if processed {
				for {
					processed, err = service.ProcessNextQueuedJob(ctx)
					if err != nil {
						log.Error().Err(err).Msg("email worker failed to drain queue")
						break
					}
					if !processed {
						break
					}
				}
			}
		}
	}
}
