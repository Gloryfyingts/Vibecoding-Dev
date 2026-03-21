# Task: Local Development Environment — Docker Compose

Build a docker-compose stack for the local development environment and validate it with an end-to-end test pipeline.

---

## 1. Infrastructure — Containers & Configuration

### 1.1 Containers to create

| Service | Image (pin a stable version) | Purpose |
|---|---|---|
| **Postgres** | `postgres:16` | Application database + Iceberg JDBC catalog (separate schema) |
| **MinIO** | `minio/minio` | S3-compatible object storage for Iceberg data files |
| **Redpanda** | `redpandadata/redpanda` | Kafka-compatible message broker (local replacement for Kafka) |
| **Flink JobManager** | `flink` | Flink session cluster — job coordination |
| **Flink TaskManager** | `flink` | Flink session cluster — job execution |
| **Spark Master** | `bitnami/spark` | Spark standalone cluster — master |
| **Spark Worker** | `bitnami/spark` | Spark standalone cluster — worker |
| **Jupyter Notebook** | `jupyter/pyspark-notebook` | Single notebook with Python + PySpark kernels |

### 1.2 Networking & dependencies

All containers on a single Docker network. Define `depends_on` with healthchecks:

- Redpanda must be healthy before Flink starts
- MinIO must be healthy before Flink and Spark start
- Postgres must be healthy before Jupyter starts (for catalog + app data)
- Spark Master must be healthy before Spark Worker starts

### 1.3 Web UIs — port mapping

Expose every available management UI:

| UI | Port |
|---|---|
| Flink Dashboard | `8081` |
| Spark Master UI | `8080` |
| Spark Worker UI | `8082` |
| MinIO Console | `9001` |
| Redpanda Console | `8888` (or bundled rpk) |
| Jupyter Notebook | `8890` |
| Postgres | `5432` |

Resolve any port conflicts if defaults collide.

### 1.4 Credentials & secrets

- All credentials in `.env` file (never hardcoded in docker-compose)
- `.env` must be in `.gitignore`
- Provide a `.env.example` with placeholder values
- MinIO: root user + root password
- Postgres: user, password, database name

### 1.5 Storage & volumes

- Postgres: named volume for data persistence
- MinIO: named volume for data persistence
- Jupyter: bind-mount a local `./notebooks/` directory so notebooks survive container restarts
- Flink: bind-mount a local `./flink-jobs/` directory for job JARs and PyFlink scripts

### 1.6 MinIO bootstrap

On first start, automatically create:
- Bucket: `warehouse` (for Iceberg table data)
- Bucket: `raw` (for Flink JSON sink output)

Use an init container or entrypoint script with `mc` client.

### 1.7 Postgres bootstrap

On first start, run an init script that creates:
- Schema `app` — for application tables (test data)
- Schema `iceberg_catalog` — for Iceberg JDBC catalog metadata
- Grant appropriate permissions

### 1.8 Iceberg catalog configuration

Spark must be configured with an Iceberg JDBC catalog:
- Catalog name: `iceberg`
- JDBC connection pointing to Postgres, schema `iceberg_catalog`
- Warehouse location: `s3a://warehouse/`
- S3 endpoint pointing to MinIO
- Include all necessary Iceberg + S3 + JDBC JARs in Spark classpath

### 1.9 Resource limits

Set memory limits to keep the stack runnable on a 16GB machine:
- Flink JobManager: 1GB
- Flink TaskManager: 2GB
- Spark Master: 512MB
- Spark Worker: 2GB
- Postgres: 512MB
- MinIO: 512MB
- Redpanda: 1GB
- Jupyter: 1GB

---

## 2. E2E Test Pipeline

The test pipeline validates that all services work together. It is NOT production code — it will be deleted after validation. Do not document it in CLAUDE.md.

### 2.0 Test data schema

Use this minimal schema everywhere in the pipeline:

```sql
CREATE TABLE app.events (
    event_id    SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL,
    event_type  VARCHAR(50) NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

Insert 5-10 sample rows with varied `event_type` values (e.g., `click`, `purchase`, `login`).

### 2.1 Step 1 — Ingest test data into Postgres

- **What:** Python script in a Jupyter notebook
- **Action:** Connects to Postgres, creates `app.events` table (if not exists), inserts sample rows
- **Run by:** User manually via Jupyter UI
- **Validates:** Postgres is reachable from Jupyter, schema setup works

### 2.2 Step 2 — Postgres to Redpanda

- **What:** Python script in a Jupyter notebook
- **Action:** Reads all rows from `app.events`, serializes each row as JSON, produces messages to Redpanda topic `events-raw`
- **Libraries:** `psycopg2` + `kafka-python` (or `confluent-kafka`)
- **Run by:** User manually via Jupyter UI
- **Validates:** Postgres read works, Redpanda is reachable and accepting messages

### 2.3 Step 3 — Flink: Redpanda to S3 (JSON)

Provide **two variants** of this job (both should work, user can run either):

#### Variant A — Flink SQL

- Submit via Flink SQL Client or REST API
- DDL: define a Kafka source table and a filesystem sink table
- Source: Redpanda topic `events-raw`, JSON format
- Sink: `s3://raw/events/` as JSON files
- Include instructions for how to submit this to the session cluster

#### Variant B — PyFlink script

- Python script using PyFlink Table API
- Same source/sink as Variant A
- Submit to the session cluster via `flink run --python`

**Both variants validate:** Flink cluster is operational, can connect to Redpanda, can write to MinIO/S3

### 2.4 Step 4 — Spark: JSON to Iceberg

- **What:** PySpark script in a Jupyter notebook
- **Action:**
  1. Read JSON files from `s3a://raw/events/`
  2. Write as an Iceberg table: `iceberg.db.events`
  3. Run a verification query: `SELECT count(*), event_type FROM iceberg.db.events GROUP BY event_type`
- **Run by:** User manually via Jupyter UI
- **Validates:** Spark cluster works, S3/MinIO access works, Iceberg JDBC catalog writes metadata to Postgres, Iceberg table is queryable

---

## 3. Post-Task Actions

After the test pipeline runs successfully end-to-end:

1. **Update CLAUDE.md** — add a `## Local Environment` section describing:
   - How to start the stack (`docker-compose up -d`)
   - Container list with ports and purpose
   - How to access each Web UI
   - Environment variables reference
   - Iceberg catalog configuration summary
   - How to stop/reset the environment
2. **Do NOT document the test pipeline** in CLAUDE.md — it is ephemeral

---

## 4. Definition of Done

All of the following must be true:

- [ ] `docker-compose up -d` starts all containers without errors
- [ ] All healthchecks pass (all containers report healthy)
- [ ] All Web UIs are accessible at their mapped ports
- [ ] MinIO has `warehouse` and `raw` buckets created
- [ ] Postgres has `app` and `iceberg_catalog` schemas created
- [ ] **Test Step 1:** Sample data is in `app.events`
- [ ] **Test Step 2:** Messages appear in Redpanda topic `events-raw`
- [ ] **Test Step 3 (either variant):** JSON files appear in `s3://raw/events/`
- [ ] **Test Step 4:** `iceberg.db.events` table exists, verification query returns correct counts
- [ ] CLAUDE.md updated with local environment documentation
- [ ] `.env.example` exists with placeholder values
- [ ] No credentials are hardcoded anywhere
