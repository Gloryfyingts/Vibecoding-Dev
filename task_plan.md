# Task: Cryptocurrency Market Data Ingestion via Binance Public API into PostgreSQL using Go Workers

---

## 1. Exchange Selection: Binance

**Why Binance:**
- Largest cryptocurrency exchange by trading volume globally
- Fully public REST API for market data -- no API key or authentication required for read-only market endpoints
- WebSocket streams available for real-time data (future enhancement)
- Supports all three required assets: BTC, ETH, USDT (as quote/base currencies)
- Generous rate limits for public endpoints (6000 request weight per minute for IP-based limits)
- Stable, well-documented API with versioned endpoints
- Base URL: `https://api.binance.com`

**Trading Pairs to Ingest:**

| Symbol       | Base  | Quote |
|--------------|-------|-------|
| `BTCUSDT`    | BTC   | USDT  |
| `ETHUSDT`    | ETH   | USDT  |
| `ETHBTC`     | ETH   | BTC   |

These three pairs cover all required assets (BTC, ETH, USDT) and their primary trading relationships.

---

## 2. Binance Public API Endpoints

All endpoints below are **public** and require **no authentication** (no API key, no HMAC signing, no account).

### 2.1 Recent Trades

**Endpoint:** `GET /api/v3/trades`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `symbol`  | Yes      | e.g., `BTCUSDT` |
| `limit`   | No       | Default 500, max 1000 |

**Response fields:**
- `id` -- trade ID (unique per symbol, monotonically increasing)
- `price` -- trade price as string
- `qty` -- trade quantity as string
- `quoteQty` -- quote asset quantity (price * qty)
- `time` -- trade timestamp in milliseconds (Unix epoch)
- `isBuyerMaker` -- boolean: true if the buyer was the maker
- `isBestMatch` -- boolean: true if the trade was the best price match

**Rate limit weight:** 25 per request (at limit=1000)

### 2.2 Historical Trades (Older Trades)

**Endpoint:** `GET /api/v3/historicalTrades`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `symbol`  | Yes      | e.g., `BTCUSDT` |
| `limit`   | No       | Default 500, max 1000 |
| `fromId`  | No       | Trade ID to fetch from (for pagination) |

**Note:** This endpoint requires an API key header (`X-MBX-APIKEY`) but does NOT require signing (no secret key needed). A free Binance API key (no account balance needed) unlocks this. For initial implementation, use `/api/v3/trades` (fully public, no key). Add historical trades support later if backfill is needed.

**Rate limit weight:** 25 per request

### 2.3 Aggregate Trades (Compressed Trades)

**Endpoint:** `GET /api/v3/aggTrades`

| Parameter   | Required | Description |
|-------------|----------|-------------|
| `symbol`    | Yes      | e.g., `BTCUSDT` |
| `fromId`    | No       | Aggregate trade ID to fetch from |
| `startTime` | No       | Start timestamp (ms) |
| `endTime`   | No       | End timestamp (ms) |
| `limit`     | No       | Default 500, max 1000 |

**Response fields:**
- `a` -- aggregate trade ID
- `p` -- price
- `q` -- quantity
- `f` -- first trade ID
- `l` -- last trade ID
- `T` -- timestamp (ms)
- `m` -- was the buyer the maker?
- `M` -- was the trade the best price match?

**Rate limit weight:** 2 per request (much cheaper than /trades)

This is the **preferred endpoint for trade ingestion** due to low rate limit cost, time-range filtering, and pagination by ID.

### 2.4 Order Book / Market Depth

**Endpoint:** `GET /api/v3/depth`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `symbol`  | Yes      | e.g., `BTCUSDT` |
| `limit`   | No       | Default 100. Valid: 5, 10, 20, 50, 100, 500, 1000, 5000 |

**Response fields:**
- `lastUpdateId` -- sequence number for the snapshot
- `bids` -- array of `[price, quantity]` pairs (sorted best to worst)
- `asks` -- array of `[price, quantity]` pairs (sorted best to worst)

**Rate limit weight:** 5 (limit=100), 10 (limit=500), 50 (limit=1000), 250 (limit=5000)

### 2.5 Ticker Price (Latest Price)

**Endpoint:** `GET /api/v3/ticker/price`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `symbol`  | No       | Single symbol. Omit for all symbols. |

**Response fields:**
- `symbol` -- trading pair
- `price` -- latest price as string

**Rate limit weight:** 2 (single symbol), 4 (all symbols)

### 2.6 24hr Ticker Statistics

**Endpoint:** `GET /api/v3/ticker/24hr`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `symbol`  | No       | Single symbol |

**Response fields (selected):**
- `symbol`, `priceChange`, `priceChangePercent`
- `weightedAvgPrice`, `prevClosePrice`
- `lastPrice`, `lastQty`
- `bidPrice`, `bidQty`, `askPrice`, `askQty`
- `openPrice`, `highPrice`, `lowPrice`
- `volume` (base), `quoteVolume` (quote)
- `openTime`, `closeTime`
- `firstId`, `lastId`, `count` (trade count)

**Rate limit weight:** 2 (single symbol), 80 (all symbols)

### 2.7 Kline/Candlestick Data

**Endpoint:** `GET /api/v3/klines`

| Parameter   | Required | Description |
|-------------|----------|-------------|
| `symbol`    | Yes      | e.g., `BTCUSDT` |
| `interval`  | Yes      | e.g., `1m`, `5m`, `1h`, `1d` |
| `startTime` | No       | Start timestamp (ms) |
| `endTime`   | No       | End timestamp (ms) |
| `limit`     | No       | Default 500, max 1000 |

**Response:** Array of arrays: `[openTime, open, high, low, close, volume, closeTime, quoteAssetVolume, numberOfTrades, takerBuyBaseVol, takerBuyQuoteVol, ignore]`

**Rate limit weight:** 2 per request

### 2.8 Exchange Information

**Endpoint:** `GET /api/v3/exchangeInfo`

| Parameter  | Required | Description |
|------------|----------|-------------|
| `symbol`   | No       | Single symbol |
| `symbols`  | No       | JSON array of symbols |

**Response fields (per symbol):**
- `symbol`, `status`, `baseAsset`, `quoteAsset`
- `baseAssetPrecision`, `quoteAssetPrecision`, `quotePrecision`
- `filters` -- array of trading rules (price filter, lot size, etc.)
- `orderTypes`, `icebergAllowed`, `isSpotTradingAllowed`
- `permissions`

**Rate limit weight:** 20

### 2.9 Rate Limits Summary

Binance enforces IP-based rate limits on public endpoints:
- **6000 weight per minute** per IP for REST API
- Each endpoint has a "weight" cost (listed above)
- Exceeding limits returns HTTP 429 with `Retry-After` header
- Response headers include: `X-MBX-USED-WEIGHT-1m` (current weight usage)

**Budget calculation for our use case (3 symbols, polling every 5 seconds):**

| Endpoint       | Weight | Calls/min | Total Weight/min |
|----------------|--------|-----------|------------------|
| aggTrades x3   | 2      | 36        | 72               |
| depth x3       | 5      | 36        | 180              |
| ticker/24hr x3 | 2      | 12        | 24               |
| **Total**      |        |           | **276**          |

This uses ~4.6% of the 6000/min budget, leaving substantial headroom.

---

## 3. Overall Architecture

```
+-------------------+     +------------------+     +------------+
|  Binance REST API |---->|  Go Worker Pool  |---->| PostgreSQL |
|  (public, no auth)|     |  (3 goroutines   |     | (pipeline  |
+-------------------+     |   per data type)  |     |  database) |
                          +------------------+     +------------+
                                  |
                          +-------v--------+
                          |  Rate Limiter  |
                          |  (token bucket)|
                          +----------------+
```

### Components

1. **Go binary: `crypto-ingest`** -- single binary, runs as a Docker container alongside the existing stack
2. **Three worker types:**
   - **Trade Worker** -- polls `/api/v3/aggTrades`, one goroutine per symbol (3 total)
   - **OrderBook Worker** -- polls `/api/v3/depth`, one goroutine per symbol (3 total)
   - **Ticker Worker** -- polls `/api/v3/ticker/24hr`, one goroutine per symbol (3 total)
3. **Shared rate limiter** -- token bucket, 6000 tokens/minute capacity, shared across all goroutines
4. **PostgreSQL writer** -- batched inserts using `pgx` copy protocol for high throughput
5. **Exchange metadata loader** -- fetches `/api/v3/exchangeInfo` once at startup, refreshes hourly

### Data Flow

1. Worker goroutine wakes up on ticker interval
2. Acquires rate limit tokens (weight of the request)
3. Makes HTTP GET to Binance API
4. Parses JSON response, normalizes to internal structs
5. Deduplicates against last-seen IDs (for trades) or timestamps (for snapshots)
6. Batches rows and flushes to PostgreSQL via COPY protocol
7. Updates high-water marks (last trade ID, last snapshot timestamp)
8. Sleeps until next tick

---

## 4. Go Worker Architecture

### 4.1 Project Structure

```
crypto-ingest/
  cmd/
    ingest/
      main.go              -- entrypoint, config loading, signal handling
  internal/
    config/
      config.go            -- configuration struct, env/flag parsing
    client/
      binance.go           -- HTTP client for Binance API, response types
      ratelimit.go         -- token bucket rate limiter
    model/
      trade.go             -- AggTrade struct
      orderbook.go         -- OrderBookSnapshot, OrderBookLevel structs
      ticker.go            -- Ticker24hr struct
      symbol.go            -- SymbolInfo struct (from exchangeInfo)
    worker/
      trade.go             -- trade polling worker
      orderbook.go         -- order book polling worker
      ticker.go            -- ticker polling worker
      manager.go           -- starts/stops all workers, coordinates shutdown
    store/
      postgres.go          -- pgx connection pool, batch insert methods
      migrations.go        -- schema creation (DDL execution on startup)
  go.mod
  go.sum
  Dockerfile
```

### 4.2 Configuration (`internal/config/config.go`)

All configuration via environment variables (12-factor app):

| Env Variable              | Default                          | Description |
|---------------------------|----------------------------------|-------------|
| `BINANCE_BASE_URL`        | `https://api.binance.com`        | API base URL |
| `BINANCE_SYMBOLS`         | `BTCUSDT,ETHUSDT,ETHBTC`        | Comma-separated symbols |
| `TRADE_POLL_INTERVAL`     | `5s`                             | How often to poll aggTrades |
| `ORDERBOOK_POLL_INTERVAL` | `5s`                             | How often to poll depth |
| `ORDERBOOK_DEPTH`         | `100`                            | Depth levels to fetch (5/10/20/50/100/500/1000) |
| `TICKER_POLL_INTERVAL`    | `30s`                            | How often to poll 24hr ticker |
| `RATE_LIMIT_PER_MINUTE`   | `5000`                           | Token bucket capacity (below Binance's 6000 for safety) |
| `BATCH_SIZE`              | `500`                            | Rows per batch insert |
| `BATCH_FLUSH_INTERVAL`    | `2s`                             | Max time before flushing partial batch |
| `POSTGRES_DSN`            | (required, from `.env`)          | PostgreSQL connection string |
| `LOG_LEVEL`               | `info`                           | Logging level: debug/info/warn/error |

### 4.3 HTTP Client (`internal/client/binance.go`)

- Use Go standard library `net/http` with a shared `http.Client` (connection pooling via default transport)
- Set `http.Client.Timeout` to 10 seconds
- Parse JSON responses using `encoding/json` (no external JSON library needed)
- Read `X-MBX-USED-WEIGHT-1m` header from every response to track actual rate limit usage
- On HTTP 429: log warning, read `Retry-After` header, sleep for that duration, then retry once
- On HTTP 418 (IP ban): log error, halt all workers, exit with code 1 (requires manual intervention)
- On HTTP 5xx: retry with exponential backoff (1s, 2s, 4s, max 3 retries)

Response structs:

```go
type AggTradeResponse struct {
    AggTradeID   int64  `json:"a"`
    Price        string `json:"p"`
    Quantity     string `json:"q"`
    FirstTradeID int64  `json:"f"`
    LastTradeID  int64  `json:"l"`
    Timestamp    int64  `json:"T"`
    IsBuyerMaker bool   `json:"m"`
    IsBestMatch  bool   `json:"M"`
}

type DepthResponse struct {
    LastUpdateID int64      `json:"lastUpdateId"`
    Bids         [][]string `json:"bids"`
    Asks         [][]string `json:"asks"`
}

type Ticker24hrResponse struct {
    Symbol             string `json:"symbol"`
    PriceChange        string `json:"priceChange"`
    PriceChangePercent string `json:"priceChangePercent"`
    WeightedAvgPrice   string `json:"weightedAvgPrice"`
    LastPrice          string `json:"lastPrice"`
    LastQty            string `json:"lastQty"`
    BidPrice           string `json:"bidPrice"`
    BidQty             string `json:"bidQty"`
    AskPrice           string `json:"askPrice"`
    AskQty             string `json:"askQty"`
    OpenPrice          string `json:"openPrice"`
    HighPrice          string `json:"highPrice"`
    LowPrice           string `json:"lowPrice"`
    Volume             string `json:"volume"`
    QuoteVolume        string `json:"quoteVolume"`
    OpenTime           int64  `json:"openTime"`
    CloseTime          int64  `json:"closeTime"`
    FirstID            int64  `json:"firstId"`
    LastID             int64  `json:"lastId"`
    Count              int64  `json:"count"`
}
```

### 4.4 Rate Limiter (`internal/client/ratelimit.go`)

Token bucket implementation:
- Capacity: configurable (default 5000 tokens per minute, below Binance's 6000 limit for safety margin)
- Refill: tokens refill continuously (5000/60 = ~83.3 tokens/second)
- Each API call consumes tokens equal to its weight before executing
- If insufficient tokens, the goroutine blocks until tokens are available
- Use `golang.org/x/time/rate` package (`rate.NewLimiter`) -- it implements token bucket natively
- `rate.NewLimiter(rate.Limit(5000.0/60.0), 5000)` -- rate of ~83.3/sec, burst of 5000

Usage:
```go
limiter.WaitN(ctx, weight)  // blocks until weight tokens available
```

### 4.5 Trade Worker (`internal/worker/trade.go`)

Per-symbol goroutine logic:

1. On startup, query PostgreSQL for the highest `agg_trade_id` for this symbol -- this is the high-water mark
2. Loop on ticker interval (default 5s):
   a. Call `limiter.WaitN(ctx, 2)` (aggTrades weight = 2)
   b. `GET /api/v3/aggTrades?symbol={symbol}&fromId={lastID+1}&limit=1000`
   c. If `lastID` is 0 (first run), omit `fromId` to get most recent trades
   d. Parse response into `[]AggTradeResponse`
   e. Filter out any trades with `AggTradeID <= lastID` (safety dedup)
   f. Convert to internal model, batch insert into `crypto.agg_trades`
   g. Update `lastID` to max `AggTradeID` from this batch
   h. If response returned exactly 1000 trades, immediately loop again (there may be more) without sleeping
3. On context cancellation (shutdown signal), flush remaining batch and exit

### 4.6 OrderBook Worker (`internal/worker/orderbook.go`)

Per-symbol goroutine logic:

1. Loop on ticker interval (default 5s):
   a. Call `limiter.WaitN(ctx, 5)` (depth weight = 5 at limit=100)
   b. `GET /api/v3/depth?symbol={symbol}&limit=100`
   c. Parse response into `DepthResponse`
   d. Assign `snapshot_time = time.Now().UTC()` (server-side timestamp not provided for REST depth)
   e. Convert bids/asks arrays into `[]OrderBookLevel` rows
   f. Batch insert snapshot header into `crypto.orderbook_snapshots` (returns `snapshot_id`)
   g. Batch insert levels into `crypto.orderbook_levels` with the `snapshot_id`
2. On context cancellation, exit cleanly

### 4.7 Ticker Worker (`internal/worker/ticker.go`)

Per-symbol goroutine logic:

1. Loop on ticker interval (default 30s):
   a. Call `limiter.WaitN(ctx, 2)` (ticker/24hr weight = 2 for single symbol)
   b. `GET /api/v3/ticker/24hr?symbol={symbol}`
   c. Parse response into `Ticker24hrResponse`
   d. Insert into `crypto.ticker_24hr`
2. On context cancellation, exit cleanly

### 4.8 Worker Manager (`internal/worker/manager.go`)

- Creates a shared `context.Context` with cancellation
- Starts all workers as goroutines
- Listens for OS signals (`SIGINT`, `SIGTERM`)
- On signal: cancels context, waits for all goroutines to finish (with a 10-second deadline)
- Reports worker errors via a shared error channel
- If any worker returns a fatal error (e.g., IP ban), cancels all workers

### 4.9 Data Parsing and Normalization

Binance returns prices and quantities as **strings** (to preserve decimal precision). The Go code must:

1. Parse price/quantity strings into `decimal.Decimal` using `shopspring/decimal` library (avoids float64 precision loss)
2. Store in PostgreSQL as `NUMERIC` type
3. Timestamps from Binance are Unix milliseconds -- convert to `time.Time` with `time.UnixMilli(ts).UTC()`
4. All timestamps stored in UTC
5. Boolean fields (`is_buyer_maker`) stored as PostgreSQL `BOOLEAN`

---

## 5. PostgreSQL Schema

All tables in schema `crypto` within the existing `pipeline` database.

### 5.1 Schema Creation

Add to `docker/postgres/init.sql`:

```sql
CREATE SCHEMA IF NOT EXISTS crypto;
```

### 5.2 Table: `crypto.symbols`

Metadata about trading pairs, populated from `/api/v3/exchangeInfo` at startup.

```sql
CREATE TABLE IF NOT EXISTS crypto.symbols (
    symbol              TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    base_asset          TEXT        NOT NULL,
    quote_asset         TEXT        NOT NULL,
    base_precision      INTEGER     NOT NULL,
    quote_precision     INTEGER     NOT NULL,
    filters_json        JSONB,
    fetched_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (symbol)
);
```

### 5.3 Table: `crypto.agg_trades`

Aggregate trade data from `/api/v3/aggTrades`.

```sql
CREATE TABLE IF NOT EXISTS crypto.agg_trades (
    symbol              TEXT        NOT NULL,
    agg_trade_id        BIGINT      NOT NULL,
    price               NUMERIC     NOT NULL,
    quantity            NUMERIC     NOT NULL,
    first_trade_id      BIGINT      NOT NULL,
    last_trade_id       BIGINT      NOT NULL,
    trade_time          TIMESTAMPTZ NOT NULL,
    is_buyer_maker      BOOLEAN     NOT NULL,
    is_best_match       BOOLEAN     NOT NULL,
    ingested_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (symbol, agg_trade_id)
);

CREATE INDEX IF NOT EXISTS idx_agg_trades_time
    ON crypto.agg_trades (symbol, trade_time);
```

**Partitioning consideration:** For production scale, this table should be partitioned by `trade_time` (monthly ranges). For the initial implementation with 3 symbols, a simple table with the composite primary key and time index is sufficient. Partitioning can be added later without schema changes to the Go code (transparent to the application).

**Deduplication:** The composite primary key `(symbol, agg_trade_id)` guarantees idempotency. On conflict, use `ON CONFLICT DO NOTHING` to silently skip duplicates.

### 5.4 Table: `crypto.orderbook_snapshots`

Snapshot metadata for order book captures.

```sql
CREATE TABLE IF NOT EXISTS crypto.orderbook_snapshots (
    snapshot_id         BIGSERIAL   NOT NULL,
    symbol              TEXT        NOT NULL,
    last_update_id      BIGINT      NOT NULL,
    snapshot_time       TIMESTAMPTZ NOT NULL,
    bid_count           INTEGER     NOT NULL,
    ask_count           INTEGER     NOT NULL,
    ingested_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (snapshot_id)
);

CREATE INDEX IF NOT EXISTS idx_ob_snapshots_symbol_time
    ON crypto.orderbook_snapshots (symbol, snapshot_time);
```

### 5.5 Table: `crypto.orderbook_levels`

Individual price levels within each snapshot.

```sql
CREATE TABLE IF NOT EXISTS crypto.orderbook_levels (
    snapshot_id         BIGINT      NOT NULL REFERENCES crypto.orderbook_snapshots(snapshot_id),
    side                TEXT        NOT NULL CHECK (side IN ('bid', 'ask')),
    level_index         SMALLINT    NOT NULL,
    price               NUMERIC     NOT NULL,
    quantity            NUMERIC     NOT NULL,
    PRIMARY KEY (snapshot_id, side, level_index)
);
```

**Note:** Storing every level of every snapshot generates significant data volume. At 100 levels * 2 sides * 3 symbols * 12 snapshots/min = ~43,200 rows/min = ~62M rows/day. For the initial implementation this is acceptable for a local dev environment. For production, consider:
- Reducing depth to 20 levels
- Reducing polling frequency to 15-30 seconds
- Adding a retention policy (delete snapshots older than N days)

### 5.6 Table: `crypto.ticker_24hr`

Rolling 24-hour ticker statistics.

```sql
CREATE TABLE IF NOT EXISTS crypto.ticker_24hr (
    id                      BIGSERIAL   NOT NULL,
    symbol                  TEXT        NOT NULL,
    price_change            NUMERIC     NOT NULL,
    price_change_percent    NUMERIC     NOT NULL,
    weighted_avg_price      NUMERIC     NOT NULL,
    last_price              NUMERIC     NOT NULL,
    last_qty                NUMERIC     NOT NULL,
    bid_price               NUMERIC     NOT NULL,
    bid_qty                 NUMERIC     NOT NULL,
    ask_price               NUMERIC     NOT NULL,
    ask_qty                 NUMERIC     NOT NULL,
    open_price              NUMERIC     NOT NULL,
    high_price              NUMERIC     NOT NULL,
    low_price               NUMERIC     NOT NULL,
    volume                  NUMERIC     NOT NULL,
    quote_volume            NUMERIC     NOT NULL,
    open_time               TIMESTAMPTZ NOT NULL,
    close_time              TIMESTAMPTZ NOT NULL,
    first_trade_id          BIGINT      NOT NULL,
    last_trade_id           BIGINT      NOT NULL,
    trade_count             BIGINT      NOT NULL,
    fetched_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_ticker_symbol_fetched
    ON crypto.ticker_24hr (symbol, fetched_at);
```

### 5.7 Table: `crypto.ingest_state`

Tracks high-water marks for each worker to enable resumable ingestion.

```sql
CREATE TABLE IF NOT EXISTS crypto.ingest_state (
    worker_type         TEXT        NOT NULL,
    symbol              TEXT        NOT NULL,
    last_id             BIGINT      NOT NULL DEFAULT 0,
    last_timestamp      TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (worker_type, symbol)
);
```

The trade worker updates `last_id` with the highest `agg_trade_id` after each successful batch. On startup, it reads this value to resume from where it left off.

---

## 6. Batching and Polling Strategy

### Approach: Polling with Adaptive Catch-Up

- **Primary strategy:** Periodic polling via `time.Ticker` per worker
- **Catch-up mode:** When a trade poll returns the maximum 1000 records, the worker immediately polls again (without sleeping) to catch up to the present. This handles both initial startup and periods where the worker fell behind.
- **No WebSocket for v1:** WebSocket would provide lower latency but adds connection management complexity. REST polling at 5-second intervals is sufficient for the stated requirements. WebSocket can be added as a future enhancement.

### Batch Inserts

- Use `pgx` COPY protocol (`pgx.CopyFrom`) for maximum insert throughput
- Buffer rows in memory up to `BATCH_SIZE` (default 500)
- Flush buffer when either:
  - Buffer reaches `BATCH_SIZE` rows, OR
  - `BATCH_FLUSH_INTERVAL` (default 2s) has elapsed since last flush
- For `agg_trades`: use `ON CONFLICT (symbol, agg_trade_id) DO NOTHING` -- this requires using regular batch INSERT instead of COPY. Use `pgx.Batch` for this.
- For `orderbook_levels` and `ticker_24hr`: COPY protocol is safe since snapshot_id is generated by the DB (no conflicts)

---

## 7. Error Handling and Retry Strategy

### HTTP Errors

| HTTP Status | Action |
|-------------|--------|
| 200         | Process response normally |
| 429         | Log warning, sleep for `Retry-After` seconds, retry once. If still 429, back off for 60 seconds. |
| 418         | IP banned. Log FATAL, cancel all workers, exit code 1. |
| 400         | Bad request (likely invalid symbol). Log error, skip this cycle, do NOT retry. |
| 5xx         | Exponential backoff: 1s, 2s, 4s. Max 3 retries per request. After 3 failures, log error and skip this cycle. |
| Network err | Same as 5xx -- exponential backoff with 3 retries. |

### PostgreSQL Errors

| Error Type | Action |
|------------|--------|
| Connection lost | Retry connection with exponential backoff (1s, 2s, 4s, 8s, 16s). `pgxpool` handles this automatically via its internal reconnection logic. |
| Unique violation | Expected for `agg_trades` dedup. Use `ON CONFLICT DO NOTHING`, no retry needed. |
| Disk full / other fatal | Log FATAL, exit code 1. |

### Graceful Shutdown

- On `SIGINT` / `SIGTERM`: cancel the root context
- Each worker's loop checks `ctx.Done()` at the top of each iteration
- Workers flush any buffered but unwritten rows before exiting
- Main goroutine waits up to 10 seconds for all workers to finish, then force-exits

---

## 8. Idempotency and Deduplication

### Trades (`crypto.agg_trades`)

- **Primary key:** `(symbol, agg_trade_id)` -- Binance aggregate trade IDs are globally unique per symbol and monotonically increasing
- **Insert strategy:** `INSERT INTO crypto.agg_trades (...) VALUES (...) ON CONFLICT (symbol, agg_trade_id) DO NOTHING`
- **High-water mark:** After each successful batch insert, update `crypto.ingest_state` with the max `agg_trade_id`. On restart, resume from `last_id + 1`.
- **Guarantee:** Even if the worker crashes mid-batch, restarting will re-fetch overlapping trades, and the `ON CONFLICT` clause silently drops duplicates. No data loss, no duplicates.

### Order Book Snapshots

- Snapshots are inherently non-idempotent (each poll creates a new snapshot). This is acceptable -- they are time-series point-in-time captures.
- `snapshot_id` is auto-generated by PostgreSQL `BIGSERIAL`.
- If the same `last_update_id` is captured twice (e.g., the order book didn't change between polls), both snapshots are stored. This is intentional for time-series completeness. A downstream consumer can deduplicate on `(symbol, last_update_id)` if needed.

### Ticker 24hr

- Similar to order book -- time-series captures. Each poll stores a new row.
- Downstream analytics can use `fetched_at` for deduplication or time-windowed aggregation.

---

## 9. Logging and Monitoring

### Logging

- Use `log/slog` (Go 1.21+ structured logging, part of standard library)
- JSON output format for machine parsing
- Log levels: DEBUG (HTTP request/response details), INFO (batch sizes, high-water marks), WARN (rate limits, retries), ERROR (failed requests, DB errors)
- Every log line includes: `timestamp`, `level`, `worker` (e.g., `trade/BTCUSDT`), `message`, and relevant structured fields

Key log events:
- Worker started/stopped
- Batch inserted (symbol, row_count, duration_ms)
- Rate limit approaching (current weight usage from `X-MBX-USED-WEIGHT-1m` header)
- Retry triggered (endpoint, attempt, delay)
- High-water mark updated (symbol, last_id)

### Monitoring (Recommendations for Future)

- Expose Prometheus metrics on `:9090/metrics` (use `prometheus/client_golang`)
- Key metrics:
  - `crypto_ingest_trades_total` (counter, by symbol)
  - `crypto_ingest_orderbook_snapshots_total` (counter, by symbol)
  - `crypto_ingest_http_requests_total` (counter, by endpoint, status_code)
  - `crypto_ingest_http_request_duration_seconds` (histogram, by endpoint)
  - `crypto_ingest_rate_limit_remaining` (gauge)
  - `crypto_ingest_batch_size` (histogram, by table)
  - `crypto_ingest_lag_seconds` (gauge, by symbol -- difference between now and latest trade_time)
- This is out of scope for v1 but the architecture supports it cleanly

---

## 10. Docker Integration

### New Files

| File | Purpose |
|------|---------|
| `crypto-ingest/Dockerfile` | Multi-stage Go build, final image based on `gcr.io/distroless/static-debian12:nonroot` |
| `crypto-ingest/cmd/ingest/main.go` | Entrypoint |
| `crypto-ingest/internal/...` | All Go packages as described above |
| `crypto-ingest/go.mod` | Go module definition |

### Docker Compose Addition

Add to the existing `docker-compose.yml`:

```yaml
  crypto-ingest:
    build:
      context: ./crypto-ingest
      dockerfile: Dockerfile
    image: crypto-ingest:0.1.0
    container_name: crypto-ingest
    environment:
      POSTGRES_DSN: "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable"
      BINANCE_SYMBOLS: "BTCUSDT,ETHUSDT,ETHBTC"
      TRADE_POLL_INTERVAL: "5s"
      ORDERBOOK_POLL_INTERVAL: "5s"
      ORDERBOOK_DEPTH: "100"
      TICKER_POLL_INTERVAL: "30s"
      LOG_LEVEL: "info"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: 256m
    networks:
      - data-pipeline
```

### Dockerfile (Multi-Stage Build)

```dockerfile
FROM golang:1.23.6-alpine3.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /crypto-ingest ./cmd/ingest

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /crypto-ingest /crypto-ingest
ENTRYPOINT ["/crypto-ingest"]
```

### Init SQL Update

Modify `docker/postgres/init.sql` to add the `crypto` schema:

```sql
CREATE SCHEMA IF NOT EXISTS app;
CREATE SCHEMA IF NOT EXISTS iceberg_catalog;
CREATE SCHEMA IF NOT EXISTS crypto;
```

The Go application will execute the table DDL on startup (via `store/migrations.go`), so tables do not need to be in `init.sql`. This keeps the DDL co-located with the Go code that uses it.

---

## 11. Scaling Recommendations (Future)

### Adding More Trading Pairs

- Add symbol names to `BINANCE_SYMBOLS` environment variable
- The worker manager dynamically creates worker goroutines per symbol -- no code changes needed
- Rate limit budget scales linearly: each additional symbol adds ~23 weight/min (at current intervals)
- With 3 symbols using 276 weight/min, you can support ~65 symbols before hitting the 5000 weight/min budget

### Adding More Exchanges

- Create a new package per exchange under `internal/client/` (e.g., `internal/client/kraken/`)
- Define a common `ExchangeClient` interface:
  ```go
  type ExchangeClient interface {
      FetchAggTrades(ctx context.Context, symbol string, fromID int64) ([]Trade, error)
      FetchOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)
      FetchTicker24hr(ctx context.Context, symbol string) (*Ticker, error)
      FetchExchangeInfo(ctx context.Context) ([]SymbolInfo, error)
  }
  ```
- Workers are parameterized by the interface, not by Binance-specific types
- Add an `exchange` column to PostgreSQL tables to distinguish data sources
- Run separate rate limiters per exchange

### Vertical Scaling

- Increase `BATCH_SIZE` for higher throughput
- Reduce poll intervals for higher data freshness
- The Go runtime efficiently handles hundreds of goroutines -- no need for external orchestration

### Horizontal Scaling (if needed much later)

- Assign different symbols to different instances
- Use the `crypto.ingest_state` table for distributed coordination (optimistic locking)
- Consider moving to Kafka/Redpanda (already in the stack) as an intermediate buffer between API polling and PostgreSQL writing

---

## Scope

### Files to CREATE

| File | Purpose |
|------|---------|
| `crypto-ingest/cmd/ingest/main.go` | Application entrypoint |
| `crypto-ingest/internal/config/config.go` | Environment-based configuration |
| `crypto-ingest/internal/client/binance.go` | Binance REST API client |
| `crypto-ingest/internal/client/ratelimit.go` | Token bucket rate limiter wrapper |
| `crypto-ingest/internal/model/trade.go` | Aggregate trade model |
| `crypto-ingest/internal/model/orderbook.go` | Order book model |
| `crypto-ingest/internal/model/ticker.go` | 24hr ticker model |
| `crypto-ingest/internal/model/symbol.go` | Symbol metadata model |
| `crypto-ingest/internal/worker/trade.go` | Trade polling worker |
| `crypto-ingest/internal/worker/orderbook.go` | Order book polling worker |
| `crypto-ingest/internal/worker/ticker.go` | Ticker polling worker |
| `crypto-ingest/internal/worker/manager.go` | Worker lifecycle manager |
| `crypto-ingest/internal/store/postgres.go` | PostgreSQL connection and batch operations |
| `crypto-ingest/internal/store/migrations.go` | DDL execution on startup |
| `crypto-ingest/go.mod` | Go module definition |
| `crypto-ingest/Dockerfile` | Multi-stage Docker build |

### Files to MODIFY

| File | Change |
|------|--------|
| `docker-compose.yml` | Add `crypto-ingest` service block |
| `docker/postgres/init.sql` | Add `CREATE SCHEMA IF NOT EXISTS crypto;` |
| `.env` | No change needed -- `POSTGRES_*` vars already exist and are reused |

### Files NOT modified

| File | Reason |
|------|--------|
| `CLAUDE.md` | Not requested |
| `clickhouse/*` | Separate subsystem, not affected |
| `flink-jobs/*` | Separate subsystem, not affected |
| `notebooks/*` | Separate subsystem, not affected |

---

## Execution Order

1. **Modify `docker/postgres/init.sql`** -- add `crypto` schema. This must happen first because the Go application depends on the schema existing. If the stack is already running, this requires running the SQL manually against the running Postgres container.

2. **Create `crypto-ingest/go.mod`** -- initialize the Go module. Required before any Go code can be written.

3. **Create model structs** (`internal/model/*.go`) -- pure data types with no dependencies. These are used by all other packages.

4. **Create config** (`internal/config/config.go`) -- needed by client, store, and worker packages.

5. **Create rate limiter** (`internal/client/ratelimit.go`) -- needed by the Binance client.

6. **Create Binance client** (`internal/client/binance.go`) -- depends on models and rate limiter.

7. **Create PostgreSQL store** (`internal/store/postgres.go` and `migrations.go`) -- depends on models. Contains all DDL and insert logic.

8. **Create workers** (`internal/worker/*.go`) -- depends on client and store.

9. **Create entrypoint** (`cmd/ingest/main.go`) -- wires everything together.

10. **Create Dockerfile** -- depends on the Go code being compilable.

11. **Modify `docker-compose.yml`** -- add the `crypto-ingest` service.

12. **End-to-end test** -- `docker compose up -d`, verify the crypto-ingest container starts, connects to Postgres, creates tables, and begins ingesting data. Check `crypto.agg_trades`, `crypto.orderbook_snapshots`, `crypto.orderbook_levels`, `crypto.ticker_24hr` for populated rows.

---

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Binance API changes or deprecates v3 endpoints | Workers fail to parse responses or get 404s | Pin to `/api/v3/` endpoints. Monitor Binance API changelog. The response parsing is tolerant of additional fields (Go's `json.Unmarshal` ignores unknown fields by default). |
| Binance blocks IP due to accidental rate limit violation | All data ingestion stops | Token bucket limiter set at 5000/min (83% of actual 6000 limit). Read `X-MBX-USED-WEIGHT-1m` header to detect drift. On HTTP 429, back off immediately. |
| Binance API is unreachable from the local network (firewall, proxy) | No data ingested | Log clear error on startup with the URL being accessed. Make `BINANCE_BASE_URL` configurable for testing with a mock server. |
| Float precision loss when parsing price/quantity strings | Incorrect prices stored in PostgreSQL | Use `shopspring/decimal` for parsing, store as PostgreSQL `NUMERIC`. Never convert to float64. |
| Order book data volume overwhelms PostgreSQL storage | Disk fills up | Default 100 levels at 5s interval = ~62M rows/day for order book levels. For local dev this is fine for days of running. Add a configurable retention policy (future enhancement). Log table sizes periodically. |
| `pgx` COPY protocol does not support `ON CONFLICT` | Cannot use COPY for dedup on agg_trades | Use `pgx.Batch` with INSERT ... ON CONFLICT DO NOTHING for agg_trades. Use COPY for orderbook_levels and ticker_24hr (no conflicts). |
| High-water mark in `ingest_state` not updated atomically with trade inserts | On crash, some trades may be re-inserted on restart | `ON CONFLICT DO NOTHING` makes this safe. Duplicates are silently dropped. The only cost is re-fetching some trades from Binance (negligible). |
| Go 1.23 not available in builder image | Build fails | Pin `golang:1.23.6-alpine3.21` (specific patch version). Alternatively use `golang:1.22.x` -- the code uses no 1.23-specific features. |
| `distroless` base image lacks shell for debugging | Cannot exec into container for troubleshooting | Use `gcr.io/distroless/static-debian12:debug` tag during development (includes busybox shell). Switch to `nonroot` for production. |
| PostgreSQL connection pool exhaustion under high goroutine count | Workers block waiting for connections | Configure `pgxpool.Config.MaxConns` to at least `num_symbols * 3 + 2` (workers + metadata loader + migrations). Default 10 is sufficient for 3 symbols. |
| `init.sql` schema change not applied to already-initialized Postgres volume | `crypto` schema does not exist | The Go application's `migrations.go` executes `CREATE SCHEMA IF NOT EXISTS crypto` on startup, independent of `init.sql`. Both paths are covered. |

---

## Go Dependencies

```
module crypto-ingest

go 1.23

require (
    github.com/jackc/pgx/v5         v5.7.2
    github.com/shopspring/decimal    v1.4.0
    golang.org/x/time               v0.9.0
)
```

- `pgx/v5` -- PostgreSQL driver with native COPY support, connection pooling, and batch operations
- `shopspring/decimal` -- arbitrary precision decimal for price/quantity parsing
- `golang.org/x/time` -- token bucket rate limiter (`rate.Limiter`)

No web framework, no ORM, no external JSON library, no external logging library. Standard library covers HTTP client, JSON parsing, and structured logging (`slog`).

---

## Definition of Done

### Infrastructure
- [ ] `docker/postgres/init.sql` contains `CREATE SCHEMA IF NOT EXISTS crypto;`
- [ ] `docker-compose.yml` contains a `crypto-ingest` service that builds from `crypto-ingest/Dockerfile`
- [ ] `crypto-ingest/Dockerfile` exists and produces a working container image using multi-stage build with pinned base images (no `latest` tag)
- [ ] The `crypto-ingest` container starts successfully with `docker compose up -d` and appears healthy in `docker compose ps`

### Go Application
- [ ] `crypto-ingest/go.mod` defines the module with pinned dependency versions
- [ ] `crypto-ingest/cmd/ingest/main.go` is the application entrypoint
- [ ] All configuration is read from environment variables (no hardcoded credentials, no config files)
- [ ] Application creates the `crypto` schema and all tables on startup if they do not exist (`CREATE TABLE IF NOT EXISTS`)
- [ ] Application connects to PostgreSQL using the `POSTGRES_DSN` environment variable

### Data Ingestion
- [ ] Trade worker polls Binance `/api/v3/aggTrades` for each configured symbol at the configured interval
- [ ] Order book worker polls Binance `/api/v3/depth` for each configured symbol at the configured interval
- [ ] Ticker worker polls Binance `/api/v3/ticker/24hr` for each configured symbol at the configured interval
- [ ] Exchange metadata is fetched from `/api/v3/exchangeInfo` and stored in `crypto.symbols` on startup
- [ ] All prices and quantities are stored as `NUMERIC` (not float) in PostgreSQL
- [ ] All timestamps are stored as `TIMESTAMPTZ` in UTC

### PostgreSQL Schema
- [ ] Table `crypto.symbols` exists with correct columns and primary key
- [ ] Table `crypto.agg_trades` exists with composite primary key `(symbol, agg_trade_id)` and time index
- [ ] Table `crypto.orderbook_snapshots` exists with primary key and symbol+time index
- [ ] Table `crypto.orderbook_levels` exists with foreign key to snapshots and composite primary key
- [ ] Table `crypto.ticker_24hr` exists with primary key and symbol+fetched_at index
- [ ] Table `crypto.ingest_state` exists with primary key `(worker_type, symbol)`

### Rate Limiting and Error Handling
- [ ] A shared token bucket rate limiter constrains all API calls to stay within Binance's 6000 weight/min limit
- [ ] HTTP 429 responses trigger a retry after the `Retry-After` delay
- [ ] HTTP 418 responses (IP ban) cause immediate shutdown with a clear log message
- [ ] HTTP 5xx responses trigger exponential backoff (max 3 retries)
- [ ] PostgreSQL connection loss is handled by `pgxpool` auto-reconnection

### Idempotency
- [ ] Trade inserts use `ON CONFLICT (symbol, agg_trade_id) DO NOTHING` to prevent duplicates
- [ ] Trade worker resumes from the last known `agg_trade_id` stored in `crypto.ingest_state` after restart
- [ ] Restarting the `crypto-ingest` container does not produce duplicate trade rows

### Logging
- [ ] Structured JSON logging via `log/slog`
- [ ] Log level is configurable via `LOG_LEVEL` environment variable
- [ ] Worker start/stop events are logged
- [ ] Batch insert events include row count and duration
- [ ] Rate limit warnings include current weight usage

### Graceful Shutdown
- [ ] `SIGINT` and `SIGTERM` trigger graceful shutdown
- [ ] All workers flush pending batches before exiting
- [ ] Shutdown completes within 10 seconds

### End-to-End Verification
- [ ] After running for 60 seconds, `crypto.agg_trades` contains rows for all 3 symbols (BTCUSDT, ETHUSDT, ETHBTC)
- [ ] After running for 60 seconds, `crypto.orderbook_snapshots` contains at least 10 snapshots per symbol
- [ ] After running for 60 seconds, `crypto.orderbook_levels` contains bid and ask rows linked to valid snapshots
- [ ] After running for 60 seconds, `crypto.ticker_24hr` contains at least 1 row per symbol
- [ ] After running for 60 seconds, `crypto.symbols` contains metadata for all 3 symbols
- [ ] Stopping and restarting the container does not produce duplicate trades in `crypto.agg_trades`
- [ ] No Go source files contain comments (per project rules)
- [ ] No credentials are hardcoded in any file (all from environment variables / `.env`)
