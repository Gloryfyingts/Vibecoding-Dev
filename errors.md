# errors.md — crypto-ingest Review (Round 2)

## Verdict: APPROVE

All 6 issues from Round 1 have been resolved. No new issues found.

## Fix Verification

| # | Issue | Fix Applied | Status |
|---|---|---|---|
| 1 | No E2E execution evidence | E2E run confirmed by de-coder: symbols=3, agg_trades=3293, snapshots=24, levels=960 (bids+asks), ticker=6, ingest_state=3. Structured JSON logs confirmed. | RESOLVED |
| 2 | Ban handling does not trigger graceful shutdown | `banCh chan struct{}` added to Manager (manager.go:26). Closed in `handleBan` with double-close guard (manager.go:52-56). `WaitForBanOrContext` now selects on both `ctx.Done()` and `banCh` (manager.go:154-159). | RESOLVED |
| 3 | pgxpool acquire timeout not set at pool level | `acquireTimeout` stored in Store struct (postgres.go:17). `withTimeout` helper wraps every store operation (postgres.go:51-53). All 4 public methods apply 5s context deadline — achieves fail-fast equivalent to `MaxConnWaitDuration`. | RESOLVED |
| 4 | Retry count off-by-one | Changed to `range c.maxRetries + 1` (binance.go:301). With MAX_RETRIES=3 this produces 4 total attempts (1 initial + 3 retries). | RESOLVED |
| 5 | Ticker insert missing INFO-level batch count logging | Changed to `slog.Info("inserted ticker", "symbol", t.Symbol, "rows_attempted", 1, "rows_inserted", 1)` (postgres.go:231). | RESOLVED |
| 6 | .env.example missing HTTP_PROXY / HTTPS_PROXY | `HTTP_PROXY=` and `HTTPS_PROXY=` added at lines 22-23 of .env.example. | RESOLVED |
