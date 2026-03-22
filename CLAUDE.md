# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Data engineering project building pipelines with Apache Flink, Apache Iceberg, S3 storage, and open API data ingestion. Rust/Go may be introduced later. The repo includes AI agent definitions for Claude Code to orchestrate development.

## Two-Repo Workflow

This project uses two repositories:
- **Vibecoding-Original** (`../Vibecoding-Original`) -- clean showcase repo, no AI artifacts. Remote: `https://github.com/Gloryfyingts/Vibecoding-Original.git`. Branches: `main`, `stage`. Claude must NEVER run git commands in this repo.
- **Vibecoding-Dev** (this repo) -- development repo with AI artifacts. Branches: `AI/Master`, `AI/Stage`, `Original/Master`, `Original/Stage`.

Branch mapping:
- `Original/Master` = mirror of Vibecoding-Original `main` (no AI artifacts)
- `Original/Stage` = mirror of Vibecoding-Original `stage` (no AI artifacts)
- `AI/Master` = development branch for main features (has AI artifacts)
- `AI/Stage` = development branch for stage features (has AI artifacts)

Git pipeline:
1. User runs `git pull` in Vibecoding-Original
2. `/pull <branch>` copies files into `Original/<branch>`, then merges into `AI/<branch>`
3. Development happens on `AI/<branch>`
4. `/push <branch>` merges `AI/<branch>` into `Original/<branch>`, then copies to Vibecoding-Original
5. User manually commits and pushes in Vibecoding-Original

AI artifacts exclusion list (never copied to Original): `.claude/`, `CLAUDE.md`, `task_plan.md`, `errors.md`, `INFRA.md`, `prompt.md`, `REPORT.MD`

## Agent Workflow

This repo uses four custom Claude Code agents (`.claude/agents/`) that enforce a strict development loop:

1. **planner** (opus) — Must be invoked FIRST for any task. Produces `task_plan.md` with scope, execution order, risks, and a strict definition of done. Waits for user approval before any code is written.
2. **de-coder** — Writes SQL, ETL scripts, Spark jobs, Airflow DAGs. Reads `task_plan.md` before starting. Reports to `.claude/docs/REPORT.MD`.
3. **local-repo-devops** (sonnet) — Docker, docker-compose, databases, Airflow setup. Reports to `INFRA.md` in project root.
4. **reviewer** (sonnet) — Read-only review after every code change. Checks against `task_plan.md` definition of done. Outputs `errors.md` if issues found.

The enforced sequence is: **plan → code → review**. Never skip planning. Never skip review.

## Key Rules

- **No comments in code** — documentation goes to `.claude/docs/`
- **Never fabricate DDL** — always read actual CREATE TABLE statements before writing SQL. If DDL is missing, stop and ask.
- **No `SELECT *`** — use explicit column lists
- **CTEs over subqueries**
- **Partition filters required** on every query to partitioned tables
- **COALESCE/IFNULL** for nullable columns in calculations
- **snake_case** for all aliases and column names
- **Docker images must be pinned** — never use `latest` tag
- **Credentials in `.env` only** — never hardcode in docker-compose or code; never commit `.env`
- **Never run `docker-compose down -v`** without explicit user approval
- **`task_plan.md` must have a definition of done** — agents will refuse to work without it
- **Mandatory E2E testing** — after developing ANY script or making changes to ANY DAG, it is MANDATORY to run a full end-to-end test pipeline for all affected scripts. No code is considered done until it has been executed successfully. This applies to Claude Code itself and to all agents except planner.

## Local Environment

### Starting and stopping

- **Start the stack:** `docker compose up -d` (first run downloads images (~5-8 GB) and builds custom Dockerfiles for Flink and Jupyter)
- **Stop the stack:** `docker compose down` (preserves named data volumes)
- **Reset the stack (destroys all data):** `docker compose down -v` then `docker compose up -d` — requires explicit user approval per project rules

### Containers

| Service | Container Name | Host Port(s) | Purpose |
|---|---|---|---|
| Postgres | postgres | 5432 | App database + Iceberg JDBC catalog |
| MinIO | minio | 9000 (API), 9001 (Console) | S3-compatible object storage |
| MinIO Init | minio-init | — | One-shot bucket creation (exits after run) |
| Redpanda | redpanda | 19092 (Kafka), 18082 (HTTP Proxy), 18081 (Schema Registry), 19644 (Admin) | Kafka-compatible message broker |
| Redpanda Console | redpanda-console | 8888 | Redpanda Web UI |
| Flink JobManager | flink-jobmanager | 8081 | Flink session cluster coordinator |
| Flink TaskManager | flink-taskmanager | — | Flink session cluster worker (internal only) |
| Spark Master | spark-master | 8080 (UI), 7077 (RPC) | Spark standalone cluster master |
| Spark Worker | spark-worker | 8082 | Spark standalone cluster worker |
| Jupyter | jupyter | 8890 | Notebook environment (PySpark + Python) |

### Web UIs

| Service | URL |
|---|---|
| Flink Dashboard | http://localhost:8081 |
| Spark Master UI | http://localhost:8080 |
| Spark Worker UI | http://localhost:8082 |
| MinIO Console | http://localhost:9001 |
| Redpanda Console | http://localhost:8888 |
| Jupyter Notebook | http://localhost:8890 |

### Environment variables

See `.env.example` for the full template. Copy to `.env` and fill in values before starting.

| Variable | Description |
|---|---|
| `POSTGRES_USER` | Postgres superuser name used by all services |
| `POSTGRES_PASSWORD` | Postgres superuser password |
| `POSTGRES_DB` | Default Postgres database name |
| `MINIO_ROOT_USER` | MinIO root access key (also used as AWS_ACCESS_KEY_ID in Jupyter and Flink) |
| `MINIO_ROOT_PASSWORD` | MinIO root secret key (also used as AWS_SECRET_ACCESS_KEY in Jupyter and Flink) |
| `JUPYTER_TOKEN` | Token required to access Jupyter UI; leave empty to disable auth |

### Iceberg catalog configuration

The Iceberg catalog is named `iceberg`, uses JDBC type backed by Postgres schema `iceberg_catalog`, and stores data files in MinIO at `s3a://warehouse/`. When creating a SparkSession that uses Iceberg, include these config properties:

```
spark.sql.catalog.iceberg = org.apache.iceberg.spark.SparkCatalog
spark.sql.catalog.iceberg.type = jdbc
spark.sql.catalog.iceberg.uri = jdbc:postgresql://postgres:5432/pipeline
spark.sql.catalog.iceberg.jdbc.schema-version = V1
spark.sql.catalog.iceberg.warehouse = s3a://warehouse/
spark.sql.catalog.iceberg.io-impl = org.apache.iceberg.aws.s3.S3FileIO
spark.sql.catalog.iceberg.s3.endpoint = http://minio:9000
spark.sql.catalog.iceberg.s3.path-style-access = true
spark.hadoop.fs.s3a.endpoint = http://minio:9000
spark.hadoop.fs.s3a.path.style.access = true
spark.hadoop.fs.s3a.impl = org.apache.hadoop.fs.s3a.S3AFileSystem
```

Required JARs via `spark.jars.packages`: `org.apache.iceberg:iceberg-spark-runtime-3.5_2.12:1.7.1,org.apache.iceberg:iceberg-aws-bundle:1.7.1,org.postgresql:postgresql:42.7.4`

### Flink jobs

Place SQL scripts in `flink-jobs/sql/` and PyFlink scripts in `flink-jobs/python/`. Both directories are bind-mounted into all Flink containers at `/opt/flink/jobs/`.

- Run a SQL job: `docker exec -it flink-jobmanager ./bin/sql-client.sh` then paste the SQL file contents
- Run a PyFlink job: `docker exec -it flink-jobmanager flink run --python /opt/flink/jobs/python/<script>.py`

### Jupyter notebooks

Place notebooks in `notebooks/`. The directory is bind-mounted into the Jupyter container at `/home/jovyan/work/`. Access the UI at http://localhost:8890 with the token set in `JUPYTER_TOKEN`.
