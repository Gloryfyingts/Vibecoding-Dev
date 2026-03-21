# Task Plan: Local Development Environment -- Docker Compose

## Task Summary

Build a docker-compose stack with Postgres, MinIO, Redpanda, Flink, Spark, and Jupyter. Validate with an end-to-end test pipeline (Postgres -> Redpanda -> Flink -> S3/MinIO -> Spark/Iceberg). Update CLAUDE.md with environment documentation.

---

## Scope

### Files to CREATE

| # | File | Description |
|---|------|-------------|
| 1 | `docker-compose.yml` | Main compose file defining all services (Postgres, MinIO, minio-init, Redpanda, Redpanda Console, Flink JobManager, Flink TaskManager, Spark Master, Spark Worker, Jupyter) |
| 2 | `.env.example` | Template with placeholder credentials |
| 3 | `.env` | Actual credentials file (will NOT be committed) |
| 4 | `.gitignore` | Must include `.env` |
| 5 | `docker/postgres/init.sql` | Bootstrap SQL: create schemas `app` and `iceberg_catalog`, grant permissions |
| 6 | `docker/jupyter/Dockerfile` | Extends Jupyter image with psycopg2-binary and kafka-python-ng |
| 7 | `docker/flink/Dockerfile` | Extends Flink image: enables S3 plugin, downloads Kafka connector JAR |
| 8 | `notebooks/test_e2e.ipynb` | Jupyter notebook with 4 test steps |
| 9 | `flink-jobs/sql/e2e_redpanda_to_s3.sql` | Flink SQL DDL+DML: Kafka source table, filesystem sink table, INSERT INTO |
| 10 | `flink-jobs/python/e2e_redpanda_to_s3.py` | PyFlink script: same logic as the SQL variant using Table API |

### Files to MODIFY

| # | File | Change |
|---|------|--------|
| 1 | `CLAUDE.md` | Append `## Local Environment` section after `## Key Rules` |

### Directories to CREATE (for bind mounts and organization)

| # | Directory | Purpose |
|---|-----------|---------|
| 1 | `notebooks/` | Jupyter notebook persistence (bind-mount) |
| 2 | `flink-jobs/sql/` | Flink SQL scripts |
| 3 | `flink-jobs/python/` | PyFlink scripts |
| 4 | `docker/postgres/` | Postgres init scripts |
| 5 | `docker/jupyter/` | Jupyter custom Dockerfile |
| 6 | `docker/flink/` | Flink custom Dockerfile |

---

## Detailed Specifications

### 1. docker-compose.yml -- Service Definitions

#### 1.1 Network

Single custom bridge network named `data-pipeline`.

#### 1.2 Postgres

- **Image:** `postgres:16.6`
- **Container name:** `postgres`
- **Ports:** `5432:5432`
- **Environment (from .env):** `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- **Volumes:**
  - Named volume `postgres-data` mounted at `/var/lib/postgresql/data`
  - Bind mount `./docker/postgres/init.sql` to `/docker-entrypoint-initdb.d/init.sql:ro` (auto-executed on first start)
- **Healthcheck:** `pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}` interval 5s, timeout 5s, retries 5
- **Memory limit:** 512MB (`deploy.resources.limits.memory: 512m`)

#### 1.3 MinIO

- **Image:** `minio/minio:RELEASE.2024-11-07T00-52-20Z`
- **Container name:** `minio`
- **Command:** `server /data --console-address ":9001"`
- **Ports:** `9000:9000` (API), `9001:9001` (Console UI)
- **Environment (from .env):** `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`
- **Volumes:** Named volume `minio-data` at `/data`
- **Healthcheck:** `curl -f http://localhost:9000/minio/health/live` interval 5s, timeout 5s, retries 5
- **Memory limit:** 512MB

#### 1.4 MinIO Init (bootstrap buckets)

- **Image:** `minio/mc:RELEASE.2024-11-17T19-35-25Z`
- **Container name:** `minio-init`
- **depends_on:** minio (healthy)
- **Entrypoint:** inline shell script that:
  1. Configures mc alias: `mc alias set local http://minio:9000 ${MINIO_ROOT_USER} ${MINIO_ROOT_PASSWORD}`
  2. Creates buckets: `mc mb --ignore-existing local/warehouse` and `mc mb --ignore-existing local/raw`
- **restart:** `"no"` (run-once init container)
- **No memory limit needed** (exits immediately after creating buckets)

#### 1.5 Redpanda

- **Image:** `redpandadata/redpanda:v24.2.18`
- **Container name:** `redpanda`
- **Command:** Multi-line command with the following flags:
  - `redpanda start`
  - `--mode dev-container`
  - `--smp 1`
  - `--memory 1G`
  - `--overprovisioned`
  - `--kafka-addr internal://0.0.0.0:9092,external://0.0.0.0:19092`
  - `--advertise-kafka-addr internal://redpanda:9092,external://localhost:19092`
  - `--pandaproxy-addr internal://0.0.0.0:8082,external://0.0.0.0:18082`
  - `--advertise-pandaproxy-addr internal://redpanda:8082,external://localhost:18082`
  - `--schema-registry-addr internal://0.0.0.0:8081,external://0.0.0.0:18081`
  - `--advertise-rpc-addr redpanda:33145`
- **Ports:**
  - `19092:19092` (Kafka API external)
  - `18082:18082` (HTTP Proxy external)
  - `18081:18081` (Schema Registry external)
  - `19644:9644` (Admin API)
- **Healthcheck:** `rpk cluster health --api-urls=localhost:9644 | grep -q 'Healthy:.*true'` interval 5s, timeout 5s, retries 10, start_period 15s
- **Memory limit:** 1GB
- **Port conflict resolution:** Redpanda's internal schema registry uses port 8081 and HTTP proxy uses 8082. These are the same ports as Flink Dashboard (8081) and Spark Worker UI (8082). The solution is dual listeners: internal ports (9092, 8081, 8082) are accessible only within the Docker network, while external ports (19092, 18081, 18082) are mapped to the host. Flink Dashboard keeps host port 8081, Spark Worker keeps host port 8082. No conflict.

#### 1.6 Redpanda Console

- **Image:** `redpandadata/console:v2.7.2`
- **Container name:** `redpanda-console`
- **Ports:** `8888:8080`
- **Environment:**
  - `KAFKA_BROKERS=redpanda:9092`
  - `REDPANDA_ADMINAPI_URLS=http://redpanda:9644`
  - `SCHEMA_REGISTRY_URLS=http://redpanda:8081`
- **depends_on:** redpanda (healthy)
- **Memory limit:** 256MB

#### 1.7 Flink JobManager

- **Image:** Built from `docker/flink/Dockerfile` (see section 1.13)
- **Container name:** `flink-jobmanager`
- **Command:** `jobmanager`
- **Ports:** `8081:8081` (Flink Dashboard UI)
- **Environment:**
  - `FLINK_PROPERTIES` multiline string containing:
    ```
    jobmanager.rpc.address: flink-jobmanager
    jobmanager.memory.process.size: 1024m
    state.backend: hashmap
    execution.checkpointing.interval: 10000
    s3.endpoint: http://minio:9000
    s3.path-style-access: true
    s3.access-key: ${MINIO_ROOT_USER}
    s3.secret-key: ${MINIO_ROOT_PASSWORD}
    ```
- **Volumes:**
  - Bind mount `./flink-jobs:/opt/flink/jobs`
- **depends_on:** redpanda (healthy), minio (healthy)
- **Healthcheck:** `curl -f http://localhost:8081/overview` interval 10s, timeout 5s, retries 10, start_period 30s
- **Memory limit:** 1GB

#### 1.8 Flink TaskManager

- **Image:** Same as JobManager (built from `docker/flink/Dockerfile`)
- **Container name:** `flink-taskmanager`
- **Command:** `taskmanager`
- **No host ports exposed** (TaskManager does not have a user-facing web UI; it communicates with JobManager internally)
- **Environment:** Same `FLINK_PROPERTIES` as JobManager, plus:
  ```
  taskmanager.memory.process.size: 2048m
  taskmanager.numberOfTaskSlots: 4
  ```
- **Volumes:** Same bind mounts as JobManager
- **depends_on:** flink-jobmanager (healthy)
- **Memory limit:** 2GB

#### 1.9 Spark Master

- **Image:** `bitnami/spark:3.5.4`
- **Container name:** `spark-master`
- **Environment:**
  - `SPARK_MODE=master`
  - `SPARK_MASTER_HOST=spark-master`
  - `SPARK_MASTER_PORT=7077`
  - `SPARK_MASTER_WEBUI_PORT=8080`
- **Ports:** `8080:8080` (Spark Master UI), `7077:7077` (Spark Master RPC)
- **depends_on:** minio (healthy)
- **Healthcheck:** `curl -f http://localhost:8080` interval 10s, timeout 5s, retries 5, start_period 15s
- **Memory limit:** 512MB

#### 1.10 Spark Worker

- **Image:** `bitnami/spark:3.5.4`
- **Container name:** `spark-worker`
- **Environment:**
  - `SPARK_MODE=worker`
  - `SPARK_MASTER_URL=spark://spark-master:7077`
  - `SPARK_WORKER_MEMORY=2g`
  - `SPARK_WORKER_WEBUI_PORT=8082`
- **Ports:** `8082:8082` (Spark Worker UI)
- **depends_on:** spark-master (healthy)
- **Healthcheck:** `curl -f http://localhost:8082` interval 10s, timeout 5s, retries 5, start_period 15s
- **Memory limit:** 2GB

#### 1.11 Jupyter Notebook

- **Image:** Built from `docker/jupyter/Dockerfile` (see section 1.14)
- **Container name:** `jupyter`
- **Ports:** `8890:8888` (Jupyter UI)
- **Environment:**
  - `JUPYTER_ENABLE_LAB=yes`
  - `JUPYTER_TOKEN=${JUPYTER_TOKEN}`
  - `AWS_ACCESS_KEY_ID=${MINIO_ROOT_USER}`
  - `AWS_SECRET_ACCESS_KEY=${MINIO_ROOT_PASSWORD}`
  - `AWS_ENDPOINT_URL=http://minio:9000`
- **Volumes:**
  - Bind mount `./notebooks:/home/jovyan/work`
- **depends_on:** postgres (healthy), spark-master (healthy), minio (healthy)
- **Healthcheck:** `curl -f http://localhost:8888/api` interval 10s, timeout 5s, retries 5, start_period 20s
- **Memory limit:** 1GB
- **user:** `root` with env `NB_UID=1000` and `NB_GID=100` and `CHOWN_HOME=yes` (needed for bind mount permissions on some hosts, the entrypoint drops back to jovyan)

#### 1.12 Volumes (named)

- `postgres-data`
- `minio-data`

#### 1.13 docker/flink/Dockerfile

```dockerfile
FROM flink:1.20.1-scala_2.12-java11

RUN cp /opt/flink/opt/flink-s3-fs-hadoop-*.jar /opt/flink/plugins/s3-fs-hadoop/ 2>/dev/null || \
    (mkdir -p /opt/flink/plugins/s3-fs-hadoop && cp /opt/flink/opt/flink-s3-fs-hadoop-*.jar /opt/flink/plugins/s3-fs-hadoop/)

RUN wget -q -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-sql-connector-kafka/3.3.0-1.20/flink-sql-connector-kafka-3.3.0-1.20.jar
```

This Dockerfile:
1. Starts from the pinned Flink 1.20.1 image
2. Enables the S3 filesystem plugin by copying the JAR from `/opt/flink/opt/` to the plugins directory (Flink ships this JAR but does not enable it by default)
3. Downloads the Kafka SQL connector JAR for Redpanda connectivity

#### 1.14 docker/jupyter/Dockerfile

```dockerfile
FROM quay.io/jupyter/pyspark-notebook:spark-3.5.4

RUN pip install --no-cache-dir psycopg2-binary kafka-python-ng
```

This Dockerfile:
1. Starts from the Jupyter PySpark notebook image pinned to Spark 3.5.4
2. Installs psycopg2-binary (for Postgres connectivity in test steps 1-2) and kafka-python-ng (for Redpanda producer in test step 2; `kafka-python-ng` is the maintained fork of `kafka-python`)

### 2. .env.example

```
POSTGRES_USER=pipeline_user
POSTGRES_PASSWORD=CHANGE_ME
POSTGRES_DB=pipeline

MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=CHANGE_ME

JUPYTER_TOKEN=
```

### 3. .gitignore

Must include at minimum:
```
.env
```

### 4. docker/postgres/init.sql

The init script runs as the POSTGRES_USER (which is the superuser for the official postgres image). It must:

1. `CREATE SCHEMA IF NOT EXISTS app;`
2. `CREATE SCHEMA IF NOT EXISTS iceberg_catalog;`

No explicit GRANT is needed because POSTGRES_USER is the owner/superuser.

### 5. Iceberg Catalog Configuration (Spark side)

These Spark configuration properties must be set when creating a SparkSession in the Jupyter notebook (test Step 4). They are NOT set in docker-compose; they are embedded in the notebook code:

- `spark.jars.packages`: `org.apache.iceberg:iceberg-spark-runtime-3.5_2.12:1.7.1,org.apache.iceberg:iceberg-aws-bundle:1.7.1,org.postgresql:postgresql:42.7.4`
- `spark.sql.catalog.iceberg`: `org.apache.iceberg.spark.SparkCatalog`
- `spark.sql.catalog.iceberg.type`: `jdbc`
- `spark.sql.catalog.iceberg.uri`: `jdbc:postgresql://postgres:5432/pipeline`
- `spark.sql.catalog.iceberg.jdbc.user`: (read from env `POSTGRES_USER` or hardcoded in test notebook as `pipeline_user`)
- `spark.sql.catalog.iceberg.jdbc.password`: (read from env `POSTGRES_PASSWORD` or hardcoded in test notebook)
- `spark.sql.catalog.iceberg.jdbc.schema-version`: `V1`
- `spark.sql.catalog.iceberg.warehouse`: `s3a://warehouse/`
- `spark.sql.catalog.iceberg.io-impl`: `org.apache.iceberg.aws.s3.S3FileIO`
- `spark.sql.catalog.iceberg.s3.endpoint`: `http://minio:9000`
- `spark.sql.catalog.iceberg.s3.path-style-access`: `true`
- `spark.hadoop.fs.s3a.endpoint`: `http://minio:9000`
- `spark.hadoop.fs.s3a.access.key`: (from env)
- `spark.hadoop.fs.s3a.secret.key`: (from env)
- `spark.hadoop.fs.s3a.path.style.access`: `true`
- `spark.hadoop.fs.s3a.impl`: `org.apache.hadoop.fs.s3a.S3AFileSystem`

**Important note:** The test notebook will use PySpark in **local mode** (master = `local[*]`), not connecting to the Spark standalone cluster. This avoids JAR classpath complexity with the remote cluster. The Spark standalone cluster (master + worker) is available for future production-like workloads but is not used by the test pipeline. The `spark.jars.packages` config will handle downloading all required JARs at SparkSession creation time into the Jupyter container.

### 6. Test Pipeline Files

#### 6.1 notebooks/test_e2e.ipynb

A Jupyter notebook with the following cells (the notebook is ephemeral test code, will be deleted after validation):

**Cell 1 (code) -- Step 1: Ingest test data into Postgres**
- `import psycopg2, os`
- Connect to `host='postgres', port=5432, dbname=os.environ.get('POSTGRES_DB', 'pipeline'), user=os.environ.get('POSTGRES_USER', 'pipeline_user'), password=os.environ.get('POSTGRES_PASSWORD', 'pipeline_pass')`
- Note: The env vars `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` are NOT directly available inside the Jupyter container (only MINIO and JUPYTER env vars are set). The notebook will either hardcode the test values or read from a config. Since this is ephemeral test code, hardcoding is acceptable. The implementer should use the same values as in `.env`.
- Execute CREATE TABLE IF NOT EXISTS `app.events` with exact schema from the task spec
- Execute INSERT with 8 rows: varied event_type values (click, purchase, login, signup, logout, page_view, add_to_cart, checkout), varied user_ids, some with JSONB payload, some with NULL payload
- Execute `SELECT count(*) FROM app.events` and print result
- Close connection

**Cell 2 (code) -- Step 2: Postgres to Redpanda**
- `import psycopg2, json, datetime`
- `from kafka import KafkaProducer`
- Connect to Postgres, read all rows from `app.events` with explicit column list: `SELECT event_id, user_id, event_type, payload, created_at FROM app.events`
- Create KafkaProducer with `bootstrap_servers='redpanda:9092'`
- For each row, build a dict, handle `datetime` serialization with `str()` or `.isoformat()`, handle `None` payload with COALESCE equivalent in Python
- Produce each row as JSON bytes to topic `events-raw`
- Flush producer, print count of messages sent

**Cell 3 (markdown) -- Step 3: Flink Instructions**
- Explain two options to run the Flink job:
  - **Option A (Flink SQL):** Run `docker exec -it flink-jobmanager ./bin/sql-client.sh` and paste contents of `flink-jobs/sql/e2e_redpanda_to_s3.sql`
  - **Option B (PyFlink):** Run `docker exec -it flink-jobmanager flink run --python /opt/flink/jobs/python/e2e_redpanda_to_s3.py`
- Note that the job is streaming: it will keep running after consuming existing messages. Wait 15-20 seconds for a checkpoint, then check MinIO Console at `localhost:9001` for files in the `raw` bucket under `events/` path. Cancel the job via Flink Dashboard (localhost:8081) after verification.

**Cell 4 (code) -- Step 4: Spark JSON to Iceberg**
- Create SparkSession with `.master("local[*]")` and all Iceberg catalog config properties from section 5
- `spark.sql("CREATE NAMESPACE IF NOT EXISTS iceberg.db")`
- Read JSON: `df = spark.read.json("s3a://raw/events/")`
- Print schema and count for verification
- Write Iceberg: `df.writeTo("iceberg.db.events").createOrReplace()`
- Verify: `spark.sql("SELECT count(*) as cnt, event_type FROM iceberg.db.events GROUP BY event_type").show()`
- Stop SparkSession

#### 6.2 flink-jobs/sql/e2e_redpanda_to_s3.sql

```sql
SET 'execution.checkpointing.interval' = '10s';

CREATE TABLE events_source (
    event_id    INT,
    user_id     INT,
    event_type  STRING,
    payload     STRING,
    created_at  STRING
) WITH (
    'connector' = 'kafka',
    'topic' = 'events-raw',
    'properties.bootstrap.servers' = 'redpanda:9092',
    'properties.group.id' = 'flink-e2e-test',
    'scan.startup.mode' = 'earliest-offset',
    'format' = 'json'
);

CREATE TABLE events_sink (
    event_id    INT,
    user_id     INT,
    event_type  STRING,
    payload     STRING,
    created_at  STRING
) WITH (
    'connector' = 'filesystem',
    'path' = 's3://raw/events/',
    'format' = 'json'
);

INSERT INTO events_sink
SELECT
    event_id,
    user_id,
    event_type,
    payload,
    created_at
FROM events_source;
```

Notes:
- `payload` is STRING (not MAP or ROW) because Flink receives it as a serialized JSON string from the Kafka topic
- `created_at` is STRING because the Python producer serializes it with `.isoformat()`
- The `SET` checkpoint interval ensures files are flushed to S3 every 10 seconds
- `scan.startup.mode = earliest-offset` ensures Flink reads all existing messages in the topic

#### 6.3 flink-jobs/python/e2e_redpanda_to_s3.py

PyFlink script that:
1. Creates a `StreamExecutionEnvironment` and `StreamTableEnvironment`
2. Executes the same DDL statements as the SQL variant (CREATE TABLE source, CREATE TABLE sink)
3. Executes the INSERT INTO statement
4. The script submits the job and exits; the job runs on the Flink cluster

### 7. CLAUDE.md Update

Append a `## Local Environment` section after `## Key Rules`. Content:

- **Starting the stack:** `docker compose up -d` (first run downloads images, approximately 5-8GB, and builds custom Dockerfiles for Flink and Jupyter)
- **Stopping the stack:** `docker compose down` (preserves data volumes)
- **Resetting the stack:** `docker compose down -v` (destroys all data -- requires explicit approval per project rules) then `docker compose up -d`
- **Container list:** Table with columns: Service, Container Name, Host Port(s), Purpose
  - Postgres | postgres | 5432 | App database + Iceberg JDBC catalog
  - MinIO | minio | 9000 (API), 9001 (Console) | S3-compatible object storage
  - Redpanda | redpanda | 19092 (Kafka), 18082 (HTTP Proxy), 18081 (Schema Registry), 19644 (Admin) | Kafka-compatible message broker
  - Redpanda Console | redpanda-console | 8888 | Redpanda Web UI
  - Flink JobManager | flink-jobmanager | 8081 | Flink session cluster coordinator
  - Flink TaskManager | flink-taskmanager | (none) | Flink session cluster worker
  - Spark Master | spark-master | 8080 (UI), 7077 (RPC) | Spark standalone cluster master
  - Spark Worker | spark-worker | 8082 | Spark standalone cluster worker
  - Jupyter | jupyter | 8890 | Notebook environment (PySpark + Python)
- **Web UIs:** Table with URL for each
  - Flink Dashboard: http://localhost:8081
  - Spark Master: http://localhost:8080
  - Spark Worker: http://localhost:8082
  - MinIO Console: http://localhost:9001
  - Redpanda Console: http://localhost:8888
  - Jupyter Notebook: http://localhost:8890
- **Environment variables:** Reference `.env.example`; list each variable with description
- **Iceberg catalog configuration summary:** Catalog name `iceberg`, type JDBC, Postgres schema `iceberg_catalog`, warehouse `s3a://warehouse/`, required Spark config properties for creating SparkSession
- **Flink jobs:** Place SQL scripts in `flink-jobs/sql/` and Python scripts in `flink-jobs/python/`; both directories are bind-mounted into the Flink containers at `/opt/flink/jobs/`
- **Jupyter notebooks:** Place notebooks in `notebooks/`; directory is bind-mounted into the Jupyter container at `/home/jovyan/work/`

---

## Execution Order

### Phase 1: Foundation (no dependencies between steps, can be done in parallel)

1. **Create `.gitignore`** -- must exist before `.env` to prevent accidental commit
2. **Create `.env.example`** -- template for credentials
3. **Create `.env`** -- copy of .env.example with actual working values for local dev
4. **Create `docker/postgres/init.sql`** -- Postgres bootstrap SQL script

### Phase 2: Custom Dockerfiles (no dependencies on each other)

5. **Create `docker/flink/Dockerfile`** -- Flink image with S3 plugin enabled and Kafka connector JAR downloaded
6. **Create `docker/jupyter/Dockerfile`** -- Jupyter image with psycopg2-binary and kafka-python-ng installed

### Phase 3: Docker Compose (depends on Phases 1-2)

7. **Create `docker-compose.yml`** -- all service definitions referencing the custom Dockerfiles, .env variables, init scripts, volumes, networks, healthchecks, and memory limits

### Phase 4: Validate Infrastructure (depends on Phase 3)

8. **Run `docker compose up -d`** -- start all containers
9. **Verify all healthchecks pass** -- all containers report healthy
10. **Verify Web UIs** -- check that each UI loads at its mapped port
11. **Verify MinIO buckets** -- `warehouse` and `raw` buckets exist in MinIO Console
12. **Verify Postgres schemas** -- `app` and `iceberg_catalog` schemas exist

### Phase 5: Test Pipeline Files (can be created while Phase 4 validates)

13. **Create `flink-jobs/sql/e2e_redpanda_to_s3.sql`** -- Flink SQL test job
14. **Create `flink-jobs/python/e2e_redpanda_to_s3.py`** -- PyFlink test job
15. **Create `notebooks/test_e2e.ipynb`** -- Jupyter notebook with all 4 test steps

### Phase 6: E2E Test Execution (depends on Phases 4-5)

16. **Run test Step 1** -- open notebook in Jupyter, run Cell 1 to insert test data into Postgres
17. **Run test Step 2** -- run Cell 2 to produce messages to Redpanda
18. **Run test Step 3** -- execute Flink job (either SQL or PyFlink variant), wait for checkpoint, verify files in MinIO
19. **Run test Step 4** -- run Cell 4 to read JSON from S3, write Iceberg table, run verification query

### Phase 7: Documentation (after E2E validation succeeds)

20. **Update `CLAUDE.md`** -- add `## Local Environment` section with all details described in section 7 of this plan

---

## Risks

### R1: Port conflicts on the host machine
- **Risk:** Ports 5432, 8080, 8081, 8082, 9000, 9001, 8888, 8890, 19092, 7077 may already be in use by other services on the developer's machine.
- **Mitigation:** The implementer must check port availability before starting the stack. Document alternative port mappings in CLAUDE.md. If a conflict is found during validation, update the port mapping in docker-compose.yml.

### R2: Redpanda internal ports collide with Flink/Spark UI ports
- **Risk:** Redpanda's internal schema registry uses port 8081, same as Flink Dashboard. Redpanda's HTTP proxy uses 8082, same as Spark Worker UI.
- **Mitigation:** Already resolved in the plan: Redpanda uses dual listeners (internal + external). Internal ports stay within the Docker network. External ports use different numbers (18081, 18082). No host port conflicts.

### R3: Flink cannot connect to Redpanda or MinIO without connector JARs
- **Risk:** The base Flink image does not include the Kafka SQL connector JAR. Without it, the E2E test Step 3 will fail with ClassNotFoundException.
- **Mitigation:** The custom `docker/flink/Dockerfile` downloads `flink-sql-connector-kafka-3.3.0-1.20.jar` into `/opt/flink/lib/` and enables the S3 filesystem plugin. If the Maven download fails during build, the Docker build will fail with a clear error.

### R4: Spark/Iceberg JAR version mismatch
- **Risk:** Iceberg runtime JAR must match the Spark major version. Using wrong versions causes ClassNotFoundException or NoSuchMethodError at runtime.
- **Mitigation:** Use exact coordinates: `iceberg-spark-runtime-3.5_2.12:1.7.1` for Spark 3.5.x. The `spark.jars.packages` mechanism resolves transitive dependencies automatically. First SparkSession creation will be slow (~1-2 min) due to JAR download.

### R5: MinIO path-style access not configured
- **Risk:** Both Spark and Flink default to virtual-hosted-style S3 access. MinIO requires path-style access. Without this setting, all S3 operations fail with "bucket not found" or DNS resolution errors.
- **Mitigation:** Explicitly set `s3.path-style-access: true` in Flink's FLINK_PROPERTIES and `spark.hadoop.fs.s3a.path.style.access: true` in Spark config. Both are specified in the plan.

### R6: Flink filesystem sink never flushes files without checkpointing
- **Risk:** Flink's filesystem sink writes files only on checkpoint completion. If no checkpoint interval is configured, files stay in "in-progress" state indefinitely and will not appear as readable files in MinIO.
- **Mitigation:** Set `execution.checkpointing.interval: 10000` (10 seconds) in FLINK_PROPERTIES and also in the SQL job with `SET 'execution.checkpointing.interval' = '10s'`. After producing messages and waiting 15-20 seconds, files will be visible.

### R7: Memory pressure on 16GB machine
- **Risk:** Total allocated: 1GB (Flink JM) + 2GB (Flink TM) + 512MB (Spark Master) + 2GB (Spark Worker) + 512MB (Postgres) + 512MB (MinIO) + 1GB (Redpanda) + 1GB (Jupyter) + 256MB (Redpanda Console) = ~8.8GB. Host OS and Docker overhead need ~4-6GB.
- **Mitigation:** Memory limits are ceilings, not reservations. Actual usage under dev workload will be lower. If issues occur, reduce Spark Worker to 1.5GB and Flink TaskManager to 1.5GB. Do not run all test steps simultaneously.

### R8: PySpark in Jupyter vs Spark standalone cluster JAR classpath
- **Risk:** If the notebook SparkSession connects to the Spark standalone cluster (spark://spark-master:7077), the Iceberg JARs downloaded via `spark.jars.packages` may not propagate to the worker. This causes "class not found" errors on the Spark Worker.
- **Mitigation:** Use `local[*]` mode in the test notebook. PySpark runs entirely within the Jupyter container, downloads JARs there, and does not need the external Spark cluster. The standalone cluster is provisioned for future use with properly configured classpaths.

### R9: JSONB/datetime serialization in test Step 2
- **Risk:** The `payload` column is JSONB and `created_at` is TIMESTAMP. When read from Postgres via psycopg2, payload returns as a Python dict and created_at as a datetime object. `json.dumps()` will fail on datetime objects.
- **Mitigation:** Use a custom JSON serializer: `json.dumps(row, default=str)`. This converts datetime to string and handles None gracefully.

### R10: Docker Compose V2 required
- **Risk:** The `deploy.resources.limits.memory` syntax requires Docker Compose V2 (`docker compose` as a plugin). Docker Compose V1 (`docker-compose` standalone binary) ignores `deploy` blocks unless `--compatibility` flag is used.
- **Mitigation:** Document that Docker Compose V2 is required. Check with `docker compose version` during validation.

### R11: Flink streaming job does not terminate after consuming test data
- **Risk:** The Flink job is a streaming job. After consuming all existing messages from Redpanda, it keeps running waiting for new messages. Users unfamiliar with streaming may think the job is stuck.
- **Mitigation:** Document in the notebook (Cell 3 markdown) that the job is streaming, files appear after checkpoint, and the job should be manually cancelled via Flink Dashboard after verification.

### R12: S3 path format difference between Flink and Spark
- **Risk:** Flink uses `s3://` scheme (via flink-s3-fs-hadoop plugin) while Spark uses `s3a://` scheme (via hadoop-aws). If the Flink job writes to `s3://raw/events/` but Spark reads from `s3a://raw/events/`, the paths must resolve to the same MinIO location.
- **Mitigation:** Both `s3://` and `s3a://` are handled by the Hadoop S3A filesystem when properly configured. The Flink S3 hadoop plugin uses the same underlying library. The paths `s3://raw/events/` and `s3a://raw/events/` both resolve to MinIO bucket `raw`, prefix `events/`. This is a standard pattern that works out of the box with both configurations.

### R13: Windows bind mount permissions
- **Risk:** On Windows with Docker Desktop, bind-mounted directories may have permission issues. Jupyter runs as user `jovyan` (UID 1000) and may not be able to write to the mounted `./notebooks` directory.
- **Mitigation:** Set `user: root` with `NB_UID=1000`, `NB_GID=100`, `CHOWN_HOME=yes` environment variables in the Jupyter service. The official Jupyter image entrypoint handles permission fixing and drops back to the jovyan user.

### R14: Postgres init.sql does not use env var for username
- **Risk:** The init.sql file hardcodes schema creation but cannot reference the POSTGRES_USER environment variable because SQL files do not support environment variable substitution.
- **Mitigation:** This is acceptable because the init script runs AS the POSTGRES_USER (who is the superuser), so no GRANT is needed. The schemas are created by the superuser and are accessible to them. If a different user is needed later, a shell script (`.sh`) can be used instead of SQL, which does support env var substitution.

---

## Definition of Done

All of the following must be true:

### Infrastructure
- [ ] `docker-compose.yml` exists at project root and defines services: postgres, minio, minio-init, redpanda, redpanda-console, flink-jobmanager, flink-taskmanager, spark-master, spark-worker, jupyter
- [ ] `docker compose up -d` starts all containers without errors
- [ ] All healthchecks pass within 2 minutes (all containers report healthy)
- [ ] All containers are on a single Docker network `data-pipeline`
- [ ] Docker images are pinned to specific versions (no `latest` tags)

### Credentials and Configuration
- [ ] `.env.example` exists with placeholder values for: POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, MINIO_ROOT_USER, MINIO_ROOT_PASSWORD, JUPYTER_TOKEN
- [ ] `.env` exists and is listed in `.gitignore`
- [ ] `.gitignore` exists at project root
- [ ] No credentials are hardcoded in `docker-compose.yml`, Dockerfiles, or any other committed file

### Web UIs
- [ ] Flink Dashboard accessible at http://localhost:8081
- [ ] Spark Master UI accessible at http://localhost:8080
- [ ] Spark Worker UI accessible at http://localhost:8082
- [ ] MinIO Console accessible at http://localhost:9001
- [ ] Redpanda Console accessible at http://localhost:8888
- [ ] Jupyter Notebook accessible at http://localhost:8890

### Bootstrapping
- [ ] Postgres has schemas `app` and `iceberg_catalog` created by `docker/postgres/init.sql`
- [ ] MinIO has buckets `warehouse` and `raw` created by `minio-init` container

### Resource Limits
- [ ] Flink JobManager: 1GB memory limit
- [ ] Flink TaskManager: 2GB memory limit
- [ ] Spark Master: 512MB memory limit
- [ ] Spark Worker: 2GB memory limit
- [ ] Postgres: 512MB memory limit
- [ ] MinIO: 512MB memory limit
- [ ] Redpanda: 1GB memory limit
- [ ] Jupyter: 1GB memory limit

### Volumes and Bind Mounts
- [ ] Named volume `postgres-data` persists Postgres data
- [ ] Named volume `minio-data` persists MinIO data
- [ ] Bind mount `./notebooks` mapped into Jupyter container
- [ ] Bind mount `./flink-jobs` mapped into Flink containers

### Custom Dockerfiles
- [ ] `docker/flink/Dockerfile` exists, builds successfully, includes S3 plugin and Kafka connector JAR
- [ ] `docker/jupyter/Dockerfile` exists, builds successfully, includes psycopg2-binary and kafka-python-ng

### E2E Test Pipeline
- [ ] `notebooks/test_e2e.ipynb` exists with all 4 test steps
- [ ] `flink-jobs/sql/e2e_redpanda_to_s3.sql` exists with valid Flink SQL (CREATE TABLE source, CREATE TABLE sink, INSERT INTO)
- [ ] `flink-jobs/python/e2e_redpanda_to_s3.py` exists with valid PyFlink script
- [ ] **Test Step 1:** Notebook Cell 1 creates `app.events` table and inserts 8 sample rows; SELECT count(*) returns 8
- [ ] **Test Step 2:** Notebook Cell 2 reads from Postgres and produces 8 messages to Redpanda topic `events-raw`; messages visible in Redpanda Console (http://localhost:8888)
- [ ] **Test Step 3 (either variant):** Flink job reads from `events-raw` topic and writes JSON files to MinIO; files visible in MinIO Console (http://localhost:9001) under bucket `raw`, path `events/`
- [ ] **Test Step 4:** Notebook Cell 4 reads JSON from `s3a://raw/events/`, writes Iceberg table `iceberg.db.events`, verification query `SELECT count(*) as cnt, event_type FROM iceberg.db.events GROUP BY event_type` returns correct grouped counts matching the inserted test data

### Documentation
- [ ] `CLAUDE.md` has a new `## Local Environment` section containing: startup command, container table with ports, Web UI URLs, environment variables reference, Iceberg catalog configuration summary, stop/reset instructions
- [ ] Existing content in `CLAUDE.md` (Project Overview, Two-Repo Workflow, Agent Workflow, Key Rules) is preserved unchanged
- [ ] The test pipeline is NOT documented in CLAUDE.md
