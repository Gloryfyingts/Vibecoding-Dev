# INFRA.md — Docker Compose Stack Setup Report

## Date

2026-03-22 (ClickHouse stand added)

## Tasks Completed

- 2026-03-21: Full local development environment (main stack) — Postgres, MinIO, Redpanda, Flink, Spark, Jupyter
- 2026-03-22: ClickHouse local stand — 2-shard cluster with Keeper, analytics and inventory databases, distributed tables, data skipping indexes, factorial UDF

---

## ClickHouse Stand

Standalone 3-container cluster defined in `clickhouse/docker-compose.yml`. Fully isolated from the main stack.

### Services

| Service | Container Name | Host Port(s) | Image | Purpose |
|---|---|---|---|---|
| ClickHouse Keeper | `clickhouse-keeper` | 19181 (Keeper client) | `clickhouse/clickhouse-keeper:24.8.7.41` | ZooKeeper-compatible coordination, distributed DDL task queue |
| ClickHouse Shard 1 | `clickhouse-shard1` | 18123 (HTTP), 19000 (TCP) | `clickhouse/clickhouse-server:24.8.7.41` | Shard 1, runs init.sql seed via init container |
| ClickHouse Shard 2 | `clickhouse-shard2` | 18124 (HTTP), 19001 (TCP) | `clickhouse/clickhouse-server:24.8.7.41` | Shard 2, receives DDL via ON CLUSTER propagation |
| ClickHouse Init | `clickhouse-init` | — | `clickhouse/clickhouse-server:24.8.7.41` | One-shot init container, runs init.sql after shards are healthy |

### Cluster Name

`cluster_2s1r` — 2 shards, 1 replica per shard.

### Databases and Tables

- `analytics`: `user_events_local`, `daily_aggregates_local`, `session_facts_local` (ReplicatedMergeTree) + `user_events`, `daily_aggregates`, `session_facts` (Distributed)
- `inventory`: `products_local`, `stock_movements_local`, `warehouse_snapshots_local` (ReplicatedMergeTree) + `products`, `stock_movements`, `warehouse_snapshots` (Distributed)

### User-Defined Function

`factorial(n)` — available on both shards via user-defined function (UDF) defined in `clickhouse/init/init.sql`.

### How to Start

```bash
cd clickhouse
cp .env.example .env
docker compose up -d
```

### How to Stop

```bash
cd clickhouse
docker compose down
```

### Named Volumes

| Volume | Service |
|---|---|
| `clickhouse-keeper-data` | clickhouse-keeper |
| `clickhouse-shard1-data` | clickhouse-shard1 |
| `clickhouse-shard2-data` | clickhouse-shard2 |

### Files Created / Modified

| File | Status | Notes |
|---|---|---|
| `clickhouse/docker-compose.yml` | Created | 4 services (keeper, shard1, shard2, init), healthchecks, resource limits, named volumes |
| `clickhouse/.env.example` | Created | CLICKHOUSE_USER, CLICKHOUSE_PASSWORD, CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT |
| `clickhouse/config/clickhouse-keeper.xml` | Created | Single-node Keeper, listens on 0.0.0.0:9181, raft on 9234 |
| `clickhouse/config/clickhouse-common.xml` | Created | cluster_2s1r topology, Keeper connection, inter-shard credentials from env vars |
| `clickhouse/config/macros-shard1.xml` | Created | cluster=cluster_2s1r, shard=1, replica=shard1 |
| `clickhouse/config/macros-shard2.xml` | Created | cluster=cluster_2s1r, shard=2, replica=shard2 |
| `clickhouse/config/users.xml` | Created | Network access (:::/0), default profile settings |
| `clickhouse/init/init.sql` | Created | 2 databases, 6 ReplicatedMergeTree tables, 6 Distributed tables, 8 data skipping indexes, factorial UDF |
| `clickhouse/init/init.sh` | Created | Waits for Keeper readiness before running init.sql, handles idempotent re-runs |
| `.gitignore` | Modified | Added clickhouse/.env |
| `.claude/docs/clickhouse-setup.md` | Created | Full architecture documentation |

---

---

## crypto-ingest Service (added 2026-04-10)

New service added to the main `docker-compose.yml` stack for cryptocurrency market data ingestion from Binance REST API.

### Service Details

| Service | Container Name | Host Port(s) | Image | Purpose |
|---|---|---|---|---|
| crypto-ingest | `crypto-ingest` | 8085 (health, internal only) | built from `./crypto-ingest` | Polls Binance API, writes trades/orderbook/ticker to PostgreSQL `crypto` schema |

### Files Modified / Created

| File | Change |
|---|---|
| `docker/postgres/init.sql` | Added `CREATE SCHEMA IF NOT EXISTS crypto;` |
| `docker-compose.yml` | Added `crypto-ingest` service with healthcheck, `depends_on`, `mem_limit: 256m`, `stop_grace_period: 15s`, `restart: unless-stopped` |
| `.env.example` | Added 12 crypto-ingest environment variables with defaults |
| `crypto-ingest/Dockerfile` | Created multi-stage build: `golang:1.22.12-alpine3.21` -> `gcr.io/distroless/static-debian12:nonroot` |

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `BINANCE_BASE_URL` | `https://api.binance.com` | Binance REST API base URL (change for geo-blocking mitigation) |
| `SYMBOLS` | `BTCUSDT,ETHUSDT,ETHBTC` | Comma-separated trading pairs |
| `TRADE_POLL_INTERVAL` | `5s` | Trade worker polling interval |
| `ORDERBOOK_POLL_INTERVAL` | `5s` | Order book worker polling interval |
| `ORDERBOOK_DEPTH` | `20` | Order book levels per side |
| `TICKER_POLL_INTERVAL` | `30s` | Ticker worker polling interval |
| `HEALTH_PORT` | `8085` | Health endpoint listen port |
| `LOG_LEVEL` | `info` | Structured log level |
| `MAX_RETRIES` | `3` | Max retries per failed API call |
| `RETRY_BASE_DELAY` | `1s` | Base delay for exponential backoff |
| `PG_MAX_CONNS` | `12` | pgx pool maximum connections |
| `PG_ACQUIRE_TIMEOUT` | `5s` | pgx pool connection acquisition timeout |

Set `HTTP_PROXY`/`HTTPS_PROXY` in `.env` if Binance is geo-blocked on the deployment network.

### Notes

- `DATABASE_URL` is constructed dynamically in docker-compose from `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` — not a separate `.env` variable.
- Healthcheck uses binary self-probe `["/crypto-ingest", "--healthcheck"]` — required because distroless image has no shell, curl, or wget.
- `crypto` schema is also created by the Go app on startup (`CREATE SCHEMA IF NOT EXISTS`) so it works on existing Postgres volumes where `init.sql` has already run.
- Do NOT run `docker compose up` until the de-coder agent has completed the Go source files.

---

## Services

| Service | Container Name | Host Port(s) | Image | Purpose |
|---|---|---|---|---|
| Postgres | `postgres` | 5432 | `postgres:16.6` | App database + Iceberg JDBC catalog |
| MinIO | `minio` | 9000 (API), 9001 (Console) | `minio/minio:RELEASE.2024-11-07T00-52-20Z` | S3-compatible object storage |
| MinIO Init | `minio-init` | — | `minio/mc:RELEASE.2024-11-17T19-35-25Z` | One-shot bucket creation (exits after run) |
| Redpanda | `redpanda` | 19092 (Kafka), 18082 (HTTP Proxy), 18081 (Schema Registry), 19644 (Admin) | `redpandadata/redpanda:v24.2.18` | Kafka-compatible message broker |
| Redpanda Console | `redpanda-console` | 8888 | `redpandadata/console:v2.7.2` | Redpanda Web UI |
| Flink JobManager | `flink-jobmanager` | 8081 | `flink-custom:1.20.1` (built) | Flink session cluster coordinator |
| Flink TaskManager | `flink-taskmanager` | — | `flink-custom:1.20.1` (built) | Flink session cluster worker |
| Spark Master | `spark-master` | 8080 (UI), 7077 (RPC) | `bitnami/spark:3.5.4` | Spark standalone cluster master |
| Spark Worker | `spark-worker` | 8082 | `bitnami/spark:3.5.4` | Spark standalone cluster worker |
| Jupyter | `jupyter` | 8890 | `jupyter-custom:spark-3.5.4` (built) | Notebook environment (PySpark + Python) |

---

## Web UIs

| Service | URL |
|---|---|
| Flink Dashboard | http://localhost:8081 |
| Spark Master UI | http://localhost:8080 |
| Spark Worker UI | http://localhost:8082 |
| MinIO Console | http://localhost:9001 |
| Redpanda Console | http://localhost:8888 |
| Jupyter Notebook | http://localhost:8890 |

---

## Named Volumes

| Volume | Mounted At | Service |
|---|---|---|
| `postgres-data` | `/var/lib/postgresql/data` | postgres |
| `minio-data` | `/data` | minio |

---

## Bind Mounts

| Host Path | Container Path | Service(s) |
|---|---|---|
| `./docker/postgres/init.sql` | `/docker-entrypoint-initdb.d/init.sql:ro` | postgres |
| `./flink-jobs` | `/opt/flink/jobs` | flink-jobmanager, flink-taskmanager |
| `./notebooks` | `/home/jovyan/work` | jupyter |

---

## Network

Single bridge network: `data-pipeline`. All services are attached to it.

---

## Files Created / Modified

| File | Status | Notes |
|---|---|---|
| `docker-compose.yml` | Created | All 10 services, healthchecks, memory limits, named volumes |
| `.env.example` | Created | Placeholder credentials template |
| `.env` | Created | Working local dev credentials (not committed) |
| `.gitignore` | Created | Includes `.env` |
| `docker/postgres/init.sql` | Created | Creates schemas `app` and `iceberg_catalog` |
| `docker/flink/Dockerfile` | Created | `flink:1.20.1-scala_2.12-java11`, S3 plugin enabled, Kafka connector JAR downloaded |
| `docker/jupyter/Dockerfile` | Created | `quay.io/jupyter/pyspark-notebook:spark-3.5.4`, psycopg2-binary + kafka-python-ng |
| `flink-jobs/sql/e2e_redpanda_to_s3.sql` | Created | Flink SQL: Redpanda source -> MinIO filesystem sink |
| `flink-jobs/python/e2e_redpanda_to_s3.py` | Created | PyFlink Table API equivalent |
| `notebooks/test_e2e.ipynb` | Created | 4-cell E2E test: Postgres insert -> Redpanda produce -> Flink -> Iceberg |

---

## How to Start

```bash
docker compose up -d
```

First run downloads images (~5-8 GB) and builds custom Dockerfiles for Flink and Jupyter. Subsequent starts are fast.

## How to Stop

```bash
docker compose down
```

Preserves named data volumes (`postgres-data`, `minio-data`).

## How to Reset (destroys all data — requires explicit approval)

```bash
docker compose down -v
docker compose up -d
```

---

## Credentials

All credentials are stored in `.env`. See `.env.example` for the variable names:

| Variable | Description |
|---|---|
| `POSTGRES_USER` | Postgres superuser name |
| `POSTGRES_PASSWORD` | Postgres superuser password |
| `POSTGRES_DB` | Default Postgres database name |
| `MINIO_ROOT_USER` | MinIO root access key (also AWS_ACCESS_KEY_ID in Jupyter and Flink) |
| `MINIO_ROOT_PASSWORD` | MinIO root secret key (also AWS_SECRET_ACCESS_KEY in Jupyter and Flink) |
| `JUPYTER_TOKEN` | Token required to access Jupyter UI; leave empty to disable auth |

---

## Port Conflict Notes

Redpanda uses dual listeners to avoid conflicts with Flink (8081) and Spark Worker (8082):
- Internal ports 9092, 8081, 8082 are accessible only within the `data-pipeline` Docker network.
- External ports 19092, 18081, 18082 are mapped to the host.
- Flink Dashboard keeps host port 8081. Spark Worker UI keeps host port 8082.

---

## E2E Test Pipeline

The test pipeline flows: Postgres -> Redpanda -> Flink -> MinIO (S3) -> Spark/Iceberg.

Test files:
- `notebooks/test_e2e.ipynb` — run Cells 1-2 in Jupyter (http://localhost:8890) to insert data and produce to Redpanda
- `flink-jobs/sql/e2e_redpanda_to_s3.sql` — Flink SQL job (paste into `docker exec -it flink-jobmanager ./bin/sql-client.sh`)
- `flink-jobs/python/e2e_redpanda_to_s3.py` — PyFlink alternative (`docker exec -it flink-jobmanager flink run --python /opt/flink/jobs/python/e2e_redpanda_to_s3.py`)
- Cell 4 in the notebook — PySpark local mode reads JSON from `s3a://raw/events/`, writes Iceberg table `iceberg.db.events`
