package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crypto-ingest/internal/model"
)

type Store struct {
	pool           *pgxpool.Pool
	acquireTimeout time.Duration
}

func New(ctx context.Context, databaseURL string, maxConns int32, acquireTimeout time.Duration) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	connCtx, cancel := context.WithTimeout(ctx, acquireTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("connected to postgres", "max_conns", maxConns, "acquire_timeout", acquireTimeout)
	return &Store{pool: pool, acquireTimeout: acquireTimeout}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.acquireTimeout)
}

func (s *Store) UpsertSymbols(ctx context.Context, symbols []model.Symbol) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	batch := &pgx.Batch{}
	for _, sym := range symbols {
		batch.Queue(
			`INSERT INTO crypto.symbols (symbol, status, base_asset, quote_asset, base_precision, quote_precision)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (symbol) DO UPDATE SET
				status = EXCLUDED.status,
				base_asset = EXCLUDED.base_asset,
				quote_asset = EXCLUDED.quote_asset,
				base_precision = EXCLUDED.base_precision,
				quote_precision = EXCLUDED.quote_precision,
				fetched_at = now()`,
			sym.Symbol, sym.Status, sym.BaseAsset, sym.QuoteAsset, sym.BasePrecision, sym.QuotePrecision,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range symbols {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("upserting symbol: %w", err)
		}
	}
	return nil
}

func (s *Store) InsertAggTrades(ctx context.Context, trades []model.AggTrade) (int64, error) {
	if len(trades) == 0 {
		return 0, nil
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, t := range trades {
		batch.Queue(
			`INSERT INTO crypto.agg_trades (symbol, agg_trade_id, price, quantity, first_trade_id, last_trade_id, trade_time, is_buyer_maker, is_best_match)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT DO NOTHING`,
			t.Symbol, t.AggTradeID, t.Price, t.Quantity, t.FirstTradeID, t.LastTradeID, t.TradeTime, t.IsBuyerMaker, t.IsBestMatch,
		)
	}

	br := tx.SendBatch(ctx, batch)
	var rowsInserted int64
	for range trades {
		ct, err := br.Exec()
		if err != nil {
			br.Close()
			return 0, fmt.Errorf("inserting agg trade: %w", err)
		}
		rowsInserted += ct.RowsAffected()
	}
	br.Close()

	var maxID int64
	var maxTime time.Time
	for _, t := range trades {
		if t.AggTradeID > maxID {
			maxID = t.AggTradeID
			maxTime = t.TradeTime
		}
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO crypto.ingest_state (worker_type, symbol, last_id, last_timestamp, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (worker_type, symbol) DO UPDATE SET
			last_id = EXCLUDED.last_id,
			last_timestamp = EXCLUDED.last_timestamp,
			updated_at = now()`,
		"agg_trades", trades[0].Symbol, maxID, maxTime,
	)
	if err != nil {
		return 0, fmt.Errorf("updating ingest state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing trade transaction: %w", err)
	}

	slog.Info("inserted agg trades",
		"symbol", trades[0].Symbol,
		"rows_attempted", len(trades),
		"rows_inserted", rowsInserted,
		"max_trade_id", maxID,
	)
	return rowsInserted, nil
}

func (s *Store) InsertOrderBook(ctx context.Context, ob *model.OrderBook) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var snapshotID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO crypto.orderbook_snapshots (symbol, last_update_id, depth_level)
		VALUES ($1, $2, $3)
		RETURNING snapshot_id`,
		ob.Symbol, ob.LastUpdateID, ob.DepthLevel,
	).Scan(&snapshotID)
	if err != nil {
		return fmt.Errorf("inserting orderbook snapshot: %w", err)
	}

	totalLevels := len(ob.Bids) + len(ob.Asks)
	if totalLevels == 0 {
		return tx.Commit(ctx)
	}

	rows := make([][]interface{}, 0, totalLevels)
	for i, bid := range ob.Bids {
		rows = append(rows, []interface{}{snapshotID, "bid", int16(i), bid.Price, bid.Quantity})
	}
	for i, ask := range ob.Asks {
		rows = append(rows, []interface{}{snapshotID, "ask", int16(i), ask.Price, ask.Quantity})
	}

	copyCount, err := tx.CopyFrom(ctx,
		pgx.Identifier{"crypto", "orderbook_levels"},
		[]string{"snapshot_id", "side", "level_index", "price", "quantity"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("copying orderbook levels: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing orderbook transaction: %w", err)
	}

	slog.Info("inserted orderbook",
		"symbol", ob.Symbol,
		"snapshot_id", snapshotID,
		"levels_attempted", totalLevels,
		"levels_inserted", copyCount,
	)
	return nil
}

func (s *Store) InsertTicker(ctx context.Context, t *model.Ticker24hr) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO crypto.ticker_24hr (
			symbol, price_change, price_change_pct, weighted_avg, prev_close,
			last_price, last_qty, bid_price, bid_qty, ask_price, ask_qty,
			open_price, high_price, low_price, volume, quote_volume,
			open_time, close_time, first_trade_id, last_trade_id, trade_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		t.Symbol, t.PriceChange, t.PriceChangePct, t.WeightedAvg, t.PrevClose,
		t.LastPrice, t.LastQty, t.BidPrice, t.BidQty, t.AskPrice, t.AskQty,
		t.OpenPrice, t.HighPrice, t.LowPrice, t.Volume, t.QuoteVolume,
		t.OpenTime, t.CloseTime, t.FirstTradeID, t.LastTradeID, t.TradeCount,
	)
	if err != nil {
		return fmt.Errorf("inserting ticker: %w", err)
	}

	slog.Info("inserted ticker", "symbol", t.Symbol, "rows_attempted", 1, "rows_inserted", 1)
	return nil
}

func (s *Store) GetIngestState(ctx context.Context, workerType, symbol string) (*model.IngestState, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var state model.IngestState
	err := s.pool.QueryRow(ctx,
		`SELECT worker_type, symbol, last_id, last_timestamp
		FROM crypto.ingest_state
		WHERE worker_type = $1 AND symbol = $2`,
		workerType, symbol,
	).Scan(&state.WorkerType, &state.Symbol, &state.LastID, &state.LastTimestamp)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying ingest state: %w", err)
	}
	return &state, nil
}
