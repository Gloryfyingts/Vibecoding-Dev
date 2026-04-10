package worker

import (
	"context"
	"log/slog"
	"time"

	"crypto-ingest/internal/client"
	"crypto-ingest/internal/store"
)

type TradeWorker struct {
	symbol   string
	interval time.Duration
	client   *client.BinanceClient
	store    *store.Store
	logger   *slog.Logger
}

func NewTradeWorker(symbol string, interval time.Duration, c *client.BinanceClient, s *store.Store) *TradeWorker {
	return &TradeWorker{
		symbol:   symbol,
		interval: interval,
		client:   c,
		store:    s,
		logger:   slog.With("worker", "trade", "symbol", symbol),
	}
}

func (w *TradeWorker) Run(ctx context.Context) {
	w.logger.Info("starting trade worker")

	var fromID *int64
	state, err := w.store.GetIngestState(ctx, "agg_trades", w.symbol)
	if err != nil {
		w.logger.Error("failed to load ingest state, starting from latest", "error", err)
	} else if state != nil && state.LastID != nil {
		resumeID := *state.LastID + 1
		fromID = &resumeID
		w.logger.Info("resuming from last trade id", "from_id", resumeID)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.pollTrades(ctx, &fromID)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("trade worker stopped")
			return
		case <-ticker.C:
			w.pollTrades(ctx, &fromID)
		}
	}
}

func (w *TradeWorker) pollTrades(ctx context.Context, fromID **int64) {
	if ctx.Err() != nil {
		return
	}

	trades, err := w.client.FetchAggTrades(ctx, w.symbol, *fromID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logger.Error("failed to fetch agg trades", "error", err)
		return
	}

	if len(trades) == 0 {
		return
	}

	_, err = w.store.InsertAggTrades(ctx, trades)
	if err != nil {
		w.logger.Error("failed to insert agg trades", "error", err)
		return
	}

	maxID := trades[len(trades)-1].AggTradeID
	nextID := maxID + 1
	*fromID = &nextID
}
