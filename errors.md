# Code Review Report — Docker Compose Local Development Environment (Re-Review)

**Reviewer:** Claude Code (Senior Data Engineering Reviewer)
**Date:** 2026-03-21
**Branch:** AI/Master
**Task:** Local Development Environment -- Docker Compose
**Review round:** 2 (post-fix re-review)
**Files reviewed:** 9

---

## Changes Verified in This Round

| # | Change | File | Status |
|---|--------|------|--------|
| 1 | Added `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` env vars to jupyter service | `docker-compose.yml` | RESOLVED |
| 2 | Added `import os`, replaced hardcoded connection params with `os.environ.get()` | `notebooks/test_e2e.ipynb` Cell 0 | RESOLVED |
| 3 | Added `import os`, replaced hardcoded connection params with `os.environ.get()` | `notebooks/test_e2e.ipynb` Cell 1 | RESOLVED |
| 4 | Replaced hardcoded `pipeline_user`/`pipeline_pass` in SparkSession config with env vars; fixed `aws_secret` fallback from fabricated `minioadmin123` to `minioadmin` | `notebooks/test_e2e.ipynb` Cell 3 | RESOLVED |

---

## Review: `docker-compose.yml`

### Confirmed Correct
- Single `data-pipeline` bridge network; all 10 services attached.
- Named volumes `postgres-data` and `minio-data` declared at top level.
- All image versions pinned; no `latest` tags.
- All credentials reference `.env` via `${VAR}` — no hardcoded secrets.
- `minio-init` uses `restart: "no"` and `$$` escaping in entrypoint — correct.
- Redpanda dual-listener config correct; no host port conflicts with Flink (8081) or Spark Worker (8082).
- All `FLINK_PROPERTIES` keys present and correct on both JobManager and TaskManager.
- `flink-taskmanager` has no host ports exposed.
- All `depends_on` blocks use `condition: service_healthy`.
- Jupyter service now correctly forwards `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` alongside existing AWS and Jupyter env vars.
- All memory limits, port mappings, bind mounts, and healthchecks match spec.

### Warning (carried over from round 1)
- [lines 144, 184, 268] `pull_policy: build` is redundant when a `build` block is already present on the same service. Docker Compose V2 already treats services with a `build` block as locally built. Not incorrect, not a blocker.

---

## Review: `notebooks/test_e2e.ipynb`

### Confirmed Correct
- All 4 cells present; Cell 2 is markdown type.
- **Cell 0:** `import os` at top. `dbname`, `user`, `password` all use `os.environ.get()` with `.env.example`-consistent fallbacks.
- **Cell 1:** `import os` at top. `dbname`, `user`, `password` all use `os.environ.get()`. Explicit column list `SELECT event_id, user_id, event_type, payload, created_at FROM app.events`. Datetime and None handling correct.
- **Cell 2:** Markdown — unchanged, correct per spec.
- **Cell 3:** `import os` at top. `pg_user` and `pg_password` extracted from env before SparkSession build. `aws_key` and `aws_secret` extracted from env with correct fallbacks (`minioadmin` matches `.env.example`). SparkSession config references variables, not literals. All Iceberg catalog config keys present. Reads `s3a://raw/events/`, writes `iceberg.db.events`, verification query uses explicit columns and `cnt` alias.

### No new issues introduced.

---

## Review: `.gitignore`

### Confirmed Correct — unchanged from round 1
- `.env` listed — primary security requirement satisfied.
- `.ipynb_checkpoints/` present.
- Python cache patterns included.

### Suggestion (carried over from round 1)
- AI artifact files (`errors.md`, `task_plan.md`) not listed. Adding them would provide a safety layer on top of the push skill exclusion logic. Not required by spec.

---

## Review: `.env.example`

### Confirmed Correct — unchanged from round 1
- All six required variables present: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`, `JUPYTER_TOKEN`.
- Placeholder values (`CHANGE_ME`) for sensitive fields.
- Matches spec section 2 exactly.

---

## Review: `docker/postgres/init.sql`

### Confirmed Correct — unchanged from round 1
- Creates `app` and `iceberg_catalog` schemas with `IF NOT EXISTS` guards.
- No explicit GRANT needed — POSTGRES_USER is superuser/owner.
- No comments. 2-line file, minimal and complete.

---

## Review: `docker/flink/Dockerfile`

### Confirmed Correct — unchanged from round 1
- Base image `flink:1.20.1-scala_2.12-java11` pinned.
- S3 plugin enabled via JAR copy with mkdir fallback.
- Kafka connector `flink-sql-connector-kafka-3.3.0-1.20.jar` downloaded — correct version for Flink 1.20.
- No comments.

---

## Review: `docker/jupyter/Dockerfile`

### Confirmed Correct — unchanged from round 1
- Base image `quay.io/jupyter/pyspark-notebook:spark-3.5.4` pinned.
- `psycopg2-binary` and `kafka-python-ng` installed with `--no-cache-dir`.
- No comments.

---

## Review: `flink-jobs/sql/e2e_redpanda_to_s3.sql`

### Confirmed Correct — unchanged from round 1
- Checkpoint interval set at line 1.
- Source and sink DDL match spec column names and types.
- Kafka connector options correct.
- Filesystem sink path `s3://raw/events/` correct for Flink S3 hadoop plugin.
- `INSERT INTO` uses explicit column list.

---

## Review: `flink-jobs/python/e2e_redpanda_to_s3.py`

### Confirmed Correct — unchanged from round 1
- Correct PyFlink API usage (`StreamExecutionEnvironment`, `StreamTableEnvironment`).
- Checkpointing enabled at 10000ms.
- DDL identical to SQL variant.
- `execute_sql()` used for DML — correct.

### Suggestion (carried over from round 1)
- No `if __name__ == '__main__':` guard. Not blocking for `flink run --python` execution pattern.

---

## Definition of Done Verification

### Infrastructure
- [PASS] `docker-compose.yml` defines all 10 services.
- [CANNOT VERIFY — runtime] `docker compose up -d` starts all containers without errors.
- [CANNOT VERIFY — runtime] All healthchecks pass within 2 minutes.
- [PASS] Single `data-pipeline` network.
- [PASS] No `latest` tags — all images pinned.

### Credentials and Configuration
- [PASS] `.env.example` exists with all 6 variables.
- [CANNOT VERIFY — not committed, correct] `.env` exists locally.
- [PASS] `.gitignore` includes `.env`.
- [PASS] **No credentials hardcoded in any committed file** — all 4 criticals from round 1 resolved.

### Web UIs
- [CANNOT VERIFY — runtime] All UIs accessible at correct ports.

### Bootstrapping
- [PASS] `docker/postgres/init.sql` creates `app` and `iceberg_catalog` schemas.
- [PASS] `minio-init` creates `warehouse` and `raw` buckets.

### Resource Limits
- [PASS] All memory limits match spec.

### Volumes and Bind Mounts
- [PASS] `postgres-data` and `minio-data` named volumes declared.
- [PASS] `./notebooks` bind-mounted into Jupyter.
- [PASS] `./flink-jobs` bind-mounted into both Flink containers.

### Custom Dockerfiles
- [PASS] `docker/flink/Dockerfile` correct.
- [PASS] `docker/jupyter/Dockerfile` correct.

### E2E Test Pipeline
- [PASS] `notebooks/test_e2e.ipynb` has all 4 cells.
- [PASS] `flink-jobs/sql/e2e_redpanda_to_s3.sql` valid.
- [PASS] `flink-jobs/python/e2e_redpanda_to_s3.py` valid.
- [CANNOT VERIFY — runtime] Test Steps 1-4 execution results.

### Documentation
- [PASS] `CLAUDE.md` has `## Local Environment` section (confirmed via system context).
- [PASS] Existing CLAUDE.md content preserved.
- [PASS] Test pipeline not documented in CLAUDE.md.

---

## Consolidated Issue List

| ID | Severity | File | Description | Round 1 Status | Round 2 Status |
|----|----------|------|-------------|----------------|----------------|
| CRITICAL-1 | Critical | `notebooks/test_e2e.ipynb` Cell 3 | `aws_secret` fallback `'minioadmin123'` fabricated credential | OPEN | RESOLVED |
| CRITICAL-2 | Critical | `notebooks/test_e2e.ipynb` Cell 0 | `password='pipeline_pass'` hardcoded literal | OPEN | RESOLVED |
| CRITICAL-3 | Critical | `notebooks/test_e2e.ipynb` Cell 1 | `password='pipeline_pass'` hardcoded literal | OPEN | RESOLVED |
| CRITICAL-4 | Critical | `notebooks/test_e2e.ipynb` Cell 3 | `jdbc.user`/`jdbc.password` hardcoded in SparkSession config | OPEN | RESOLVED |
| WARNING-1 | Warning | `docker-compose.yml` lines 144, 184, 268 | `pull_policy: build` redundant alongside `build` block | OPEN | OPEN (not blocking) |
| SUGGESTION-1 | Suggestion | `flink-jobs/python/e2e_redpanda_to_s3.py` | No `if __name__ == '__main__':` guard | OPEN | OPEN (not blocking) |
| SUGGESTION-2 | Suggestion | `.gitignore` | AI artifact files not listed | OPEN | OPEN (not blocking) |

---

## Summary

**Files reviewed:** 9
**Critical:** 0 (was 4 in round 1 — all resolved)
**Warning:** 1 (carried over — not blocking)
**Suggestion:** 2 (carried over — not blocking)
**Verdict: APPROVE**

All four critical issues from the round 1 review are correctly and completely resolved. The credential fix pattern is consistent: `os.environ.get('VAR', 'fallback')` with fallback values matching `.env.example` defaults across all affected cells. The `docker-compose.yml` change correctly injects `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` into the Jupyter container environment, closing the loop between the host `.env` file and the notebook runtime. No new issues were introduced by any of the four changes. The infrastructure layer remains fully correct. All statically verifiable Definition of Done items pass.

---

---

# Code Review Report — ClickHouse Local Stand

**Reviewer:** Claude Code (Senior Data Engineering Reviewer)
**Date:** 2026-03-22
**Branch:** AI/Master
**Task:** Standalone ClickHouse Docker Compose Setup
**Review round:** 1
**Files reviewed:** 12

---

## Review: `clickhouse/.env.example`

No issues. Three required variables (`CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT`) present with placeholder values. No hardcoded secrets.

---

## Review: `clickhouse/config/clickhouse-keeper.xml`

No issues. TCP port 9181, server_id 1, correct log and snapshot paths, raft port 9234, coordination settings all match the approved plan exactly.

---

## Review: `clickhouse/config/clickhouse-common.xml`

No issues. Cluster `cluster_2s1r` defined with 2 shards, 1 replica each. Keeper connection correct. `<distributed_ddl>` path present. Inter-shard credentials use `from_env` attribute — no hardcoded passwords.

---

## Review: `clickhouse/config/macros-shard1.xml`

No issues. Values match plan: `{cluster}=cluster_2s1r`, `{shard}=1`, `{replica}=shard1`.

---

## Review: `clickhouse/config/macros-shard2.xml`

No issues. Values match plan: `{cluster}=cluster_2s1r`, `{shard}=2`, `{replica}=shard2`.

---

## Review: `clickhouse/config/users.xml`

### Critical

- [lines:8-16] The `default` user is configured with unrestricted network access (`<ip>::/0</ip>`) but ClickHouse's built-in `default` user has no password by default. This creates an unauthenticated access vector from any container on the `clickhouse-cluster` Docker network. The plan spec required "network access from all addresses" but did not account for the passwordless default user security exposure. -> Either restrict `default` user `<networks>` to localhost only (`<ip>127.0.0.1</ip>` and `<ip>::1</ip>`), or add a non-empty password hash. The intended admin account is the one created via the `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` env vars — `default` should not be externally accessible.

---

## Review: `clickhouse/init/init.sql`

### Warning

- [line:134] `CREATE OR REPLACE FUNCTION factorial ON CLUSTER cluster_2s1r` deviates from the approved plan which specifies `CREATE FUNCTION factorial ON CLUSTER cluster_2s1r`. Functionally this is more idempotent, but it is an unapproved deviation from the approved plan. -> Either update the plan to approve `CREATE OR REPLACE FUNCTION`, or revert to `CREATE FUNCTION` (and rely on the `FUNCTION_ALREADY_EXISTS` guard in `init.sh`).

All other items pass: 14 DDL statements all use `ON CLUSTER cluster_2s1r`, macro paths correct, all 3 index types (`set`, `bloom_filter`, `minmax`) present, all 6 local tables have `PARTITION BY`, zero comments, all identifiers snake_case.

---

## Review: `clickhouse/init/init.sh`

### Critical

- [line:7] `SELECT * FROM system.zookeeper WHERE path='/' LIMIT 1` violates the CLAUDE.md rule "No SELECT *". Explicit column list required even for system table queries. -> Replace with `SELECT name FROM system.zookeeper WHERE path='/' LIMIT 1`.

### Warning

- [lines:19-26] Dead code: the `FUNCTION_ALREADY_EXISTS` error handler is unreachable because `init.sql` uses `CREATE OR REPLACE FUNCTION`, which never raises this error. -> Remove the `FUNCTION_ALREADY_EXISTS` branch. If `init.sql` is reverted to `CREATE FUNCTION`, this guard becomes necessary again — resolve together with the `init.sql` warning above.

---

## Review: `clickhouse/docker-compose.yml`

### Critical

- [lines:85-103] A fourth service `clickhouse-init` is defined. The approved definition of done is unambiguous: "defines exactly 3 services: clickhouse-keeper, clickhouse-shard1, clickhouse-shard2". The implementation added an undocumented init container that is not in the approved plan. The approved plan explicitly stated that init.sql should run via `/docker-entrypoint-initdb.d/` on `clickhouse-shard1`. -> Either remove `clickhouse-init` and restore the bind-mount of `init/` to `/docker-entrypoint-initdb.d/` on `clickhouse-shard1` (as approved), or submit a plan amendment and get it approved before merging.

- [lines:85-103] `clickhouse-init` service has no healthcheck. The definition of done states "Healthchecks are defined for all 3 services." If the 4th service is retained it also requires a healthcheck definition. -> Add healthcheck, or remove the service.

### Warning

- [lines:85-103] `clickhouse-init` has no `deploy.resources.limits` block. Every other service in the file defines a memory limit. This is inconsistent. -> Add `deploy.resources.limits.memory: 128m`.

- [lines:31-34] `clickhouse-shard1` does not mount `init/` to `/docker-entrypoint-initdb.d/` as approved in the plan. The init approach was changed from the plan's bind-mount method to a separate container without plan approval. -> Revert to the approved approach or get the plan amended.

---

## Review: `.claude/docs/clickhouse-setup.md`

### Critical

- [line:93] Documentation states "Only `clickhouse-shard1` mounts the `init/` directory to `/docker-entrypoint-initdb.d/`" — this is factually wrong. The actual implementation uses a separate `clickhouse-init` container that executes `init.sql` via `clickhouse-client` over TCP after both shards reach healthy state. The documentation describes the plan, not the implementation. -> Update this section to accurately describe the `clickhouse-init` container approach: it waits for Keeper readiness, then calls `clickhouse-client --host clickhouse-shard1 --multiquery < /init.sql`.

- [line:187] `SELECT * FROM system.zookeeper WHERE path = '/clickhouse/task_queue/ddl'` in a code example violates CLAUDE.md rule "No SELECT *". All code — including documentation examples — must use explicit column lists. -> Replace with an explicit column list, for example: `SELECT name, value FROM system.zookeeper WHERE path = '/clickhouse/task_queue/ddl'`.

### Warning

- [line:38] "then start and run `init.sql`" is inaccurate. Shard nodes do not run `init.sql` directly in the implemented architecture. -> Fix to describe the `clickhouse-init` container approach accurately.

- [lines:210-225] `init.sh` is missing from the file structure diagram. The file was created, is mounted in `docker-compose.yml`, and is critical to the init flow, but does not appear in the `clickhouse/init/` listing. -> Add `init.sh` to the file structure under `init/`.

---

## Review: `.gitignore`

No issues. `clickhouse/.env` is present at line 10.

---

## Review: `INFRA.md`

### Warning

- [line:38] "`factorial(n)` — available on both shards via ClickHouse built-in" is factually incorrect. `factorial` is not a ClickHouse built-in function. It is a user-defined function (UDF) created by `init.sql` using `CREATE OR REPLACE FUNCTION ... AS (n) -> ...` syntax. -> Change to "available on both shards via user-defined function (UDF) defined in `clickhouse/init/init.sql`".

---

## Definition of Done — Gap Analysis (ClickHouse Stand)

| DoD Item | Status | Notes |
|---|---|---|
| Directory `clickhouse/` exists with correct structure | PARTIAL | `init.sh` was added but is absent from the plan's file structure spec |
| `docker-compose.yml` defines exactly 3 services | FAIL | 4 services defined: keeper, shard1, shard2, init |
| All Docker images pinned to exact version tags | PASS | All use `24.8.7.41`, no `latest` |
| All credentials reference `.env` variables via `${VAR}` | PASS | No hardcoded passwords in docker-compose or XML files |
| Host ports 18123, 18124, 19000, 19001, 19181 used and conflict-free | PASS | All correct, verified against main stack port list |
| Healthchecks defined for all 3 services | FAIL | `clickhouse-init` (4th service) has no healthcheck; original 3 pass |
| Shard services `depends_on` keeper with `service_healthy` | PASS | Both shards declare correct dependency |
| Named volumes for data persistence | PASS | 3 named volumes declared and used |
| `clickhouse-common.xml` defines `cluster_2s1r` with 2 shards, 1 replica each | PASS | Topology correct |
| `macros-shard1.xml` values correct | PASS | cluster=cluster_2s1r, shard=1, replica=shard1 |
| `macros-shard2.xml` values correct | PASS | cluster=cluster_2s1r, shard=2, replica=shard2 |
| `clickhouse-keeper.xml` configures Keeper with port 9181 and raft 9234 | PASS | Correct |
| `init.sql` creates `analytics` and `inventory` databases `ON CLUSTER` | PASS | Correct |
| `analytics` contains 3 local ReplicatedMergeTree + 3 Distributed tables | PASS | Correct |
| `inventory` contains 3 local ReplicatedMergeTree + 3 Distributed tables | PASS | Correct |
| All three index types present: `minmax`, `set`, `bloom_filter` | PASS | All three used across tables |
| Every local table has `PARTITION BY` | PASS | All 6 local tables partitioned |
| All ReplicatedMergeTree tables use Keeper macro paths | PASS | All use `/clickhouse/tables/{shard}/{database}/{table}` |
| One `CREATE FUNCTION factorial ON CLUSTER` statement | PARTIAL | Uses `CREATE OR REPLACE FUNCTION` — function exists, wording deviates from plan |
| Zero comments in any code file (SQL, YAML, XML) | PASS | No comments found |
| All identifiers use snake_case | PASS | Compliant throughout |
| `clickhouse/.env.example` exists with placeholder values | PASS | Correct |
| `.gitignore` contains `clickhouse/.env` | PASS | Line 10 |
| `.claude/docs/clickhouse-setup.md` exists with required sections | PARTIAL | Exists but contains factual inaccuracies and a `SELECT *` in a code example |
| `docker compose config` validates without errors | UNKNOWN | Cannot run in read-only review mode |
| `docker compose up -d` starts all containers and they reach healthy | UNKNOWN | Cannot verify in read-only review mode |
| Cluster query returns 2 rows | UNKNOWN | Cannot verify in read-only review mode |
| All 6 distributed tables queryable | UNKNOWN | Cannot verify in read-only review mode |
| `SELECT factorial(5)` returns 120 | UNKNOWN | Cannot verify in read-only review mode |

---

## Items That Must Be Reworked Before Approval

| # | Severity | File | Issue |
|---|---|---|---|
| 1 | Critical | `clickhouse/docker-compose.yml` lines 85-103 | 4th service `clickhouse-init` violates DoD "exactly 3 services". Remove it or get plan amended. |
| 2 | Critical | `clickhouse/docker-compose.yml` lines 85-103 | `clickhouse-init` has no healthcheck. |
| 3 | Critical | `clickhouse/config/users.xml` lines 8-16 | Passwordless `default` user exposed to all network addresses. Restrict to localhost or set a password. |
| 4 | Critical | `clickhouse/init/init.sh` line 7 | `SELECT *` violates CLAUDE.md rule. Replace with explicit column. |
| 5 | Critical | `.claude/docs/clickhouse-setup.md` line 93 | Documentation describes wrong init mechanism (`/docker-entrypoint-initdb.d/` vs `clickhouse-init` container). Fix to match actual implementation. |
| 6 | Critical | `.claude/docs/clickhouse-setup.md` line 187 | `SELECT *` in a code example violates CLAUDE.md rule. Replace with explicit columns. |
| 7 | Warning | `clickhouse/init/init.sql` line 134 | `CREATE OR REPLACE FUNCTION` deviates from approved plan spec. Align with plan or amend plan. |
| 8 | Warning | `clickhouse/init/init.sh` lines 19-26 | Dead code: `FUNCTION_ALREADY_EXISTS` handler unreachable when using `CREATE OR REPLACE FUNCTION`. Remove. |
| 9 | Warning | `clickhouse/docker-compose.yml` lines 85-103 | `clickhouse-init` has no memory resource limit. |
| 10 | Warning | `.claude/docs/clickhouse-setup.md` line 38 | Inaccurate startup description. Fix to match actual init container approach. |
| 11 | Warning | `.claude/docs/clickhouse-setup.md` lines 210-225 | `init.sh` missing from file structure diagram. |
| 12 | Warning | `INFRA.md` line 38 | `factorial` described as "ClickHouse built-in" — it is a UDF. Correct the description. |

---

## Consolidated Summary (ClickHouse Stand)

**Files reviewed:** 12
**Critical:** 6
**Warning:** 7
**Suggestion:** 0
**Verdict: NOT_APPROVED**

The core SQL schema, XML configs, macro files, port assignments, named volumes, image pinning, and credential handling are all correct and compliant. The critical blockers are: (1) an unapproved 4th service added to docker-compose.yml that violates the definition of done, (2) a `SELECT *` in init.sh violating CLAUDE.md, (3) a security exposure on the `default` user in users.xml, and (4) two CLAUDE.md and factual violations in the documentation file. All 6 critical issues must be resolved before this can be approved.
