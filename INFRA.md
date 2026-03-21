# INFRA.md — Docker Compose Stack Setup Report

## Date

2026-03-21

## Task Completed

Full local development environment implemented per `task_plan.md`. All 10 services defined in `docker-compose.yml` with healthchecks, named volumes, memory limits, and credentials from `.env`. Custom Dockerfiles for Flink and Jupyter created. Postgres init SQL, Flink SQL/PyFlink jobs, and Jupyter E2E test notebook created.

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
