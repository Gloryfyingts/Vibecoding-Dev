package worker

import (
	"context"
	"log/slog"
	"time"

	"crypto-ingest/internal/client"
	"crypto-ingest/internal/store"
)

type OrderBookWorker struct {
	symbol   string
	depth    int
	interval time.Duration
	client   *client.BinanceClient
	store    *store.Store
	logger   *slog.Logger
}

func NewOrderBookWorker(symbol string, depth int, interval time.Duration, c *client.BinanceClient, s *store.Store) *OrderBookWorker {
	return &OrderBookWorker{
		symbol:   symbol,
		depth:    depth,
		interval: interval,
		client:   c,
		store:    s,
		logger:   slog.With("worker", "orderbook", "symbol", symbol),
	}
}

func (w *OrderBookWorker) Run(ctx context.Context) {
	w.logger.Info("starting orderbook worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.pollOrderBook(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("orderbook worker stopped")
			return
		case <-ticker.C:
			w.pollOrderBook(ctx)
		}
	}
}

func (w *OrderBookWorker) pollOrderBook(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	ob, err := w.client.FetchOrderBook(ctx, w.symbol, w.depth)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logger.Error("failed to fetch orderbook", "error", err)
		return
	}

	if ob == nil {
		return
	}

	if err := w.store.InsertOrderBook(ctx, ob); err != nil {
		w.logger.Error("failed to insert orderbook", "error", err)
	}
}
