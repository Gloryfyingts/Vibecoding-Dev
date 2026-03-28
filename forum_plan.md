# Forum Plan: Cryptocurrency Market Data Ingestion

## Task: Binance Public API -> PostgreSQL via Go Workers

### Task Understanding

Build a Go-based data ingestion service that polls Binance's public REST API for cryptocurrency market data (BTC, ETH, USDT trading pairs) and stores trades, order book snapshots, and 24hr ticker data into PostgreSQL. The service runs as a Docker container alongside the existing data engineering stack. This is a greenfield Go project -- no Go code exists in the repo yet.

### Chosen Approach

**Binance REST API with polling** -- Single Go binary (`crypto-ingest`) using goroutine-per-symbol-per-data-type workers. Polling at 5-second intervals for trades and order book, 30-second intervals for ticker. Data written to PostgreSQL via `pgx/v5` with batch inserts and `ON CONFLICT DO NOTHING` deduplication for trades. Resumable ingestion via high-water mark table.

**Why this won:**
- Binance is the largest exchange by volume, truly public API for market data, no auth required, generous rate limits (6000 weight/min)
- REST polling is simpler than WebSocket for v1 -- no connection lifecycle management, reconnection logic, or partial message handling
- Single binary with goroutines is appropriate for 3 symbols x 3 data types (9 workers) -- no microservice overhead needed
- `/api/v3/aggTrades` (weight=2) is 12.5x cheaper than `/api/v3/trades` (weight=25) on rate budget

### Rejected Alternatives

- **WebSocket streaming** -- Adds connection lifecycle management, heartbeat logic, reconnection with gap detection. Overkill for 3 symbols at 5s intervals. Defer to v1.1.
- **Multiple exchanges** -- Scope creep. Binance alone covers all three assets. Multi-exchange support can be added later (architecture supports it).
- **Microservice per data type** -- Unnecessary coordination cost for 9 workers. Single binary is simpler to deploy, monitor, and debug.
- **Historical backfill in v1** -- `/api/v3/historicalTrades` requires API key. Start with real-time forward-only ingestion; backfill is a separate task.
- **Prometheus metrics in v1** -- Adds dependency (`prometheus/client_golang`) for an internal dev tool. Structured `slog` logging is sufficient. Defer to v1.1.

### Binance API Endpoints (Verified by Verifier)

| Endpoint | Weight | Data | Auth Required |
|----------|--------|------|---------------|
| `GET /api/v3/aggTrades?symbol=BTCUSDT&limit=1000` | 2 | Aggregated trades (price, qty, time, trade ID) | No |
| `GET /api/v3/depth?symbol=BTCUSDT&limit=20` | 2 | Order book (bids + asks at 20 levels) | No |
| `GET /api/v3/ticker/24hr?symbol=BTCUSDT` | 2 | 24hr rolling stats (OHLC, volume, trade count) | No |
| `GET /api/v3/exchangeInfo?symbols=["BTCUSDT","ETHUSDT","ETHBTC"]` | 20 | Symbol metadata (precision, status) | No |
| `GET /api/v3/ping` | 1 | Connectivity test | No |

Trading pairs: **BTCUSDT**, **ETHUSDT**, **ETHBTC** (covers all BTC/ETH/USDT relationships)

### Architecture

```
Binance REST API (public, no auth)
        |
        v
Go binary: crypto-ingest (single Docker container)
  +-- Trade Workers      (1 goroutine per symbol, 3 total, 5s poll)
  +-- OrderBook Workers  (1 goroutine per symbol, 3 total, 5s poll)
  +-- Ticker Workers     (1 goroutine per symbol, 3 total, 30s poll)
  +-- Metadata Loader    (fetches exchangeInfo once at startup)
  +-- Health Endpoint    (:8085/healthz, self-probe via --healthcheck flag)
  +-- Shared Token Bucket Rate Limiter (5000 weight/min, 83% of 6000 limit)
  +-- PostgreSQL Batch Writer (pgx.Batch + ON CONFLICT for trades)
        |
        v
PostgreSQL (existing `pipeline` database, new `crypto` schema)
  +-- crypto.symbols              (symbol metadata)
  +-- crypto.agg_trades           (executed trades, deduped by agg_trade_id)
  +-- crypto.orderbook_snapshots  (snapshot headers)
  +-- crypto.orderbook_levels     (individual price levels per snapshot)
  +-- crypto.ticker_24hr          (24hr rolling stats)
  +-- crypto.ingest_state         (high-water marks for resumable ingestion)
```

### Rate Limit Budget (at default settings)

| Worker Type | Symbols | Interval | Weight/Call | Calls/Min | Weight/Min |
|-------------|---------|----------|-------------|-----------|------------|
| aggTrades   | 3       | 5s       | 2           | 36        | 72         |
| depth (20)  | 3       | 5s       | 2           | 36        | 72         |
| ticker      | 3       | 30s      | 2           | 6         | 12         |
| exchangeInfo| 1       | startup  | 20          | ~0        | ~0         |
| **Total**   |         |          |             | **78**    | **156**    |

Budget: 156/5000 allocated = 3.1% of safety-capped limit. Massive headroom for scaling to more pairs.

### Scope

**Files to CREATE (16 files):**

| File | Purpose |
|------|---------|
| `crypto-ingest/go.mod` | Go module definition |
| `crypto-ingest/cmd/ingest/main.go` | Application entrypoint, config validation, wiring |
| `crypto-ingest/internal/config/config.go` | Environment variable parsing |
| `crypto-ingest/internal/client/binance.go` | Binance REST API client + response types |
| `crypto-ingest/internal/client/ratelimit.go` | Token bucket wrapper (`golang.org/x/time/rate`) |
| `crypto-ingest/internal/model/trade.go` | AggTrade struct |
| `crypto-ingest/internal/model/orderbook.go` | OrderBookSnapshot + OrderBookLevel structs |
| `crypto-ingest/internal/model/ticker.go` | Ticker24hr struct |
| `crypto-ingest/internal/model/symbol.go` | SymbolInfo struct |
| `crypto-ingest/internal/worker/trade.go` | Trade polling goroutine |
| `crypto-ingest/internal/worker/orderbook.go` | Order book polling goroutine |
| `crypto-ingest/internal/worker/ticker.go` | Ticker polling goroutine |
| `crypto-ingest/internal/worker/manager.go` | Worker lifecycle, graceful shutdown, health endpoint |
| `crypto-ingest/internal/store/postgres.go` | pgx pool, batch inserts, COPY for orderbook/ticker |
| `crypto-ingest/internal/store/migrations.go` | CREATE SCHEMA/TABLE IF NOT EXISTS on startup |
| `crypto-ingest/Dockerfile` | Multi-stage build (golang:1.23-alpine -> distroless) |

**Files to MODIFY (2 files):**

| File | Change |
|------|--------|
| `docker/postgres/init.sql` | Add `CREATE SCHEMA IF NOT EXISTS crypto;` |
| `docker-compose.yml` | Add `crypto-ingest` service with healthcheck |

**Files to CREATE (1 verification script):**

| File | Purpose |
|------|---------|
| `scripts/verify_crypto_e2e.sh` | Automated E2E check: query row counts after 60s |

### PostgreSQL Schema (crypto schema)

**crypto.symbols**
```sql
CREATE TABLE IF NOT EXISTS crypto.symbols (
    symbol          TEXT        PRIMARY KEY,
    status          TEXT        NOT NULL,
    base_asset      TEXT        NOT NULL,
    quote_asset     TEXT        NOT NULL,
    base_precision  INTEGER     NOT NULL,
    quote_precision INTEGER     NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**crypto.agg_trades**
```sql
CREATE TABLE IF NOT EXISTS crypto.agg_trades (
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
);
CREATE INDEX IF NOT EXISTS idx_agg_trades_symbol_time
    ON crypto.agg_trades (symbol, trade_time);
```

**crypto.orderbook_snapshots**
```sql
CREATE TABLE IF NOT EXISTS crypto.orderbook_snapshots (
    snapshot_id     BIGSERIAL   PRIMARY KEY,
    symbol          TEXT        NOT NULL,
    last_update_id  BIGINT      NOT NULL,
    depth_level     INTEGER     NOT NULL,
    snapshot_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ob_snapshots_symbol_time
    ON crypto.orderbook_snapshots (symbol, snapshot_time);
```

**crypto.orderbook_levels** (no FK -- consensus from all debating planners)
```sql
CREATE TABLE IF NOT EXISTS crypto.orderbook_levels (
    snapshot_id     BIGINT      NOT NULL,
    side            TEXT        NOT NULL CHECK (side IN ('bid', 'ask')),
    level_index     SMALLINT    NOT NULL,
    price           NUMERIC     NOT NULL,
    quantity        NUMERIC     NOT NULL,
    PRIMARY KEY (snapshot_id, side, level_index)
);
```

**crypto.ticker_24hr**
```sql
CREATE TABLE IF NOT EXISTS crypto.ticker_24hr (
    id              BIGSERIAL   PRIMARY KEY,
    symbol          TEXT        NOT NULL,
    price_change    NUMERIC     NOT NULL,
    price_change_pct NUMERIC   NOT NULL,
    weighted_avg    NUMERIC     NOT NULL,
    prev_close      NUMERIC     NOT NULL,
    last_price      NUMERIC     NOT NULL,
    bid_price       NUMERIC     NOT NULL,
    ask_price       NUMERIC     NOT NULL,
    open_price      NUMERIC     NOT NULL,
    high_price      NUMERIC     NOT NULL,
    low_price       NUMERIC     NOT NULL,
    volume          NUMERIC     NOT NULL,
    quote_volume    NUMERIC     NOT NULL,
    open_time       TIMESTAMPTZ NOT NULL,
    close_time      TIMESTAMPTZ NOT NULL,
    trade_count     BIGINT      NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ticker_symbol_time
    ON crypto.ticker_24hr (symbol, fetched_at);
```

**crypto.ingest_state**
```sql
CREATE TABLE IF NOT EXISTS crypto.ingest_state (
    worker_type     TEXT        NOT NULL,
    symbol          TEXT        NOT NULL,
    last_id         BIGINT,
    last_timestamp  TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (worker_type, symbol)
);
```

### Key Implementation Decisions (from debate)

| Decision | Rationale | Settled By |
|----------|-----------|------------|
| **No `shopspring/decimal`** -- pass Binance price strings directly to PG as NUMERIC | Binance returns prices as strings; Go code does not compute with them, only transfers to DB. PG's NUMERIC parser handles all edge cases. Validate non-empty strings before insert. For COPY batches, validate before sending; for pgx.Batch, handle per-statement errors. | Skeptic + Pragmatist (simplification wins, correctness preserved) |
| **No FK on `orderbook_levels`** | With ~2M inserts/day, FK lookup on every insert adds overhead. Referential integrity is guaranteed by application logic (snapshot inserted moments before levels). No external writer exists. | All 4 agree |
| **Atomic `ingest_state` updates** | Wrap trade batch INSERT + ingest_state UPDATE in same pgx transaction. Prevents wasted re-fetch work on crash recovery. | All 4 agree |
| **Orderbook depth default: 20** (not 100) | Weight drops from 5 to 2 per request. Volume drops from ~10.4M to ~2.1M rows/day. Top 20 levels capture meaningful spread. Bump to 100 via env var. | Pragmatist + Skeptic |
| **Keep `ticker_24hr` in v1** | User explicitly asked for "other basic market data." Ticker is the most fundamental market data endpoint. Cost: ~50 LOC, 6 weight/min, 8640 rows/day. Trivial. | Skeptic + Architect + Verifier (3:1) |
| **`pgx.Batch`** for trades (not temp-table COPY) | At 500-1000 rows per batch, pgx.Batch is sufficient. Temp-table adds 4 SQL operations per flush for marginal gain at this scale. | Skeptic conceded, consensus |
| **exchangeInfo once at startup** (no hourly refresh) | Exchange metadata changes extremely rarely. Hourly refresh adds complexity for zero benefit. | Pragmatist + Skeptic |
| **Drop `filters_json`** from symbols | Trading rule filters are irrelevant for read-only ingestion. Simplifies model. | Skeptic + Pragmatist |
| **Keep `is_best_match`** column | One boolean per row preserves API response fidelity. Negligible cost. | Architect |
| **Health endpoint** at `:8085/healthz` + docker-compose healthcheck | Every existing service has a healthcheck. Without one, `docker compose ps` shows "Up" even if goroutines panicked. Distroless has no wget/curl/shell, so the Go binary includes a `--healthcheck` flag that self-probes and exits 0/1. | All 4 agree |
| **`stop_grace_period: 15s`** on crypto-ingest in docker-compose | Ensures flush completes on shutdown. Workers need time to finish in-flight batch inserts. | Architect |
| **Startup connectivity check** -- ping Binance API, fail fast if unreachable | Binance restricts API from certain regions. Clear error message beats silent failure during first poll. | Skeptic |
| **`os.Interrupt + syscall.SIGTERM`** for signal handling | Covers both Windows dev (os.Interrupt) and Linux containers (SIGTERM). | Skeptic |
| **On context cancellation, discard in-flight HTTP responses** | Partial data in buffer after cancelled read could corrupt batch. Only flush fully parsed data. | Skeptic |
| **Defer `exchange` column** to multi-exchange milestone | PG 11+ handles `ALTER TABLE ADD COLUMN NOT NULL DEFAULT` as metadata-only operation. No painful migration later. Not needed for single-exchange v1. | Pragmatist (skeptic conceded) |
| **Defer retention policy** | DELETE on millions of rows causes lock contention and WAL bloat. Proper retention needs partitioning. At depth=20, volume is manageable for weeks. | Pragmatist (skeptic conceded) |
| **E2E verification script** | CLAUDE.md mandates E2E testing. Script connects to PG after 60s and asserts row counts > 0 for each table. Reviewer agent needs something runnable. | Skeptic + Pragmatist |

### Execution Order

1. **Modify `docker/postgres/init.sql`** -- add `CREATE SCHEMA IF NOT EXISTS crypto;`
   - Why first: establishes schema for fresh volumes; Go app also creates it on startup for existing volumes

2. **Create `crypto-ingest/go.mod`** -- initialize Go module with dependencies
   - Dependencies: `github.com/jackc/pgx/v5`, `golang.org/x/time`
   - Why second: all Go files need the module

3. **Create model structs** (`internal/model/*.go`)
   - Why third: pure data types with zero dependencies, used by all other packages

4. **Create config** (`internal/config/config.go`)
   - Parse env vars: `DATABASE_URL`, `BINANCE_BASE_URL`, `SYMBOLS`, `TRADE_POLL_INTERVAL`, `ORDERBOOK_POLL_INTERVAL`, `ORDERBOOK_DEPTH`, `TICKER_POLL_INTERVAL`
   - Validate all required vars on startup, fail fast with clear error
   - Why fourth: needed by client, store, and worker packages

5. **Create rate limiter** (`internal/client/ratelimit.go`)
   - Wrap `golang.org/x/time/rate.Limiter`, acquire tokens per API weight
   - Why fifth: needed by Binance client

6. **Create Binance client** (`internal/client/binance.go`)
   - HTTP client with rate limiter, response parsing, startup ping check
   - Read `X-MBX-USED-WEIGHT-1m` header for rate limit monitoring
   - Validate non-empty price/qty strings before returning
   - Why sixth: depends on models + rate limiter

7. **Create PostgreSQL store** (`internal/store/postgres.go` + `migrations.go`)
   - `migrations.go`: `CREATE SCHEMA IF NOT EXISTS crypto` + all `CREATE TABLE IF NOT EXISTS`
   - `postgres.go`: pgx pool, batch trade inserts (pgx.Batch + ON CONFLICT DO NOTHING), COPY for orderbook levels, simple INSERT for ticker, atomic ingest_state updates
   - Why seventh: depends on models, parallel with step 6

8. **Create workers** (`internal/worker/*.go`)
   - `trade.go`: poll aggTrades, resume from ingest_state high-water mark
   - `orderbook.go`: poll depth, insert snapshot + levels in transaction
   - `ticker.go`: poll 24hr ticker
   - `manager.go`: start all workers, health endpoint, graceful shutdown (SIGINT/SIGTERM, 10s flush deadline)
   - Why eighth: depends on client + store

9. **Create entrypoint** (`cmd/ingest/main.go`)
   - Wire config -> client -> store -> workers -> manager
   - Validate config, run migrations, ping Binance, start workers
   - Include `--healthcheck` flag: when set, binary makes HTTP GET to localhost:HEALTH_PORT/healthz and exits 0/1 (needed because distroless has no wget/curl/shell)
   - Why ninth: depends on all internal packages

10. **Create Dockerfile**
    - Multi-stage: `golang:1.22.12-alpine3.21` build stage -> `gcr.io/distroless/static-debian12:nonroot` runtime
    - Why tenth: needs compilable Go code

11. **Modify `docker-compose.yml`**
    - Add `crypto-ingest` service with: environment from `.env`, `depends_on: postgres: condition: service_healthy`, `mem_limit: 256m`, `stop_grace_period: 15s`, healthcheck on `:8085/healthz`, network `data-pipeline`, `restart: unless-stopped`
    - Why eleventh: needs Dockerfile

12. **Create `scripts/verify_crypto_e2e.sh`**
    - Wait 60s, query `SELECT count(*) FROM crypto.agg_trades`, `crypto.orderbook_snapshots`, `crypto.orderbook_levels`, `crypto.ticker_24hr`, `crypto.symbols`
    - Assert all counts > 0
    - Why twelfth: needs running stack

13. **E2E test**
    - `docker compose up -d`, wait for healthy, run verification script
    - Why last: validates everything

### Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Binance API inaccessible (geo-restrictions for Russia/CIS/US) | No data ingested | `BINANCE_BASE_URL` configurable; alternative endpoints `api1/api2/api3.binance.com`; startup ping check fails fast with clear error |
| Float precision loss on prices/quantities | Incorrect financial data | Pass Binance string prices directly to PG NUMERIC; never use float64; validate non-empty before insert |
| Rate limit exceeded -> HTTP 429 or 418 IP ban | Ingestion stops | Token bucket at 5000/min (83% of 6000); read `X-MBX-USED-WEIGHT-1m` header; exponential backoff on 429 |
| `init.sql` not re-run on existing Postgres volume | `crypto` schema missing | Go app runs `CREATE SCHEMA/TABLE IF NOT EXISTS` on startup independently |
| Orderbook data volume (~2.1M rows/day at depth=20) | Disk fills over weeks | Configurable depth/interval; retention strategy deferred to v1.1 (volume manageable for weeks) |
| `pgx.CopyFrom` incompatible with `ON CONFLICT` | Cannot dedup trades via COPY | Use `pgx.Batch` with INSERT ON CONFLICT for trades; COPY only for orderbook/ticker |
| Docker image tag unavailable | Build fails | Use `golang:1.22.12-alpine3.21` (verified stable) or `golang:1.23-alpine` (verify tag exists) |
| Distroless image has no shell/curl/wget | Docker healthcheck fails | Go binary includes `--healthcheck` flag for self-probing |
| Crash between batch insert and ingest_state update | Wasted re-fetch work on restart | Atomic: wrap both in same pgx transaction |
| In-flight HTTP request during shutdown | Partial/corrupt data | Discard in-progress fetches on context cancellation; only flush fully parsed data |

### Configuration (via environment variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `BINANCE_BASE_URL` | `https://api.binance.com` | Binance REST API base URL |
| `SYMBOLS` | `BTCUSDT,ETHUSDT,ETHBTC` | Comma-separated trading pairs |
| `TRADE_POLL_INTERVAL` | `5s` | Trade worker polling interval |
| `ORDERBOOK_POLL_INTERVAL` | `5s` | Order book worker polling interval |
| `ORDERBOOK_DEPTH` | `20` | Order book levels per side (5, 10, 20, 50, 100, 500) |
| `TICKER_POLL_INTERVAL` | `30s` | Ticker worker polling interval |
| `HEALTH_PORT` | `8085` | Health endpoint listen port |
| `LOG_LEVEL` | `info` | Structured log level (debug, info, warn, error) |
| `MAX_RETRIES` | `3` | Max retries per failed API call |
| `RETRY_BASE_DELAY` | `1s` | Base delay for exponential backoff |

### Go Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/jackc/pgx/v5` | v5.7.x | PostgreSQL driver with COPY, batch, pool support |
| `golang.org/x/time` | v0.9.x | Token bucket rate limiter (`rate.Limiter`) |

Two external dependencies total. `log/slog` is stdlib (Go 1.21+).

### Definition of Done

**Infrastructure:**
- [ ] `docker/postgres/init.sql` includes `CREATE SCHEMA IF NOT EXISTS crypto;`
- [ ] `docker-compose.yml` includes `crypto-ingest` service with healthcheck, `depends_on`, `stop_grace_period: 15s`, mem_limit, restart policy
- [ ] `crypto-ingest/Dockerfile` uses pinned, multi-stage build
- [ ] `docker compose up -d` brings up `crypto-ingest` alongside existing services without errors

**Go Application:**
- [ ] `go.mod` has exactly 2 external dependencies: `pgx/v5`, `golang.org/x/time`
- [ ] `shopspring/decimal` is NOT in `go.mod`
- [ ] Application validates all required env vars on startup and exits with clear error if missing
- [ ] Application pings Binance API on startup and exits with clear error if unreachable
- [ ] Application creates `crypto` schema and all tables on startup via `CREATE IF NOT EXISTS`

**Data Ingestion:**
- [ ] Trade workers poll `/api/v3/aggTrades` for BTCUSDT, ETHUSDT, ETHBTC
- [ ] Order book workers poll `/api/v3/depth?limit=20` for all 3 symbols
- [ ] Ticker workers poll `/api/v3/ticker/24hr` for all 3 symbols
- [ ] Exchange metadata loaded from `/api/v3/exchangeInfo` once at startup
- [ ] All workers respect shared token bucket rate limiter (5000 weight/min)
- [ ] Workers retry failed requests with exponential backoff (max 3 retries)

**Schema:**
- [ ] `crypto.symbols` populated with 3 symbol records at startup
- [ ] `crypto.agg_trades` has composite PK `(symbol, agg_trade_id)` and uses `ON CONFLICT DO NOTHING`
- [ ] `crypto.orderbook_snapshots` has BIGSERIAL PK
- [ ] `crypto.orderbook_levels` has NO foreign key constraint on `snapshot_id`
- [ ] `crypto.ticker_24hr` stores all fields from the 24hr ticker endpoint
- [ ] `crypto.ingest_state` stores high-water marks per (worker_type, symbol)
- [ ] All prices/quantities stored as NUMERIC (no float64)
- [ ] All timestamps stored as TIMESTAMPTZ

**Idempotency and Resumability:**
- [ ] Trade batch inserts and `ingest_state` updates occur in the same pgx transaction
- [ ] On restart, trade workers resume from `last_id + 1` in `ingest_state`
- [ ] Duplicate trades are silently skipped via `ON CONFLICT DO NOTHING`

**Operational:**
- [ ] Health endpoint at `:8085/healthz` returns 200 when workers running and DB connected
- [ ] `--healthcheck` flag makes binary self-probe `/healthz` and exit 0/1 (distroless compatibility)
- [ ] Docker-compose healthcheck uses `["/crypto-ingest", "--healthcheck"]`
- [ ] Structured logging via `slog` with configurable log level
- [ ] Graceful shutdown on SIGINT/SIGTERM: cancel context, flush in-progress batches, 10s deadline
- [ ] `os.Interrupt + syscall.SIGTERM` used for cross-platform signal handling
- [ ] On context cancellation, in-flight HTTP responses are discarded (only fully parsed data flushed)

**E2E Verification:**
- [ ] `scripts/verify_crypto_e2e.sh` exists and checks row counts > 0 for all tables
- [ ] After running `docker compose up -d` for 60 seconds, all verification checks pass
- [ ] `crypto.agg_trades` contains rows with valid prices, quantities, and timestamps
- [ ] `crypto.orderbook_levels` contains bid and ask levels
- [ ] `crypto.ticker_24hr` contains 24hr stats for all 3 symbols
- [ ] `crypto.symbols` contains 3 records (BTCUSDT, ETHUSDT, ETHBTC)

### Validation Checklist

- [ ] `docker compose up -d` -- all services healthy including crypto-ingest
- [ ] `docker compose logs crypto-ingest` -- no errors, structured JSON logs
- [ ] `curl localhost:8085/healthz` -- returns 200
- [ ] `scripts/verify_crypto_e2e.sh` -- all checks pass after 60s
- [ ] Stop and restart crypto-ingest -- resumes from last ingested trade ID (no duplicates)
- [ ] `docker compose down && docker compose up -d` -- crypto-ingest creates schema/tables on startup

### Open Questions

- **Binance geo-availability:** If the user's network blocks Binance, they may need to use a proxy or switch to `api.binance.us` (which has slightly different endpoints). This should be tested early in implementation.
- **Go Docker image tag:** Verifier recommends `golang:1.22.12-alpine3.21` as verified stable. `golang:1.23-alpine` is also viable but exact patch tag should be confirmed. The code is compatible with Go 1.22+.

### Deferred to v1.1

- WebSocket streaming (real-time push instead of REST polling)
- Historical trade backfill (`/api/v3/historicalTrades` -- requires API key)
- Order book depth > 20 levels (configurable via env var, just change default)
- Prometheus metrics endpoint
- Data retention policy (partitioning or batched deletes for orderbook data)
- `exchange` column on all tables (for multi-exchange support)
- Kline/candlestick ingestion (`/api/v3/klines`)
- Multiple exchange support

---

*Plan produced by 4-planner adversarial forum: architect, skeptic, pragmatist, verifier. Each perspective independently analyzed the task, then cross-critiqued and debated to convergence. Final synthesis applied resolution rules: verifier wins on repo facts, skeptic on risks, pragmatist on simplification, architect on structure.*
