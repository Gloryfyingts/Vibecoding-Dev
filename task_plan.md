# Task: ClickHouse DDL Extraction Script

## Summary

Build a Python CLI tool (`clickhouse/extract_ddl.py`) that connects to a ClickHouse server via `clickhouse-connect`, introspects all DDL objects from system tables, reconstructs `ON CLUSTER` clauses dynamically from `system.clusters`, and writes a single idempotent `.sql` file that can recreate the entire schema from scratch. The tool must work against both clustered and single-node ClickHouse instances.

---

## Scope

### Files to CREATE

| File | Purpose |
|---|---|
| `clickhouse/extract_ddl.py` | Main Python CLI script (~350-450 lines) |
| `clickhouse/requirements.txt` | Pinned dependencies for the script |

### Files to MODIFY

None.

### Files NOT modified

| File | Reason |
|---|---|
| `clickhouse/docker-compose.yml` | Infrastructure already exists and is working |
| `clickhouse/init/init.sql` | Reference file only; script output should be functionally equivalent to it |
| `CLAUDE.md` | Not requested |

---

## Detailed File Specifications

### 1. `clickhouse/requirements.txt`

```
clickhouse-connect==0.8.12
python-dotenv==1.0.1
```

Pin exact versions. `clickhouse-connect` is the HTTP-protocol client library specified in the task. `python-dotenv` handles `.env` file loading for credential resolution. No other dependencies are needed.

### 2. `clickhouse/extract_ddl.py`

#### CLI Interface (argparse)

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

Precedence for each parameter: CLI arg > environment variable > .env file value > hardcoded default.

#### Credential Resolution Logic

1. Load `.env` file from `--env-file` path using `python-dotenv` (load_dotenv with override=False so OS env vars take precedence).
2. For `--user`: if CLI arg provided, use it. Else check `os.environ["CLICKHOUSE_USER"]` (which may have been populated from .env). Else fall back to `"default"`.
3. For `--password`: same chain, using `os.environ["CLICKHOUSE_PASSWORD"]`, fallback to `""`.

#### Connection

Use `clickhouse_connect.get_client(host=..., port=..., username=..., password=..., interface="http")`. The `interface="http"` parameter is the default for `clickhouse-connect` and matches port 18123 (HTTP).

#### Cluster Detection

Query `system.clusters` to detect cluster topology:

```sql
SELECT DISTINCT cluster
FROM system.clusters
WHERE cluster NOT IN ('test_shard_localhost', 'test_cluster_one_shard_two_replicas', 'test_cluster_two_shards', 'test_cluster_two_shards_localhost', 'test_unavailable_shard')
```

Filter out ClickHouse's built-in test clusters. The built-in test cluster names vary by version; a safer approach is:

```sql
SELECT cluster, count() AS shard_count
FROM system.clusters
GROUP BY cluster
HAVING shard_count > 0
```

Then cross-reference with `system.databases` or `system.tables` `engine_full` column to find which cluster name appears in actual DDL. The most robust approach: for each table, `SHOW CREATE TABLE` already contains `ON CLUSTER` if it was created that way. However, ClickHouse does NOT persist `ON CLUSTER` in `SHOW CREATE TABLE` output. Therefore, cluster detection must be done separately.

**Chosen approach:** Query `system.clusters` and filter out known internal clusters (those starting with `test_`). If exactly one user-defined cluster remains, use it globally. If multiple clusters exist, determine per-database or per-table which cluster to use by examining `ReplicatedMergeTree` zoo_path patterns (the `{shard}` macro in the path is cluster-specific). If zero clusters found (single-node setup), omit `ON CLUSTER` from all output.

Concrete implementation:

```python
def detect_clusters(client):
    result = client.query(
        "SELECT cluster, shard_num, replica_num, host_name "
        "FROM system.clusters "
        "WHERE cluster NOT LIKE 'test_%' "
        "ORDER BY cluster, shard_num, replica_num"
    )
    clusters = {}
    for row in result.result_rows:
        cluster_name = row[0]
        if cluster_name not in clusters:
            clusters[cluster_name] = []
        clusters[cluster_name].append({
            "shard_num": row[1],
            "replica_num": row[2],
            "host_name": row[3]
        })
    return clusters
```

If the `clusters` dict is empty, the script operates in single-node mode and never emits `ON CLUSTER`. If it has one entry, that cluster name is used for all `ON CLUSTER` clauses. If multiple entries exist, use the cluster whose name appears in the `engine_full` of `Distributed` tables (parsed from `Distributed(cluster_name, ...)` engine definition).

#### DDL Extraction -- Object Types in Order

**1. Databases**

```sql
SELECT name
FROM system.databases
WHERE name NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default')
ORDER BY name
```

If `--databases` is specified, filter to only those. If `--exclude-dbs` is specified, additionally exclude those.

Output format per database:
```sql
CREATE DATABASE IF NOT EXISTS <db_name> ON CLUSTER <cluster>;
```

Omit `ON CLUSTER` in single-node mode.

**2. Tables -- Local Engines (ReplicatedMergeTree, MergeTree, and other non-View/non-Distributed engines)**

Query:
```sql
SELECT database, name, engine, create_table_query
FROM system.tables
WHERE database NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default')
  AND engine NOT IN ('View', 'MaterializedView', 'Distributed')
  AND NOT startsWith(name, '.inner')
ORDER BY database, name
```

The `NOT startsWith(name, '.inner')` filter excludes internal tables backing materialized views (ClickHouse creates `.inner.mv_name` or `.inner_id.<uuid>` tables automatically).

For each table, use `SHOW CREATE TABLE <database>.<table>` to get the full DDL. Then:

a. Inject `IF NOT EXISTS` after `CREATE TABLE` if not already present.
b. Inject `ON CLUSTER <cluster>` after the table name (before the opening parenthesis or `AS` keyword) if cluster mode is active.
c. For `ReplicatedMergeTree` tables: the `SHOW CREATE TABLE` output will contain resolved macro values (e.g., `/clickhouse/tables/1/analytics/user_events_local`). These must be **re-parameterized** back to macros: replace the literal shard number with `{shard}` and the literal replica name with `{replica}`. This is critical for the output DDL to work correctly when applied to a different cluster or when re-applied on both shards.

Re-parameterization logic for ReplicatedMergeTree paths:
- Query the node's macros: `SELECT macro, substitution FROM system.macros`
- In the `SHOW CREATE TABLE` output, find the ReplicatedMergeTree path string (first argument in single quotes).
- Replace the literal `substitution` value of `{shard}` macro with `{shard}`, and `{replica}` with `{replica}`.
- Example: `'/clickhouse/tables/1/analytics/user_events_local'` on shard1 becomes `'/clickhouse/tables/{shard}/analytics/user_events_local'`.

**3. Views and Materialized Views**

```sql
SELECT database, name, engine, create_table_query
FROM system.tables
WHERE database NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default')
  AND engine IN ('View', 'MaterializedView')
  AND NOT startsWith(name, '.inner')
ORDER BY database, engine, name
```

Use `SHOW CREATE TABLE` for each. Inject `IF NOT EXISTS` and `ON CLUSTER` as with local tables.

**Dependency ordering note:** Materialized views depend on their source tables. Since local tables are extracted in step 2, this ordering is naturally correct. If a materialized view depends on another view, topological sorting within this group may be needed. For the current schema (no views exist), this is not a concern, but the implementation should handle it gracefully by catching errors in the ordering and falling back to alphabetical.

**4. Distributed Tables**

```sql
SELECT database, name, engine, engine_full, create_table_query
FROM system.tables
WHERE database NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default')
  AND engine = 'Distributed'
ORDER BY database, name
```

Use `SHOW CREATE TABLE` for each. The output for Distributed tables created with `AS <source_table>` syntax will be expanded to full column definitions. This is acceptable -- the output is functionally equivalent.

Inject `IF NOT EXISTS` and `ON CLUSTER` as above.

**5. Dictionaries**

```sql
SELECT database, name, source
FROM system.dictionaries
WHERE database NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default')
ORDER BY database, name
```

For each dictionary: `SHOW CREATE DICTIONARY <database>.<name>`.
Inject `ON CLUSTER` if applicable.

If no dictionaries exist (as in the current schema), the section header is still emitted with a comment `-- (none)` or simply omitted. Per project rules (no comments in code), omit the entire section if empty.

**6. User-Defined Functions**

```sql
SELECT name, create_query
FROM system.functions
WHERE origin = 'SQLUserDefined'
ORDER BY name
```

Note: `system.functions` has a `create_query` column in ClickHouse 22.8+. Use it directly. The `create_query` from `system.functions` does NOT include `ON CLUSTER`. Inject it if cluster mode is active.

For idempotency, use `CREATE OR REPLACE FUNCTION` instead of `CREATE FUNCTION`. Replace in the extracted DDL string.

**7. Settings Profiles**

```sql
SELECT name
FROM system.settings_profiles
WHERE storage = 'local_directory'
ORDER BY name
```

The `storage = 'local_directory'` filter excludes built-in profiles. For each: `SHOW CREATE SETTINGS PROFILE <name>`. Inject `ON CLUSTER` if applicable.

Alternatively, if `system.settings_profiles` does not exist (older versions) or has no user-defined entries, skip this section.

**8. Row Policies**

```sql
SELECT name, short_name, database, table
FROM system.row_policies
WHERE storage = 'local_directory'
ORDER BY name
```

For each: `SHOW CREATE ROW POLICY <short_name> ON <database>.<table>`. Inject `ON CLUSTER` if applicable.

**9. Quotas**

```sql
SELECT name
FROM system.quotas
WHERE storage = 'local_directory'
ORDER BY name
```

For each: `SHOW CREATE QUOTA <name>`. Inject `ON CLUSTER` if applicable.

#### Output File Format

The file structure follows the style of `clickhouse/init/init.sql`:

```sql
-- === Databases ===

CREATE DATABASE IF NOT EXISTS analytics ON CLUSTER cluster_2s1r;

CREATE DATABASE IF NOT EXISTS inventory ON CLUSTER cluster_2s1r;

-- === Tables (Local Engines) ===

CREATE TABLE IF NOT EXISTS analytics.daily_aggregates_local ON CLUSTER cluster_2s1r
(
    ...
)
ENGINE = ReplicatedMergeTree(...)
...;

...

-- === Views and Materialized Views ===

...

-- === Distributed Tables ===

CREATE TABLE IF NOT EXISTS analytics.user_events ON CLUSTER cluster_2s1r
...;

-- === Dictionaries ===

...

-- === User-Defined Functions ===

CREATE OR REPLACE FUNCTION factorial ON CLUSTER cluster_2s1r AS ...;

-- === Settings Profiles ===

...

-- === Row Policies ===

...

-- === Quotas ===

...
```

Statements separated by `;\n\n`. Section headers as `-- === <Type> ===` comment lines. Empty sections (no objects of that type) are omitted entirely -- no empty section headers.

**Important note on section headers:** The task prompt says "Grouped by object type with comment headers." The project rule says "No comments in code." Section headers in the OUTPUT SQL file are metadata/documentation for the generated file, not comments in the Python source code. The Python source code itself must have zero comments. The generated `.sql` output file has section header comments as specified by the task.

#### ON CLUSTER Injection Logic

The `SHOW CREATE TABLE` output from ClickHouse never includes `ON CLUSTER`. The script must inject it. The injection point depends on the DDL type:

- `CREATE DATABASE <name>` -> `CREATE DATABASE IF NOT EXISTS <name> ON CLUSTER <cluster>`
- `CREATE TABLE <db>.<name> (...)` -> `CREATE TABLE IF NOT EXISTS <db>.<name> ON CLUSTER <cluster> (...)`
- `CREATE MATERIALIZED VIEW <db>.<name>` -> `CREATE MATERIALIZED VIEW IF NOT EXISTS <db>.<name> ON CLUSTER <cluster>`
- `CREATE DICTIONARY <db>.<name>` -> `CREATE DICTIONARY IF NOT EXISTS <db>.<name> ON CLUSTER <cluster>`
- `CREATE FUNCTION <name>` -> `CREATE OR REPLACE FUNCTION <name> ON CLUSTER <cluster>`
- `CREATE SETTINGS PROFILE <name>` -> inject `ON CLUSTER` after profile name
- `CREATE ROW POLICY <name> ON <db>.<table>` -> inject `ON CLUSTER` after policy name, before `ON <db>.<table>`
- `CREATE QUOTA <name>` -> inject `ON CLUSTER` after quota name

Implementation: use regex-based string manipulation. For each DDL type, define a pattern that matches the object identifier and inject `ON CLUSTER <cluster>` immediately after it.

Concrete regex for table DDL injection:
```python
import re

def inject_on_cluster(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = r"(CREATE\s+(?:OR\s+REPLACE\s+)?(?:TABLE|DICTIONARY|MATERIALIZED\s+VIEW|VIEW|FUNCTION|DATABASE|SETTINGS\s+PROFILE|QUOTA)(?:\s+IF\s+NOT\s+EXISTS)?\s+`?[\w.]+`?)"

    replacement = rf"\1 ON CLUSTER {cluster_name}"
    return re.sub(pattern, replacement, ddl, count=1)
```

This is simplified; the actual implementation needs to handle edge cases (backtick-quoted identifiers, `ROW POLICY ... ON db.table`, etc.). Each DDL type should have its own injection function for clarity.

#### ReplicatedMergeTree Path Re-parameterization

When ClickHouse returns `SHOW CREATE TABLE` for a ReplicatedMergeTree table, the zoo_path and replica_name arguments contain resolved literal values, not macros:

```sql
ENGINE = ReplicatedMergeTree('/clickhouse/tables/1/analytics/user_events_local', 'shard1')
```

The script must convert these back to macro form:

```sql
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/analytics/user_events_local', '{replica}')
```

Steps:
1. At startup, query `SELECT macro, substitution FROM system.macros` to get current node's macro values.
2. For each ReplicatedMergeTree DDL, find the engine arguments (two single-quoted strings).
3. In the first argument (zoo_path), replace the literal shard value with `{shard}`.
4. In the second argument (replica_name), replace the literal replica value with `{replica}`.

Edge case: if macros are not set (single-node without macros), skip re-parameterization.

Concrete implementation:
```python
def reparameterize_replicated_paths(ddl, macros):
    shard_val = macros.get("shard")
    replica_val = macros.get("replica")
    if not shard_val or not replica_val:
        return ddl

    def replace_engine_args(match):
        full = match.group(0)
        full = full.replace(f"/{shard_val}/", "/{shard}/")
        full = full.replace(f"'{replica_val}'", "'{replica}'")
        return full

    return re.sub(
        r"ReplicatedMergeTree\([^)]+\)",
        replace_engine_args,
        ddl
    )
```

**Risk:** The literal shard value (e.g., `1`) could appear elsewhere in the path (e.g., in a table name containing `1`). Mitigation: only replace within the first argument's path segments, specifically targeting `/<shard_val>/` with surrounding slashes.

#### Distributed Table: AS-syntax vs Full Column List

The original `init.sql` uses `AS analytics.user_events_local` shorthand:
```sql
CREATE TABLE IF NOT EXISTS analytics.user_events ON CLUSTER cluster_2s1r
AS analytics.user_events_local
ENGINE = Distributed(cluster_2s1r, analytics, user_events_local, rand());
```

But `SHOW CREATE TABLE` for a Distributed table returns the full column list. This is functionally equivalent and acceptable. The output will be valid DDL that recreates the table identically.

#### Error Handling

- If connection fails, print a clear error message with host:port and exit with code 1.
- If a `SHOW CREATE` query fails for a specific object, print a warning to stderr and continue with the next object (do not abort the entire extraction).
- If the output file cannot be written, exit with code 1.

#### Script Structure (function layout)

```
extract_ddl.py
  parse_args()                    -> argparse.Namespace
  load_credentials(args)          -> (user, password)
  connect(host, port, user, pw)   -> clickhouse_connect.Client
  detect_clusters(client)         -> dict[str, list]
  resolve_cluster(clusters, client) -> str | None
  get_macros(client)              -> dict[str, str]
  get_databases(client, include, exclude) -> list[str]
  get_tables(client, databases, engine_filter) -> list[dict]
  extract_databases(client, cluster, databases) -> list[str]
  extract_local_tables(client, cluster, macros, databases) -> list[str]
  extract_views(client, cluster, databases) -> list[str]
  extract_distributed_tables(client, cluster, databases) -> list[str]
  extract_dictionaries(client, cluster, databases) -> list[str]
  extract_functions(client, cluster) -> list[str]
  extract_settings_profiles(client, cluster) -> list[str]
  extract_row_policies(client, cluster) -> list[str]
  extract_quotas(client, cluster) -> list[str]
  inject_if_not_exists(ddl)       -> str
  inject_on_cluster(ddl, cluster) -> str
  reparameterize_replicated_paths(ddl, macros) -> str
  format_output(sections)         -> str
  main()
```

No classes needed. Pure functions. No comments in the Python source code.

---

## Execution Order

1. **Create `clickhouse/requirements.txt`** -- must exist first so the developer can `pip install -r requirements.txt` before running the script. No dependencies on other files.

2. **Create `clickhouse/extract_ddl.py`** -- the main deliverable. Depends on `requirements.txt` being installable. Depends on the ClickHouse cluster being running (for testing), but not for file creation.

3. **Test against local cluster** -- run `cd clickhouse && docker compose up -d`, wait for healthy state, then run `python extract_ddl.py --output test_output.sql`. Compare `test_output.sql` against `init/init.sql` for functional equivalence.

---

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| `SHOW CREATE TABLE` output format varies between ClickHouse versions | Regex-based ON CLUSTER injection or path re-parameterization breaks | Test against ClickHouse 24.8.7.41 (the pinned version in docker-compose.yml). Use flexible regex patterns that handle optional whitespace and backtick quoting. |
| `system.functions` column `create_query` does not exist in older ClickHouse versions | Script crashes when extracting UDFs | Guard with a try/except. If the column is missing, fall back to `SHOW CREATE FUNCTION <name>`. ClickHouse 22.8+ has `create_query`; our cluster runs 24.8, so this is low risk. |
| `system.settings_profiles`, `system.row_policies`, `system.quotas` may have no `storage` column in some versions or the tables may not exist | Script crashes on access control queries | Wrap each access control extraction in try/except. If the system table does not exist or the query fails, skip that section gracefully. |
| Literal shard value (e.g., `1`) appears in table names or other parts of ReplicatedMergeTree path | False-positive replacement corrupts the zoo_path | Only replace `/<shard_val>/` (with surrounding slashes) in the first quoted argument of ReplicatedMergeTree. This ensures we only hit the path segment, not arbitrary substrings. |
| `clickhouse-connect` version incompatibility | Import errors or API changes | Pin to exact version 0.8.12 in requirements.txt. This version is stable and well-tested with ClickHouse 24.x. |
| Multiple user-defined clusters exist (e.g., production environments) | Script picks the wrong cluster or emits incorrect ON CLUSTER clauses | If multiple clusters detected, use the one that appears in Distributed table engine definitions. If no Distributed tables exist, use the first cluster alphabetically and warn to stderr. |
| `.env` file path is relative and depends on working directory | Script fails to find `.env` when run from a different directory | Resolve `--env-file` path relative to the script's own directory if the provided path is relative. Use `pathlib.Path(__file__).parent / ".env"` as default. |
| `ON CLUSTER` injection regex fails on DDL with unusual formatting (newlines, extra spaces) | ON CLUSTER clause not injected or injected in wrong position | Use `re.IGNORECASE` and `\s+` to handle whitespace variations. Include comprehensive pattern alternatives for each DDL type. |
| ROW POLICY `SHOW CREATE` syntax: `ROW POLICY name ON db.table` -- the `ON` keyword conflicts with `ON CLUSTER` | ON CLUSTER injected in wrong position for row policies | Handle ROW POLICY as a special case: inject `ON CLUSTER` between the policy name and `ON db.table`. |
| Single-node ClickHouse has no entries in `system.clusters` or only has `test_*` entries | Script incorrectly thinks it is single-node when a real cluster exists, or vice versa | After filtering `test_*` clusters, also check for clusters named `default` (ClickHouse auto-creates this in some configurations). If the user wants a specific cluster, they could pass it via a future `--cluster` flag, but this is out of scope for now. |
| `python-dotenv` not installed on user's system | Import error on startup | Clearly stated in requirements.txt. The script's import section will catch ImportError for `dotenv` and print a message directing user to `pip install -r requirements.txt`. |

---

## Definition of Done

- [ ] File `clickhouse/extract_ddl.py` exists and is a valid Python 3.10+ script
- [ ] File `clickhouse/requirements.txt` exists with pinned versions of `clickhouse-connect` and `python-dotenv`
- [ ] Script uses `clickhouse-connect` library (HTTP protocol) for all ClickHouse communication
- [ ] CLI accepts all options specified: `--host`, `--port`, `--user`, `--password`, `--env-file`, `--output`, `--databases`, `--exclude-dbs`
- [ ] Credential precedence is: CLI args > env vars > .env file > defaults
- [ ] Script detects cluster topology from `system.clusters` (not hardcoded)
- [ ] Script emits `ON CLUSTER <cluster_name>` in output DDL when a cluster is detected
- [ ] Script omits `ON CLUSTER` when no user-defined cluster is detected (single-node mode)
- [ ] Script extracts all 9 object types in the specified order: databases, local tables, views/materialized views, distributed tables, dictionaries, UDFs, settings profiles, row policies, quotas
- [ ] System databases (`system`, `information_schema`, `INFORMATION_SCHEMA`, `default`) are always excluded
- [ ] `--databases` and `--exclude-dbs` filters work correctly
- [ ] Output is a single `.sql` file with statements separated by `;\n\n`
- [ ] Output uses section headers like `-- === Databases ===`
- [ ] Empty sections (no objects of that type) are omitted from the output entirely
- [ ] Output DDL is idempotent: `CREATE DATABASE IF NOT EXISTS`, `CREATE TABLE IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`
- [ ] ReplicatedMergeTree zoo_path and replica arguments are re-parameterized from literal values back to `{shard}` and `{replica}` macros
- [ ] Internal tables (`.inner.*`) are excluded from extraction
- [ ] Distributed tables appear after their underlying local tables in the output
- [ ] The Python source file contains zero comments (per project rules)
- [ ] Zero hardcoded credentials in the script
- [ ] Running `python extract_ddl.py` against the local docker cluster (`localhost:18123`) produces a `.sql` file that is functionally equivalent to `clickhouse/init/init.sql` (same databases, same tables with same columns/engines/indexes/partitioning, same UDF)
- [ ] The script exits with code 0 on success and code 1 on connection failure or write failure
- [ ] Failed `SHOW CREATE` for individual objects prints a warning to stderr but does not abort the full extraction
