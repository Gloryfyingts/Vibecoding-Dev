package store

import (
	"context"
	"fmt"
	"log/slog"
)

func (s *Store) RunMigrations(ctx context.Context) error {
	slog.Info("running database migrations")

	queries := []string{
		`CREATE SCHEMA IF NOT EXISTS crypto`,

		`CREATE TABLE IF NOT EXISTS crypto.symbols (
    symbol          TEXT        PRIMARY KEY,
    status          TEXT        NOT NULL,
    base_asset      TEXT        NOT NULL,
    quote_asset     TEXT        NOT NULL,
    base_precision  INTEGER     NOT NULL,
    quote_precision INTEGER     NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now()
)`,

		`CREATE TABLE IF NOT EXISTS crypto.agg_trades (
    symbol          TEXT        NOT NULL,
    agg_trade_id    BIGINT      NOT NULL,
    price           NUMERIC     NOT NULL,
    quantity        NUMERIC     NOT NULL,
    first_trade_id  BIGINT      NOT NULL,
    last_trade_id   BIGINT      NOT NULL,
    trade_time      TIMESTAMPTZ NOT NULL,
    is_buyer_maker  BOOLEAN     NOT NULL,
    is_best_match   BOOLEAN     NOT NULL,
    inserted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (symbol, agg_trade_id)
)`,

		`CREATE INDEX IF NOT EXISTS idx_agg_trades_symbol_time
    ON crypto.agg_trades (symbol, trade_time)`,

		`CREATE TABLE IF NOT EXISTS crypto.orderbook_snapshots (
    snapshot_id     BIGSERIAL   PRIMARY KEY,
    symbol          TEXT        NOT NULL,
    last_update_id  BIGINT      NOT NULL,
    depth_level     INTEGER     NOT NULL,
    snapshot_time   TIMESTAMPTZ NOT NULL DEFAULT now()
)`,

		`CREATE INDEX IF NOT EXISTS idx_ob_snapshots_symbol_time
    ON crypto.orderbook_snapshots (symbol, snapshot_time)`,

		`CREATE TABLE IF NOT EXISTS crypto.orderbook_levels (
    snapshot_id     BIGINT      NOT NULL,
    side            TEXT        NOT NULL CHECK (side IN ('bid', 'ask')),
    level_index     SMALLINT    NOT NULL,
    price           NUMERIC     NOT NULL,
    quantity        NUMERIC     NOT NULL,
    PRIMARY KEY (snapshot_id, side, level_index)
)`,

		`CREATE TABLE IF NOT EXISTS crypto.ticker_24hr (
    id              BIGSERIAL   PRIMARY KEY,
    symbol          TEXT        NOT NULL,
    price_change    NUMERIC     NOT NULL,
    price_change_pct NUMERIC   NOT NULL,
    weighted_avg    NUMERIC     NOT NULL,
    prev_close      NUMERIC     NOT NULL,
    last_price      NUMERIC     NOT NULL,
    last_qty        NUMERIC     NOT NULL,
    bid_price       NUMERIC     NOT NULL,
    bid_qty         NUMERIC     NOT NULL,
    ask_price       NUMERIC     NOT NULL,
    ask_qty         NUMERIC     NOT NULL,
    open_price      NUMERIC     NOT NULL,
    high_price      NUMERIC     NOT NULL,
    low_price       NUMERIC     NOT NULL,
    volume          NUMERIC     NOT NULL,
    quote_volume    NUMERIC     NOT NULL,
    open_time       TIMESTAMPTZ NOT NULL,
    close_time      TIMESTAMPTZ NOT NULL,
    first_trade_id  BIGINT      NOT NULL,
    last_trade_id   BIGINT      NOT NULL,
    trade_count     BIGINT      NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now()
)`,

		`CREATE INDEX IF NOT EXISTS idx_ticker_symbol_time
    ON crypto.ticker_24hr (symbol, fetched_at)`,

		`CREATE TABLE IF NOT EXISTS crypto.ingest_state (
    worker_type     TEXT        NOT NULL,
    symbol          TEXT        NOT NULL,
    last_id         BIGINT,
    last_timestamp  TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (worker_type, symbol)
)`,
	}

	for i, q := range queries {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migration query %d failed: %w", i, err)
		}
	}

	slog.Info("database migrations completed")
	return nil
}
