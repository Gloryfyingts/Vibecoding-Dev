# Swarm Plan: Cryptocurrency Market Data Ingestion

## Task: Binance Public API -> PostgreSQL via Go Workers

### Task Understanding

Build a Go-based data ingestion service that polls Binance's public REST API for cryptocurrency market data (BTC, ETH, USDT trading pairs) and stores trades, order book snapshots, and 24hr ticker data into PostgreSQL. The service runs as a Docker container alongside the existing data engineering stack. This is a greenfield Go project -- no Go code exists in the repo yet.

### Chosen Approach

**Binance REST API with polling** -- Single Go binary (`crypto-ingest`) using goroutine-per-symbol-per-data-type workers. Polling at 5-second intervals for trades and order book, 30-second intervals for ticker. Data written to PostgreSQL via `pgx/v5` with batch inserts and `ON CONFLICT DO NOTHING` deduplication for trades. Resumable ingestion via high-water mark table.

**Why this won (unanimous across 12 agents, 3 independent teams):**
- Binance is the largest exchange by volume, truly public API for market data, no auth required, generous rate limits (6000 weight/min)
- REST polling is simpler than WebSocket for v1 -- no connection lifecycle management, reconnection logic, or partial message handling
- Single binary with goroutines is appropriate for 3 symbols x 3 data types (9 workers) -- no microservice overhead needed
- `/api/v3/aggTrades` (weight=2) is 12.5x cheaper than `/api/v3/trades` (weight=25) on rate budget

### Rejected Alternatives

- **WebSocket streaming** -- Adds connection lifecycle management, heartbeat logic, reconnection with gap detection. Overkill for 3 symbols at 5s intervals. Defer to v1.1. (All 3 teams agree)
- **Multiple exchanges** -- Scope creep. Binance alone covers all three assets. Multi-exchange support can be added later. (All 3 teams agree)
- **Microservice per data type** -- Unnecessary coordination cost for 9 workers. Single binary is simpler to deploy, monitor, and debug. (All 3 teams agree)
- **Historical backfill in v1** -- `/api/v3/historicalTrades` requires API key. Start with real-time forward-only ingestion. (All 3 teams agree)
- **Prometheus metrics in v1** -- Adds dependency for an internal dev tool. Structured `slog` logging is sufficient. Defer to v1.1. (All 3 teams agree)
- **FATAL exit on HTTP 418** -- With `restart: unless-stopped` in docker-compose, fatal exit creates a restart loop that hammers Binance repeatedly, potentially extending the IP ban duration. Stop workers + 503 healthz is strictly superior. (Charlie-Skeptic won this debate; Alpha+Bravo conceded)
- **Premature Fetcher interface** -- No interface abstraction in v1. Concrete Binance client only. Defer interface extraction to multi-exchange milestone. (Alpha-Skeptic, adopted by all)

### Binance API Endpoints (Verified by 3 independent Verifiers)

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
  +-- Server-Side Rate Monitor (X-MBX-USED-WEIGHT-1m header, pause at 95%)
  +-- PostgreSQL Batch Writer (pgx.Batch + ON CONFLICT for trades, COPY for orderbook/ticker)
        |
        v
PostgreSQL (existing `pipeline` database, new `crypto` schema)
  +-- crypto.symbols              (symbol metadata)
  +-- crypto.agg_trades           (executed trades, deduped by agg_trade_id)
  +-- crypto.orderbook_snapshots  (snapshot headers with depth_level)
  +-- crypto.orderbook_levels     (individual price levels per snapshot, no FK)
  +-- crypto.ticker_24hr          (24hr rolling stats, full column set)
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

**Files to CREATE (~16 new files):**

| File | Purpose |
|------|---------|
| `crypto-ingest/go.mod` | Go module definition |
| `crypto-ingest/cmd/ingest/main.go` | Application entrypoint, config validation, wiring |
| `crypto-ingest/internal/config/config.go` | Environment variable parsing |
| `crypto-ingest/internal/client/binance.go` | Binance REST API client + response types |
| `crypto-ingest/internal/client/ratelimit.go` | Token bucket wrapper (`golang.org/x/time/rate`) |
| `crypto-ingest/internal/model/*.go` | Structs for AggTrade, OrderBook, Ticker, Symbol (coder decides file granularity) |
| `crypto-ingest/internal/worker/trade.go` | Trade polling goroutine |
| `crypto-ingest/internal/worker/orderbook.go` | Order book polling goroutine |
| `crypto-ingest/internal/worker/ticker.go` | Ticker polling goroutine |
| `crypto-ingest/internal/worker/manager.go` | Worker lifecycle, graceful shutdown, health endpoint |
| `crypto-ingest/internal/store/postgres.go` | pgx pool, batch inserts, COPY for orderbook/ticker |
| `crypto-ingest/internal/store/migrations.go` | CREATE SCHEMA/TABLE IF NOT EXISTS on startup |
| `crypto-ingest/Dockerfile` | Multi-stage build |
| `scripts/verify_crypto_e2e.sh` | Automated E2E check |

**Files to MODIFY (2 files):**

| File | Change |
|------|--------|
| `docker/postgres/init.sql` | Add `CREATE SCHEMA IF NOT EXISTS crypto;` |
| `docker-compose.yml` | Add `crypto-ingest` service with healthcheck |

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

**crypto.orderbook_levels** (no FK -- unanimous across all 12 agents)
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

**crypto.ticker_24hr** (fuller column set -- Bravo, adopted by swarm)
```sql
CREATE TABLE IF NOT EXISTS crypto.ticker_24hr (
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

### Key Implementation Decisions (from 12-agent swarm debate)

| Decision | Rationale | Resolved By |
|----------|-----------|-------------|
| **No `shopspring/decimal`** -- pass Binance price strings directly to PG as NUMERIC | Binance returns prices as strings; Go code does not compute with them. PG's NUMERIC parser handles all edge cases. | All 12 agents unanimous |
| **No FK on `orderbook_levels`** | ~2.1M inserts/day, FK lookup adds overhead. Referential integrity guaranteed by application logic. | All 12 agents unanimous |
| **Atomic `ingest_state` updates** | Wrap trade batch INSERT + ingest_state UPDATE in same pgx transaction. Prevents wasted re-fetch on crash recovery. | All 12 agents unanimous |
| **Orderbook depth default: 20** (not 100) | Weight drops from 5 to 2 per request. Volume drops from ~10.4M to ~2.1M rows/day. Configurable via env var. | All 12 agents unanimous |
| **HTTP 418 = stop workers + 503** (NOT fatal exit) | With `restart: unless-stopped`, fatal exit creates restart loop hammering Binance, extending ban. Stop workers, return 503 on /healthz with ban reason, log ERROR. Operator sees unhealthy container and intervenes. | Charlie-Skeptic won; Alpha+Bravo conceded (restart loop argument) |
| **pgx pool: MaxConns=12, AcquireTimeout=5s** | 9 workers + health check + migrations + buffer = 12 connections. AcquireTimeout prevents indefinite blocking on pool exhaustion. | Bravo + Charlie, adopted by all |
| **Batch insert count logging** | Log rows_attempted vs rows_inserted per batch. ON CONFLICT DO NOTHING silently drops rows -- this is the only way to detect it. | Charlie, adopted by all |
| **Server-side rate monitoring (X-MBX-USED-WEIGHT-1m)** | Read header on every response. Warn at 80% of budget, pause all workers at 95%. Defense in depth: token bucket is proactive (local estimate), header is reactive (server truth). | Alpha, adopted by all |
| **Exponential backoff with jitter** | Prevents thundering herd when multiple workers retry after shared failure. Formula: `delay * (0.5 + rand.Float64())`. | Bravo, adopted by all |
| **Log raw response on unmarshal error** | Log first 500 bytes of response body on JSON parse failure. Essential for debugging API format changes. | Charlie, adopted by all |
| **No premature Fetcher interface in v1** | Concrete Binance client only. Defer interface extraction to multi-exchange milestone. | Alpha-Skeptic, adopted by all |
| **Validate Binance response structure before insert** | Fail loudly on unexpected schema changes. Never silently insert malformed data. | Alpha-Skeptic, adopted by all |
| **Fuller ticker_24hr columns** | Zero additional API calls. Includes: last_qty, bid_qty, ask_qty, first_trade_id, last_trade_id. Complete data fidelity. | Bravo, adopted by all |
| **Health endpoint at `:8085/healthz`** + `--healthcheck` self-probe | Port 8085 verified free. Distroless has no shell/curl/wget, so binary self-probes. | All 12 agents unanimous |
| **`stop_grace_period: 15s`** | Application flush deadline 10s, Docker gives 15s before SIGKILL. | All 12 agents unanimous |
| **Startup Binance ping + fail fast** | Geo-blocking is the #1 project risk. Clear error message listing alternative URLs + HTTP_PROXY support. | All 12 agents unanimous |
| **E2E verification script** | CLAUDE.md mandates E2E testing. Script checks row counts > 0 after 60s. | All 12 agents unanimous |

### Error Handling Strategy (from swarm debate)

| HTTP Status | Action | Rationale |
|-------------|--------|-----------|
| **200** | Process response, update ingest_state | Normal operation |
| **400** | Log error at WARN, skip cycle, no retry | Client error, retrying won't help |
| **418** | **Stop ALL workers, /healthz returns 503 with ban reason, log ERROR, do NOT exit** | IP ban. Container stays running but unhealthy. Prevents restart loop. Manual intervention required. |
| **429** | Respect `Retry-After` header, exponential backoff with jitter, max 3 retries | Rate limited. Backoff respects server guidance. |
| **5xx** | Exponential backoff with jitter (`delay * (0.5 + rand.Float64())`), max 3 retries | Server error, transient. |
| **Network error** | Exponential backoff with jitter, max 3 retries | Transient connectivity issue. |
| **JSON unmarshal error** | Log first 500 bytes of raw response at ERROR, skip cycle | API format change detection. |

### Execution Order

0. **Step 0: User verifies Binance connectivity** -- Run `curl -s https://api.binance.com/api/v3/ping` from deployment network. If this fails, resolve connectivity (VPN, proxy, alternative URL) before proceeding. **Do not write any code until this passes.**

1. **Modify `docker/postgres/init.sql`** -- add `CREATE SCHEMA IF NOT EXISTS crypto;`
   - Why first: establishes schema for fresh volumes; Go app also creates it on startup for existing volumes

2. **Create `crypto-ingest/go.mod`** -- initialize Go module with dependencies
   - Dependencies: `github.com/jackc/pgx/v5`, `golang.org/x/time`
   - Why second: all Go files need the module

3. **Create model structs** (`internal/model/`)
   - Why third: pure data types with zero dependencies, used by all other packages

4. **Create config** (`internal/config/config.go`)
   - Parse env vars: `DATABASE_URL`, `BINANCE_BASE_URL`, `SYMBOLS`, poll intervals, `ORDERBOOK_DEPTH`, `HEALTH_PORT`, `LOG_LEVEL`, `MAX_RETRIES`, `RETRY_BASE_DELAY`, `PG_MAX_CONNS`, `PG_ACQUIRE_TIMEOUT`
   - Validate all required vars on startup, fail fast with clear error
   - Why fourth: needed by client, store, and worker packages

5. **Create rate limiter** (`internal/client/ratelimit.go`)
   - Wrap `golang.org/x/time/rate.Limiter`, acquire tokens per API weight
   - Why fifth: needed by Binance client

6. **Create Binance client** (`internal/client/binance.go`)
   - HTTP client with rate limiter, response parsing, startup ping check
   - Read `X-MBX-USED-WEIGHT-1m` header on every response: warn at 80% (4800), pause all workers at 95% (5700)
   - Validate non-empty price/qty strings before returning
   - Log first 500 bytes of raw response on JSON unmarshal error
   - Clear startup error message with alternative Binance URLs + HTTP_PROXY note
   - Why sixth: depends on models + rate limiter

7. **Create PostgreSQL store** (`internal/store/postgres.go` + `migrations.go`)
   - `migrations.go`: `CREATE SCHEMA IF NOT EXISTS crypto` + all `CREATE TABLE IF NOT EXISTS`
   - `postgres.go`: pgx pool (MaxConns=12, AcquireTimeout=5s), batch trade inserts (pgx.Batch + ON CONFLICT DO NOTHING), COPY for orderbook levels, simple INSERT for ticker, atomic ingest_state updates
   - Log rows_attempted vs rows_inserted on every batch insert
   - Why seventh: depends on models, parallel with step 6

8. **Create workers** (`internal/worker/*.go`)
   - `trade.go`: poll aggTrades, resume from ingest_state high-water mark (fromId = last_id + 1)
   - `orderbook.go`: poll depth, insert snapshot + levels in transaction
   - `ticker.go`: poll 24hr ticker
   - `manager.go`: start all workers, health endpoint (200 when healthy, 503 on IP ban), graceful shutdown (SIGINT/SIGTERM, 10s flush deadline), 418 handler (stop all workers, set health to 503)
   - Why eighth: depends on client + store

9. **Create entrypoint** (`cmd/ingest/main.go`)
   - Wire config -> client -> store -> workers -> manager
   - Validate config, run migrations, ping Binance, start workers
   - Include `--healthcheck` flag for distroless self-probe
   - Why ninth: depends on all internal packages

10. **Create Dockerfile**
    - Multi-stage: `golang:1.22.12-alpine3.21` build stage -> `gcr.io/distroless/static-debian12:nonroot` runtime
    - Why tenth: needs compilable Go code

11. **Modify `docker-compose.yml`**
    - Add `crypto-ingest` service with: environment from `.env`, `depends_on: postgres: condition: service_healthy`, `mem_limit: 256m`, `stop_grace_period: 15s`, healthcheck using `["/crypto-ingest", "--healthcheck"]`, network `data-pipeline`, `restart: unless-stopped`
    - Why eleventh: needs Dockerfile

12. **Update `.env.example`**
    - Add crypto-ingest variables: `DATABASE_URL`, `BINANCE_BASE_URL`, `SYMBOLS`, poll intervals, etc.
    - Why twelfth: documents new configuration

13. **Create `scripts/verify_crypto_e2e.sh`**
    - Wait 60s, query row counts for all crypto tables, assert all > 0
    - Why thirteenth: needs running stack

14. **E2E test**
    - `docker compose up -d`, wait for healthy, run verification script
    - Why last: validates everything

### Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Binance API inaccessible (geo-restrictions for Russia/CIS/US) | No data ingested | Step 0 manual check; `BINANCE_BASE_URL` configurable; alternatives `api1/api2/api3.binance.com`; `HTTP_PROXY`/`HTTPS_PROXY` env var support (native in Go net/http); startup ping fails fast with clear error listing alternatives |
| Float precision loss on prices/quantities | Incorrect financial data | Pass Binance string prices directly to PG NUMERIC; never use float64; validate non-empty before insert; DoD: grep for zero float64 in model structs |
| Rate limit exceeded -> HTTP 429 or 418 IP ban | Ingestion stops | Token bucket at 5000/min (83% of 6000); server-side X-MBX-USED-WEIGHT-1m monitoring (pause at 95%); exponential backoff with jitter on 429; 418 stops workers + 503 healthz (no restart loop) |
| `init.sql` not re-run on existing Postgres volume | `crypto` schema missing | Go app runs `CREATE SCHEMA/TABLE IF NOT EXISTS` on startup independently |
| Orderbook data volume (~2.1M rows/day at depth=20) | Disk fills over weeks | Configurable depth/interval; retention strategy deferred to v1.1 |
| Silent data loss from ON CONFLICT DO NOTHING | Undetected missing trades | Batch insert count logging: log rows_attempted vs rows_inserted per batch |
| pgx pool exhaustion with 9 concurrent workers | Workers block indefinitely | MaxConns=12, AcquireTimeout=5s -- fail fast, retry next cycle |
| Binance API response format change | Parse errors, no data | Log first 500 bytes of raw response on unmarshal error; validate response structure before insert |
| Docker restart loop on IP ban | Hammers Binance, extends ban | 418 = stop workers + 503 (NOT fatal exit); container stays running but unhealthy |
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
| `ORDERBOOK_DEPTH` | `20` | Order book levels per side |
| `TICKER_POLL_INTERVAL` | `30s` | Ticker worker polling interval |
| `HEALTH_PORT` | `8085` | Health endpoint listen port |
| `LOG_LEVEL` | `info` | Structured log level (debug, info, warn, error) |
| `MAX_RETRIES` | `3` | Max retries per failed API call |
| `RETRY_BASE_DELAY` | `1s` | Base delay for exponential backoff |
| `PG_MAX_CONNS` | `12` | pgx pool maximum connections |
| `PG_ACQUIRE_TIMEOUT` | `5s` | pgx pool connection acquisition timeout |

Go's `net/http` automatically respects `HTTP_PROXY`/`HTTPS_PROXY` environment variables -- no code needed, just set in docker-compose environment for geo-blocking mitigation.

### Go Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/jackc/pgx/v5` | v5.7.x | PostgreSQL driver with COPY, batch, pool support |
| `golang.org/x/time` | v0.9.x | Token bucket rate limiter (`rate.Limiter`) |

Two external dependencies total. `log/slog` is stdlib (Go 1.21+).

### Definition of Done

**Pre-implementation Gate:**
- [ ] User confirmed `curl -s https://api.binance.com/api/v3/ping` returns `{}` from deployment network

**Infrastructure:**
- [ ] `docker/postgres/init.sql` includes `CREATE SCHEMA IF NOT EXISTS crypto;`
- [ ] `docker-compose.yml` includes `crypto-ingest` service with healthcheck, `depends_on`, `stop_grace_period: 15s`, `mem_limit: 256m`, `restart: unless-stopped`
- [ ] `crypto-ingest/Dockerfile` uses pinned multi-stage build: `golang:1.22.12-alpine3.21` -> `gcr.io/distroless/static-debian12:nonroot`
- [ ] `docker compose up -d` brings up `crypto-ingest` alongside existing services without errors
- [ ] `.env.example` updated with crypto-ingest variables

**Go Application:**
- [ ] `go.mod` has exactly 2 external dependencies: `pgx/v5`, `golang.org/x/time`
- [ ] `shopspring/decimal` is NOT in `go.mod`
- [ ] **Zero `float64` in any model struct** -- grep-verifiable: `grep -r "float64" crypto-ingest/internal/model/` returns nothing
- [ ] **No comments in Go code** -- per CLAUDE.md
- [ ] Application validates all required env vars on startup and exits with clear error if missing
- [ ] Application pings Binance API on startup and fails fast with error listing alternative URLs + HTTP_PROXY
- [ ] Application creates `crypto` schema and all tables on startup via `CREATE IF NOT EXISTS`
- [ ] pgx pool configured with MaxConns=12, AcquireTimeout=5s

**Data Ingestion:**
- [ ] Trade workers poll `/api/v3/aggTrades` for BTCUSDT, ETHUSDT, ETHBTC
- [ ] Order book workers poll `/api/v3/depth?limit=20` for all 3 symbols
- [ ] Ticker workers poll `/api/v3/ticker/24hr` for all 3 symbols
- [ ] Exchange metadata loaded from `/api/v3/exchangeInfo` once at startup
- [ ] All workers respect shared token bucket rate limiter (5000 weight/min)
- [ ] X-MBX-USED-WEIGHT-1m header read on every response: warn at 80%, pause at 95%
- [ ] Workers retry failed requests with exponential backoff + jitter (max 3 retries)
- [ ] Batch insert count logged: rows_attempted vs rows_inserted per batch

**Error Handling:**
- [ ] HTTP 418: stop ALL workers, /healthz returns 503 with ban reason, do NOT exit process
- [ ] HTTP 429: respect Retry-After header, exponential backoff with jitter, max 3 retries
- [ ] HTTP 400: log WARN, skip cycle, no retry
- [ ] HTTP 5xx: exponential backoff with jitter, max 3 retries
- [ ] JSON unmarshal error: log first 500 bytes of raw response at ERROR
- [ ] Binance response structure validated before insert

**Schema:**
- [ ] `crypto.symbols` populated with 3 symbol records at startup
- [ ] `crypto.agg_trades` has composite PK `(symbol, agg_trade_id)` and uses `ON CONFLICT DO NOTHING`
- [ ] `crypto.orderbook_snapshots` has BIGSERIAL PK and `depth_level` column
- [ ] `crypto.orderbook_levels` has NO foreign key constraint on `snapshot_id`
- [ ] `crypto.ticker_24hr` includes full column set (last_qty, bid_qty, ask_qty, first_trade_id, last_trade_id)
- [ ] `crypto.ingest_state` stores high-water marks per (worker_type, symbol)
- [ ] All prices/quantities stored as NUMERIC (string passthrough from Binance)
- [ ] All timestamps stored as TIMESTAMPTZ

**Idempotency and Resumability:**
- [ ] Trade batch inserts and `ingest_state` updates occur in the same pgx transaction
- [ ] On restart, trade workers resume from `last_id + 1` in `ingest_state`
- [ ] Duplicate trades are silently skipped via `ON CONFLICT DO NOTHING`

**Operational:**
- [ ] Health endpoint at `:8085/healthz` returns 200 when healthy, 503 when IP-banned
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
- [ ] `crypto.ticker_24hr` contains 24hr stats for all 3 symbols with full column data
- [ ] `crypto.symbols` contains 3 records (BTCUSDT, ETHUSDT, ETHBTC)

### Validation Checklist

- [ ] `docker compose up -d` -- all services healthy including crypto-ingest
- [ ] `docker compose logs crypto-ingest` -- no errors, structured JSON logs, batch insert counts visible
- [ ] `curl localhost:8085/healthz` -- returns 200
- [ ] `scripts/verify_crypto_e2e.sh` -- all checks pass after 60s
- [ ] Stop and restart crypto-ingest -- resumes from last ingested trade ID (no duplicates)
- [ ] `docker compose down && docker compose up -d` -- crypto-ingest creates schema/tables on startup
- [ ] `grep -r "float64" crypto-ingest/internal/model/` -- returns nothing

### Open Questions

- **Binance geo-availability:** If the user's network blocks Binance, they may need to use a proxy (`HTTP_PROXY`/`HTTPS_PROXY`), VPN, or switch to alternative endpoints (`api1/api2/api3.binance.com`). **Step 0 tests this before any code is written.**
- **Go Docker image tag:** `golang:1.22.12-alpine3.21` verified as stable by all 3 Verifier agents. Code is compatible with Go 1.22+.

### Deferred to v1.1

- WebSocket streaming (real-time push instead of REST polling)
- Historical trade backfill (`/api/v3/historicalTrades` -- requires API key)
- Order book depth > 20 levels (configurable via env var, just change default)
- Prometheus metrics endpoint
- Data retention policy (partitioning or batched deletes for orderbook data)
- `exchange` column on all tables (for multi-exchange support)
- Kline/candlestick ingestion (`/api/v3/klines`)
- Multiple exchange support
- Auto-resume cooldown after IP ban (BAN_COOLDOWN timer)

### Swarm Notes

**Key disagreements resolved:**

1. **HTTP 418 handling (biggest debate):** Alpha and Bravo initially proposed FATAL exit (shutdown binary). Charlie-Skeptic argued this creates a restart loop with `restart: unless-stopped` that hammers Binance and extends the ban. Alpha-Verifier clinched it by confirming no existing service in docker-compose uses restart policies -- adding one with FATAL creates exactly this failure mode. **Resolution: stop workers + 503 healthz (Charlie-Skeptic's approach). 10 of 12 agents explicitly conceded.**

2. **Ticker_24hr column completeness:** Alpha had a leaner column set. Bravo argued for full columns (last_qty, bid_qty, ask_qty, first_trade_id, last_trade_id) since they're free -- zero additional API calls, parsed from the same JSON response. **Resolution: fuller columns (Bravo's approach).**

3. **pgx pool configuration:** Alpha didn't specify pool settings. Bravo proposed MaxConns=12. Charlie added AcquireTimeout=5s to prevent indefinite blocking. **Resolution: MaxConns=12 + AcquireTimeout=5s (Charlie's more complete specification).**

4. **File count (14 vs 16 vs 17):** Structural consolidation question -- whether to merge model files, use embed.FS for schema. **Resolution: left to coder agent's discretion. All approaches are functionally identical.**

5. **Batch insert observability:** Alpha and Bravo didn't specify insert count logging. Charlie proposed logging rows_attempted vs rows_inserted to detect silent data loss from ON CONFLICT DO NOTHING. **Resolution: adopted (Charlie's addition). ~3 lines of code, high observability value.**

6. **429 retry count:** Brief debate between 1 retry (initial Bravo position) and 3 retries (Alpha/Charlie). **Resolution: max 3 retries with exponential backoff + jitter.**

**Confidence level: HIGH**

All 12 agents across 3 independent teams converged on the same architecture, schema, endpoints, dependencies, and Docker approach. The only substantive debate (418 handling) was resolved with a clear technical winner. Every repo fact was verified by 3 independent Verifier agents -- no speculative claims survived.

---

*Plan produced by 12-agent adversarial swarm forum: 3 independent teams of 4 (architect, skeptic, pragmatist, verifier) each debated internally, presented cross-team, then all 12 agents converged through full-swarm debate. Synthesis applied resolution rules: verifiers win on repo facts, skeptics on risk identification, pragmatists on simplification, architects on structure. Where teams disagreed, the strongest technical argument prevailed regardless of team origin.*
