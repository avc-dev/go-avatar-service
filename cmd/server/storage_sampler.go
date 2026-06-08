package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/avc-dev/go-avatar-service/internal/metrics"
)

// storageSampleInterval is how often the storage-usage gauge is refreshed.
// Storage totals drift slowly, so a coarse interval keeps DB load negligible
// while staying fresh enough for a business KPI panel.
const storageSampleInterval = 30 * time.Second

// storageSizer is the slice of the repository the sampler needs — declared at
// the consumer side so the sampler depends on one method, not the whole repo.
type storageSizer interface {
	SumActiveSizeBytes(ctx context.Context) (int64, error)
}

// sampleStorageBytes periodically reads the total stored-bytes sum and publishes
// it to the gauge until ctx is cancelled. It samples once immediately so the
// gauge is populated before the first scrape rather than reading 0 for the
// first interval.
func sampleStorageBytes(ctx context.Context, log *slog.Logger, sizer storageSizer, g *metrics.Storage) {
	ticker := time.NewTicker(storageSampleInterval)
	defer ticker.Stop()

	sample := func() {
		total, err := sizer.SumActiveSizeBytes(ctx)
		if err != nil {
			// Transient DB hiccup: log at debug and keep the previous value;
			// the next tick will refresh it.
			log.Debug("storage sampler: query failed", "err", err)
			return
		}
		g.Set(total)
	}

	sample()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample()
		}
	}
}
