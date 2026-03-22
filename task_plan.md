# Task: Standalone ClickHouse Docker Compose Setup

## Summary

Create a self-contained ClickHouse cluster scenario in a `clickhouse/` directory at the repo root. The setup includes 2 shard nodes (no replicas), 1 ClickHouse Keeper node, proper XML cluster configs, a seed `init.sql` with 2 databases, 6 tables (local + distributed), data skipping indexes, and 1 user-defined function. Fully isolated from the main `docker-compose.yml` stack.

---

## Scope

### Files to CREATE

| File | Purpose |
|---|---|
| `clickhouse/docker-compose.yml` | Compose file for the 3-container ClickHouse cluster |
| `clickhouse/.env.example` | Template for credentials and settings |
| `clickhouse/config/clickhouse-keeper.xml` | ClickHouse Keeper configuration (raft settings, server ID, ports) |
| `clickhouse/config/clickhouse-common.xml` | Shared ClickHouse server config: Keeper connection, remote_servers cluster definition |
| `clickhouse/config/macros-shard1.xml` | Macros file for shard 1 (`<shard>1</shard>`, `<replica>shard1</replica>`) |
| `clickhouse/config/macros-shard2.xml` | Macros file for shard 2 (`<shard>2</shard>`, `<replica>shard2</replica>`) |
| `clickhouse/config/users.xml` | User configuration: network access, profiles |
| `clickhouse/init/init.sql` | Seed SQL: 2 databases, 6 local tables, 6 distributed tables, indexes, 1 UDF |
| `.claude/docs/clickhouse-setup.md` | Documentation for the ClickHouse stand (replaces in-code comments per project rules) |

### Files to MODIFY

| File | Change |
|---|---|
| `.gitignore` | Add `clickhouse/.env` line to prevent credential leakage |

### Files NOT modified

| File | Reason |
|---|---|
| `docker-compose.yml` (root) | The ClickHouse stack is completely separate |
| `CLAUDE.md` | Not requested; only updated if user asks after implementation |

---

## Port Allocation

Ports chosen to avoid all conflicts with the main stack (5432, 7077, 8080, 8081, 8082, 8888, 8890, 9000, 9001, 18081, 18082, 19092, 19644).

| Service | Container Port | Host Port | Protocol |
|---|---|---|---|
| clickhouse-shard1 | 8123 | 18123 | HTTP |
| clickhouse-shard1 | 9000 | 19000 | Native TCP |
| clickhouse-shard2 | 8123 | 18124 | HTTP |
| clickhouse-shard2 | 9000 | 19001 | Native TCP |
| clickhouse-keeper | 9181 | 19181 | Keeper client port |

Inter-node ports (Keeper raft 9234, inter-server 9009) remain internal to the Docker network and are not exposed to the host.

---

## Detailed File Specifications

### 1. `clickhouse/docker-compose.yml`

- **Shard image:** `clickhouse/clickhouse-server:24.8.7.41` (LTS release, pinned)
- **Keeper image:** `clickhouse/clickhouse-keeper:24.8.7.41` (same LTS release, pinned, dedicated keeper image)
- **Network:** `clickhouse-cluster` (bridge, isolated from main stack)
- **Named volumes:** `clickhouse-shard1-data`, `clickhouse-shard2-data`, `clickhouse-keeper-data`
- **Services:**
  - `clickhouse-keeper`: dedicated keeper container, mounts `clickhouse-keeper.xml` to `/etc/clickhouse-keeper/keeper_config.xml`, exposes 19181:9181
  - `clickhouse-shard1`: mounts `clickhouse-common.xml` + `macros-shard1.xml` + `users.xml` to `/etc/clickhouse-server/config.d/`, bind-mounts `init/` to `/docker-entrypoint-initdb.d/`, depends_on keeper healthy, exposes 18123:8123 and 19000:9000
  - `clickhouse-shard2`: mounts `clickhouse-common.xml` + `macros-shard2.xml` + `users.xml` to `/etc/clickhouse-server/config.d/`, depends_on keeper healthy, exposes 18124:8123 and 19001:9000
- **Healthchecks:**
  - Shard nodes: `wget --no-verbose --tries=1 --spider http://localhost:8123/ping` interval 5s, timeout 3s, retries 5, start_period 10s
  - Keeper: `echo ruok | nc localhost 9181 | grep -q imok` interval 5s, timeout 3s, retries 5, start_period 10s
- **Resource limits:** 1g per shard node, 256m for keeper
- **Environment:** `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT` from `.env` via `${VAR}` syntax
- **Init SQL:** Only shard1 runs init.sql via `/docker-entrypoint-initdb.d/`. All DDL uses `ON CLUSTER cluster_2s1r` so objects propagate to shard2 automatically.

### 2. `clickhouse/.env.example`

```
CLICKHOUSE_USER=ch_admin
CLICKHOUSE_PASSWORD=CHANGE_ME
CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1
```

### 3. `clickhouse/config/clickhouse-keeper.xml`

ClickHouse Keeper configuration:
- `<keeper_server>` block: TCP port 9181, server_id 1, log storage path `/var/lib/clickhouse-keeper/coordination/log`, snapshot storage path `/var/lib/clickhouse-keeper/coordination/snapshots`
- `<raft_configuration>`: single server entry (id=1, hostname=clickhouse-keeper, port=9234)
- `<coordination_settings>`: operation_timeout_ms 10000, session_timeout_ms 30000

### 4. `clickhouse/config/clickhouse-common.xml`

Shared server configuration mounted into both shard nodes:
- `<remote_servers>`: cluster named `cluster_2s1r` with 2 `<shard>` blocks, each containing 1 `<replica>` pointing to `clickhouse-shard1:9000` and `clickhouse-shard2:9000` respectively
- `<zookeeper>`: single `<node>` pointing to `clickhouse-keeper:9181` (ClickHouse uses the `<zookeeper>` tag for Keeper connectivity)
- `<distributed_ddl>`: path `/clickhouse/task_queue/ddl`

### 5. `clickhouse/config/macros-shard1.xml` and `macros-shard2.xml`

Each file provides per-node macros used by ReplicatedMergeTree paths:

**macros-shard1.xml:**
```xml
<clickhouse>
    <macros>
        <cluster>cluster_2s1r</cluster>
        <shard>1</shard>
        <replica>shard1</replica>
    </macros>
</clickhouse>
```

**macros-shard2.xml:**
```xml
<clickhouse>
    <macros>
        <cluster>cluster_2s1r</cluster>
        <shard>2</shard>
        <replica>shard2</replica>
    </macros>
</clickhouse>
```

### 6. `clickhouse/config/users.xml`

Configures network access and default profile. The ClickHouse Docker image handles user creation automatically when `CLICKHOUSE_USER` and `CLICKHOUSE_PASSWORD` environment variables are set, so this file should NOT redefine the user. It should set:
- `<networks>` allowing connections from all addresses (`<ip>::/0</ip>`)
- Default profile with `allow_experimental_object_type` and `allow_nondeterministic_mutations` if needed

### 7. `clickhouse/init/init.sql`

All DDL uses `ON CLUSTER cluster_2s1r` so it propagates to both shards. All ReplicatedMergeTree tables use Keeper paths with macros: `/clickhouse/tables/{shard}/{database}/{table}`. No comments in the file. All names use snake_case.

**Database: analytics**

| Local Table | Engine | Columns | ORDER BY | PARTITION BY | Indexes |
|---|---|---|---|---|---|
| `analytics.user_events_local` | ReplicatedMergeTree | `event_id UInt64, user_id UInt64, event_type LowCardinality(String), event_timestamp DateTime64(3), page_url String, session_id UUID, duration_seconds Float32, is_mobile UInt8, country_code FixedString(2), payload String` | (user_id, event_timestamp) | toYYYYMM(event_timestamp) | INDEX idx_event_type (event_type) TYPE set(100) GRANULARITY 4, INDEX idx_country (country_code) TYPE bloom_filter(0.01) GRANULARITY 4 |
| `analytics.daily_aggregates_local` | ReplicatedMergeTree | `dt Date, metric_name LowCardinality(String), dimension_key String, total_count UInt64, total_sum Float64, min_value Float64, max_value Float64, avg_value Float64, unique_users UInt64` | (dt, metric_name) | toYYYYMM(dt) | INDEX idx_dimension (dimension_key) TYPE bloom_filter(0.01) GRANULARITY 3 |
| `analytics.session_facts_local` | ReplicatedMergeTree | `session_id UUID, user_id UInt64, started_at DateTime64(3), ended_at DateTime64(3), page_count UInt16, total_duration_seconds UInt32, is_bounce UInt8, entry_url String, exit_url String, device_type LowCardinality(String)` | (user_id, started_at) | toYYYYMM(started_at) | INDEX idx_duration (total_duration_seconds) TYPE minmax GRANULARITY 4 |

Distributed counterparts (same database, no `_local` suffix): `analytics.user_events`, `analytics.daily_aggregates`, `analytics.session_facts` -- each using `Distributed(cluster_2s1r, analytics, <local_table>, rand())`.

**Database: inventory**

| Local Table | Engine | Columns | ORDER BY | PARTITION BY | Indexes |
|---|---|---|---|---|---|
| `inventory.products_local` | ReplicatedMergeTree | `product_id UInt64, sku String, product_name String, category LowCardinality(String), subcategory LowCardinality(String), price Decimal64(2), weight_kg Float32, created_at DateTime, updated_at DateTime, is_active UInt8` | (category, product_id) | is_active | INDEX idx_sku (sku) TYPE bloom_filter(0.01) GRANULARITY 3, INDEX idx_price (price) TYPE minmax GRANULARITY 4 |
| `inventory.stock_movements_local` | ReplicatedMergeTree | `movement_id UInt64, product_id UInt64, warehouse_id UInt16, movement_type LowCardinality(String), quantity Int32, movement_timestamp DateTime64(3), batch_reference String, operator_id UInt32` | (product_id, movement_timestamp) | toYYYYMM(movement_timestamp) | INDEX idx_warehouse (warehouse_id) TYPE set(50) GRANULARITY 4 |
| `inventory.warehouse_snapshots_local` | ReplicatedMergeTree | `snapshot_date Date, warehouse_id UInt16, product_id UInt64, quantity_on_hand Int32, quantity_reserved Int32, quantity_available Int32, last_restock_date Nullable(Date), reorder_point UInt32` | (snapshot_date, warehouse_id, product_id) | toYYYYMM(snapshot_date) | INDEX idx_qty_available (quantity_available) TYPE minmax GRANULARITY 4 |

Distributed counterparts: `inventory.products`, `inventory.stock_movements`, `inventory.warehouse_snapshots`.

**User-Defined Function (factorial):**

```sql
CREATE FUNCTION factorial ON CLUSTER cluster_2s1r AS (n) -> toUInt64(arrayProduct(arrayMap(x -> toUInt64(x), range(1, toUInt64(n) + 1))));
```

Notes:
- `arrayProduct` was introduced in ClickHouse 22.1 and is stable on 24.8 LTS.
- `range(1, n+1)` produces `[1, 2, ..., n]`. For n=0, `range(1, 1)` returns an empty array and `arrayProduct` of an empty array returns 1 (multiplicative identity), which is the correct value for 0!.
- The implementer must verify this edge case during testing. If `arrayProduct([])` returns 0 instead of 1, wrap with `if(n <= 0, toUInt64(1), ...)`.

### 8. `.claude/docs/clickhouse-setup.md`

Documentation file covering:
- Architecture overview (2 shards, 1 keeper, no replicas)
- How to start/stop the stack (`cd clickhouse && docker compose up -d` / `docker compose down`)
- Port mapping table
- How init.sql propagation works (ON CLUSTER DDL)
- How to connect: `clickhouse-client --host localhost --port 19000 --user <user> --password <pass>`
- How to query distributed tables vs local tables
- How macros and ReplicatedMergeTree paths work
- Memory requirements (~2.5 GB total)

### 9. `.gitignore` modification

Add one line at the end of the file:
```
clickhouse/.env
```

---

## Execution Order

1. **Create `clickhouse/.env.example`** -- needed before docker-compose.yml references variables; establishes the credential contract
2. **Create `clickhouse/config/clickhouse-keeper.xml`** -- Keeper must be configured before shards can reference it
3. **Create `clickhouse/config/clickhouse-common.xml`** -- cluster topology definition, depends on knowing keeper hostname from step 2
4. **Create `clickhouse/config/macros-shard1.xml` and `macros-shard2.xml`** -- per-node overrides, depends on cluster name defined in step 3
5. **Create `clickhouse/config/users.xml`** -- user/auth configuration for the ClickHouse nodes
6. **Create `clickhouse/init/init.sql`** -- seed DDL, depends on cluster name from step 3 and macro names from step 4
7. **Create `clickhouse/docker-compose.yml`** -- wires everything together, depends on all config files and init.sql existing at the paths it references
8. **Modify `.gitignore`** -- add `clickhouse/.env` entry to prevent credential leakage
9. **Create `.claude/docs/clickhouse-setup.md`** -- documentation written last since it references final port assignments, file paths, and verified details from all previous steps

---

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Port 19000 or 19001 conflicts with other services on the host | Container fails to start with port binding error | Verified against all ports in root `docker-compose.yml` (5432, 7077, 8080-8082, 8888, 8890, 9000-9001, 18081-18082, 19092, 19644). No conflicts exist. Port 19000 and 19001 are free. |
| `ON CLUSTER` DDL fails if Keeper is not ready when shard1 init runs | Init SQL partially executes; shard2 has no tables | Use `depends_on` with `condition: service_healthy` on keeper. Set generous `start_period` on shard nodes. ClickHouse Docker entrypoint retries init scripts. |
| `CREATE FUNCTION` syntax differs or fails on ClickHouse 24.8 | Init SQL execution stops at the function statement | Place `CREATE FUNCTION` as the **last** statement in init.sql so all tables are created even if this fails. Use `arrayProduct` which is stable since v22.1 and confirmed available on 24.x LTS. |
| ClickHouse Docker image conflicts with custom `users.xml` when `CLICKHOUSE_USER` env is set | Auth errors or duplicate user definitions | Rely on env vars (`CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`) for user creation. The `users.xml` configures only network access and profiles, never redefines the user. |
| `ReplicatedMergeTree` with single replica per shard | Not a real risk: ClickHouse supports ReplicatedMergeTree with exactly 1 replica for metadata management via Keeper | No mitigation needed; this is the intended development cluster configuration. |
| Main stack running simultaneously consumes host memory | Docker host may run out of RAM if both stacks run at once (~10 GB main + ~2.5 GB ClickHouse) | Document in `.claude/docs/clickhouse-setup.md` that the ClickHouse stack needs ~2.5 GB RAM. Advise stopping the main stack if memory is constrained. |
| `arrayProduct` of empty array returns 0 instead of 1 for factorial(0) | Incorrect result for edge case | Test during validation. If needed, wrap with `if(n <= 0, toUInt64(1), ...)` guard. |
| ClickHouse Keeper healthcheck (`echo ruok | nc`) requires `nc` in the keeper image | Healthcheck always fails; dependents never start | The `clickhouse/clickhouse-keeper` image is Debian-based and includes `bash`. If `nc` is unavailable, fall back to `clickhouse-keeper --version` (always succeeds if the binary works) or use `wget` to probe a metrics endpoint. Verify during implementation. |
| Windows line endings (CRLF) in XML config files corrupt ClickHouse parsing | Containers fail to parse config on startup | Ensure all XML files are written with LF line endings. Add a `.gitattributes` entry if needed, or verify the Write tool produces LF output. |

---

## Definition of Done

- [ ] Directory `clickhouse/` exists at the repo root with the following structure:
  ```
  clickhouse/
    docker-compose.yml
    .env.example
    config/
      clickhouse-keeper.xml
      clickhouse-common.xml
      macros-shard1.xml
      macros-shard2.xml
      users.xml
    init/
      init.sql
  ```
- [ ] `clickhouse/docker-compose.yml` defines exactly 3 services: `clickhouse-keeper`, `clickhouse-shard1`, `clickhouse-shard2`
- [ ] All Docker images are pinned to exact version tags (no `latest`)
- [ ] All credentials reference `.env` variables via `${VAR}` syntax -- zero hardcoded passwords in docker-compose.yml or config files
- [ ] Host ports 18123, 18124, 19000, 19001, 19181 are used and do not conflict with any port in the root `docker-compose.yml`
- [ ] Healthchecks are defined for all 3 services
- [ ] Shard services declare `depends_on` with `condition: service_healthy` on the keeper service
- [ ] Named volumes are used for data persistence (not bind mounts or anonymous volumes for data dirs)
- [ ] `clickhouse/config/clickhouse-common.xml` defines cluster `cluster_2s1r` with 2 shards, 1 replica each
- [ ] `clickhouse/config/macros-shard1.xml` sets `{cluster}=cluster_2s1r`, `{shard}=1`, `{replica}=shard1`
- [ ] `clickhouse/config/macros-shard2.xml` sets `{cluster}=cluster_2s1r`, `{shard}=2`, `{replica}=shard2`
- [ ] `clickhouse/config/clickhouse-keeper.xml` configures a single-node Keeper with client port 9181 and raft port 9234
- [ ] `clickhouse/init/init.sql` creates databases `analytics` and `inventory` using `ON CLUSTER cluster_2s1r`
- [ ] `analytics` database contains 3 local `ReplicatedMergeTree` tables and 3 `Distributed` tables (6 total)
- [ ] `inventory` database contains 3 local `ReplicatedMergeTree` tables and 3 `Distributed` tables (6 total)
- [ ] At least one table uses each of these index types: `minmax`, `set`, `bloom_filter`
- [ ] Every table has a `PARTITION BY` clause
- [ ] All `ReplicatedMergeTree` tables use Keeper macro paths (e.g., `/clickhouse/tables/{shard}/{database}/{table}`)
- [ ] One `CREATE FUNCTION` statement exists (factorial) using `ON CLUSTER cluster_2s1r`
- [ ] Zero comments exist in any code file (SQL, YAML, XML) -- all documentation is in `.claude/docs/clickhouse-setup.md`
- [ ] All identifiers use snake_case (databases, tables, columns, aliases, service names)
- [ ] `clickhouse/.env.example` exists with placeholder values
- [ ] `.gitignore` contains `clickhouse/.env`
- [ ] `.claude/docs/clickhouse-setup.md` exists with architecture overview, startup/shutdown instructions, port table, and connection examples
- [ ] `docker compose config` (run from `clickhouse/` directory) validates without errors after copying `.env.example` to `.env`
- [ ] `docker compose up -d` from `clickhouse/` successfully starts all 3 containers and they reach healthy state
- [ ] Connecting to shard1 on port 19000 and running `SELECT cluster, shard_num, replica_num, host_name FROM system.clusters WHERE cluster = 'cluster_2s1r'` returns 2 rows (one per shard)
- [ ] All 6 distributed tables are queryable from either shard node without errors
- [ ] `SELECT factorial(5)` returns 120 on either shard node
