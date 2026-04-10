package worker

import (
	"context"
	"log/slog"
	"time"

	"crypto-ingest/internal/client"
	"crypto-ingest/internal/store"
)

type TickerWorker struct {
	symbol   string
	interval time.Duration
	client   *client.BinanceClient
	store    *store.Store
	logger   *slog.Logger
}

func NewTickerWorker(symbol string, interval time.Duration, c *client.BinanceClient, s *store.Store) *TickerWorker {
	return &TickerWorker{
		symbol:   symbol,
		interval: interval,
		client:   c,
		store:    s,
		logger:   slog.With("worker", "ticker", "symbol", symbol),
	}
}

func (w *TickerWorker) Run(ctx context.Context) {
	w.logger.Info("starting ticker worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.pollTicker(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("ticker worker stopped")
			return
		case <-ticker.C:
			w.pollTicker(ctx)
		}
	}
}

func (w *TickerWorker) pollTicker(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	t, err := w.client.FetchTicker24hr(ctx, w.symbol)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logger.Error("failed to fetch ticker", "error", err)
		return
	}

	if t == nil {
		return
	}

	if err := w.store.InsertTicker(ctx, t); err != nil {
		w.logger.Error("failed to insert ticker", "error", err)
	}
}
