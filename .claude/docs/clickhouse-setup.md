# ClickHouse Local Stand — Setup Documentation

## Architecture Overview

The ClickHouse local stand is a self-contained cluster defined in `clickhouse/docker-compose.yml`, completely isolated from the main `docker-compose.yml` stack. It consists of three containers:

- **clickhouse-keeper** — a dedicated ClickHouse Keeper node (ZooKeeper-compatible coordination service). Handles distributed DDL task queues and ReplicatedMergeTree metadata. Single-node Keeper is sufficient for a development cluster.
- **clickhouse-shard1** — ClickHouse server node, shard 1.
- **clickhouse-shard2** — ClickHouse server node, shard 2. Receives table definitions via `ON CLUSTER` DDL propagated through Keeper.
- **clickhouse-init** — one-shot container that waits for both shards and Keeper to be healthy, then runs `init.sql` via `clickhouse-client --multiquery`. Same pattern as `minio-init` in the main stack.

This is a 2-shard, 1-replica-per-shard configuration. There is no replication between shards — each shard holds a disjoint partition of distributed data. ReplicatedMergeTree is used (rather than plain MergeTree) because it registers table metadata with Keeper, which is required for `ON CLUSTER` DDL to work correctly.

```
                  ┌─────────────────────┐
                  │  clickhouse-keeper  │
                  │  port 9181 (client) │
                  │  port 9234 (raft)   │
                  └─────────┬───────────┘
                            │ ZK protocol
              ┌─────────────┴─────────────┐
              │                           │
   ┌──────────▼──────────┐   ┌────────────▼────────────┐
   │  clickhouse-shard1  │   │   clickhouse-shard2     │
   │  HTTP  18123:8123   │   │   HTTP  18124:8123      │
   │  TCP   19000:9000   │   │   TCP   19001:9000      │
   └─────────────────────┘   └─────────────────────────┘
```

## How to Start the Stack

```bash
cd clickhouse
cp .env.example .env
# Edit .env and set a real password for CLICKHOUSE_PASSWORD
docker compose up -d
```

First run pulls images (~1.5 GB total). Startup sequence: Keeper starts first, shard nodes wait for Keeper to pass its healthcheck, then the `clickhouse-init` one-shot container waits for both shards to be healthy and Keeper coordination to be available, then runs `init.sql` via `clickhouse-client --multiquery`.

## How to Stop the Stack

```bash
cd clickhouse
docker compose down
```

Named volumes are preserved. Data survives restarts.

## How to Reset (destroys all data — requires explicit user approval)

```bash
cd clickhouse
docker compose down -v
docker compose up -d
```

## Port Mapping

| Service | Container Port | Host Port | Protocol |
|---|---|---|---|
| clickhouse-shard1 | 8123 | 18123 | HTTP |
| clickhouse-shard2 | 8123 | 18124 | HTTP |
| clickhouse-shard1 | 9000 | 19000 | Native TCP |
| clickhouse-shard2 | 9000 | 19001 | Native TCP |
| clickhouse-keeper | 9181 | 19181 | Keeper client |

Internal ports not exposed to host: 9234 (Keeper raft), 9009 (ClickHouse inter-server replication).

All host ports are chosen to avoid conflicts with the main stack (5432, 7077, 8080–8082, 8888, 8890, 9000–9001, 18081–18082, 19092, 19644).

## How to Connect

Using `clickhouse-client` from the host (requires clickhouse-client installed locally):

```bash
clickhouse-client --host localhost --port 19000 --user ch_admin --password <your_password>
```

Using HTTP from the host:

```bash
curl "http://localhost:18123/?query=SELECT+1&user=ch_admin&password=<your_password>"
```

Using `docker exec` (no local client required):

```bash
docker exec -it clickhouse-shard1 clickhouse-client --user ch_admin --password <your_password>
```

## How init.sql Propagation Works

A dedicated `clickhouse-init` one-shot container handles initialization. It depends on both shard services being healthy, then runs `init.sh` which polls `system.zookeeper` until Keeper coordination is fully available, and finally executes `init.sql` via `clickhouse-client --multiquery` against shard1.

All DDL statements in `init.sql` use `ON CLUSTER cluster_2s1r`. When shard1 executes a `CREATE TABLE ON CLUSTER` statement, ClickHouse writes the task to the Keeper task queue at `/clickhouse/task_queue/ddl`. Shard2 polls this queue and executes the same DDL locally. This is why shard2 does not need its own copy of `init.sql`.

## Databases and Tables

### analytics database

| Table | Type | Engine |
|---|---|---|
| `analytics.user_events_local` | Local | ReplicatedMergeTree |
| `analytics.daily_aggregates_local` | Local | ReplicatedMergeTree |
| `analytics.session_facts_local` | Local | ReplicatedMergeTree |
| `analytics.user_events` | Distributed | Distributed(cluster_2s1r, analytics, user_events_local, rand()) |
| `analytics.daily_aggregates` | Distributed | Distributed(cluster_2s1r, analytics, daily_aggregates_local, rand()) |
| `analytics.session_facts` | Distributed | Distributed(cluster_2s1r, analytics, session_facts_local, rand()) |

### inventory database

| Table | Type | Engine |
|---|---|---|
| `inventory.products_local` | Local | ReplicatedMergeTree |
| `inventory.stock_movements_local` | Local | ReplicatedMergeTree |
| `inventory.warehouse_snapshots_local` | Local | ReplicatedMergeTree |
| `inventory.products` | Distributed | Distributed(cluster_2s1r, inventory, products_local, rand()) |
| `inventory.stock_movements` | Distributed | Distributed(cluster_2s1r, inventory, stock_movements_local, rand()) |
| `inventory.warehouse_snapshots` | Distributed | Distributed(cluster_2s1r, inventory, warehouse_snapshots_local, rand()) |

## Querying Distributed vs Local Tables

Insert data through the Distributed table — it fans out to shards using `rand()` sharding:

```sql
INSERT INTO analytics.user_events (event_id, user_id, event_type, event_timestamp, page_url, session_id, duration_seconds, is_mobile, country_code, payload)
VALUES (1, 42, 'click', now(), 'https://example.com', generateUUIDv4(), 3.5, 1, 'US', '{}');
```

Query through the Distributed table to aggregate across both shards:

```sql
SELECT event_type, count() AS cnt
FROM analytics.user_events
GROUP BY event_type
ORDER BY cnt DESC;
```

Query a local table to inspect what lives on a specific shard:

```sql
SELECT count() FROM analytics.user_events_local;
```

## How Macros and ReplicatedMergeTree Paths Work

Each shard node loads a different macros XML file:

- `clickhouse-shard1` loads `macros-shard1.xml`: `{cluster}=cluster_2s1r`, `{shard}=1`, `{replica}=shard1`
- `clickhouse-shard2` loads `macros-shard2.xml`: `{cluster}=cluster_2s1r`, `{shard}=2`, `{replica}=shard2`

When ClickHouse evaluates a `ReplicatedMergeTree` engine path like `/clickhouse/tables/{shard}/{database}/{table}`, it substitutes the local macro values. On shard1, the path becomes `/clickhouse/tables/1/analytics/user_events_local`. On shard2, it becomes `/clickhouse/tables/2/analytics/user_events_local`. These paths are stored in Keeper and uniquely identify each replica.

## Data Skipping Indexes Used

| Index Type | Tables |
|---|---|
| `set(N)` | `analytics.user_events_local` (event_type), `inventory.stock_movements_local` (warehouse_id) |
| `bloom_filter(p)` | `analytics.user_events_local` (country_code), `analytics.daily_aggregates_local` (dimension_key), `inventory.products_local` (sku) |
| `minmax` | `analytics.session_facts_local` (total_duration_seconds), `inventory.products_local` (price), `inventory.warehouse_snapshots_local` (quantity_available) |

## User-Defined Function

`factorial(n)` is created `ON CLUSTER` so it exists on both shards:

```sql
SELECT factorial(5);
```

Returns `120`.

Edge case: `factorial(0)` uses `range(1, 1)` which produces an empty array. `arrayProduct` of an empty array returns `1` in ClickHouse 24.8 (multiplicative identity), giving the correct result `0! = 1`. Verify with `SELECT factorial(0)` — expected output is `1`.

If a future ClickHouse version changes this behavior, wrap with: `if(n <= 0, toUInt64(1), toUInt64(arrayProduct(arrayMap(x -> toUInt64(x), range(1, toUInt64(n) + 1)))))`.

## Verifying Cluster Health

```sql
SELECT cluster, shard_num, replica_num, host_name
FROM system.clusters
WHERE cluster = 'cluster_2s1r';
```

Expected output: 2 rows, one for `clickhouse-shard1` (shard_num=1) and one for `clickhouse-shard2` (shard_num=2).

```sql
SELECT name, value, czxid, mzxid FROM system.zookeeper WHERE path = '/clickhouse/task_queue/ddl';
```

Shows the distributed DDL task queue in Keeper.

## Memory Requirements

| Container | Memory Limit |
|---|---|
| clickhouse-keeper | 256 MB |
| clickhouse-shard1 | 1 GB |
| clickhouse-shard2 | 1 GB |
| Total | ~2.5 GB |

The main stack (`docker-compose.yml` at repo root) requires ~10 GB. Running both stacks simultaneously requires approximately 12.5 GB of available RAM. Stop the main stack before starting the ClickHouse stand if host memory is constrained:

```bash
cd ..
docker compose down
cd clickhouse
docker compose up -d
```

## File Structure

```
clickhouse/
  docker-compose.yml
  .env.example
  .env                  (not committed — see .gitignore)
  config/
    clickhouse-keeper.xml
    clickhouse-common.xml
    macros-shard1.xml
    macros-shard2.xml
    users.xml
  init/
    init.sql
    init.sh
```
