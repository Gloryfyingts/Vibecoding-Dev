# ClickHouse DDL Extraction Script

## Goal

Build a Python CLI tool that connects to a ClickHouse server, extracts all DDL objects, and writes a single `.sql` file that can recreate the entire schema from scratch.

## Connection

- Use the `clickhouse-connect` library (HTTP protocol)
- For local development, connect to `clickhouse-shard1` at `localhost:18123`
- Read credentials from `clickhouse/.env` file (`CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`)
- The script must also accept CLI arguments to override host, port, user, and password for use against any ClickHouse server

## Cluster Handling

- Connect to **one node only**
- Detect cluster topology from `system.clusters`
- Reconstruct `ON CLUSTER <cluster_name>` clauses in the output DDL where applicable
- Must also work correctly against a **single-node** (non-clustered) ClickHouse — omit `ON CLUSTER` when no cluster is detected

## DDL Objects to Extract

Extract and output in this order (dependency-aware where possible):

| # | Object Type | Source |
|---|---|---|
| 1 | Databases | `system.databases` (skip `system`, `information_schema`, `INFORMATION_SCHEMA`, `default`) |
| 2 | Tables — local engines | `system.tables` — `ReplicatedMergeTree`, `MergeTree`, and other local engine families |
| 3 | Views & Materialized Views | `system.tables` where `engine` = `View` / `MaterializedView` |
| 4 | Distributed tables | `system.tables` where `engine` = `Distributed` (must come after their underlying local tables) |
| 5 | Dictionaries | `system.dictionaries` |
| 6 | User-defined functions | `system.functions` where `origin` = `SQLUserDefined` |
| 7 | Settings profiles | `system.settings_profiles` |
| 8 | Row policies | `system.row_policies` |
| 9 | Quotas | `system.quotas` |

**Excluded:** Users, Roles, Grants

## CLI Interface

```
python extract_ddl.py [OPTIONS]

Options:
  --host          ClickHouse host          (default: localhost)
  --port          ClickHouse HTTP port     (default: 18123)
  --user          ClickHouse user          (env: CLICKHOUSE_USER)
  --password      ClickHouse password      (env: CLICKHOUSE_PASSWORD)
  --env-file      Path to .env file        (default: clickhouse/.env)
  --output        Output .sql file path    (default: clickhouse_ddl_dump.sql)
  --databases     Comma-separated list of databases to include (default: all non-system)
  --exclude-dbs   Comma-separated list of databases to exclude
```

Precedence: CLI args > env vars > .env file > defaults.

## Output Format

- A single `.sql` file
- Statements separated by `;\n\n`
- Grouped by object type with comment headers (e.g. `-- === Databases ===`)
- The file should be idempotent where possible (`CREATE DATABASE IF NOT EXISTS`, `CREATE TABLE IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`)
- Reference output style: `clickhouse/init/init.sql`

## Local Development Setup

The local ClickHouse cluster is defined in `clickhouse/docker-compose.yml`:
- 2-shard cluster named `cluster_2s1r`
- `clickhouse-shard1` on ports `18123` (HTTP) / `19000` (native)
- `clickhouse-shard2` on ports `18124` (HTTP) / `19001` (native)
- `clickhouse-keeper` for coordination
- Init data loaded from `clickhouse/init/init.sql` via `clickhouse-init` container

Start with: `cd clickhouse && docker compose up -d`

## Definition of Done

- [ ] Python script using `clickhouse-connect` that extracts all listed DDL object types
- [ ] Outputs a single `.sql` file that can recreate the full schema
- [ ] `ON CLUSTER` clauses reconstructed from system tables (not hardcoded)
- [ ] Works against both clustered and single-node ClickHouse
- [ ] CLI with `argparse` supporting all options listed above
- [ ] Tested against local docker cluster — output should be functionally equivalent to `clickhouse/init/init.sql`
- [ ] `requirements.txt` with pinned dependencies
