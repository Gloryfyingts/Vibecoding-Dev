from pyflink.datastream import StreamExecutionEnvironment
from pyflink.table import StreamTableEnvironment

env = StreamExecutionEnvironment.get_execution_environment()
env.enable_checkpointing(10000)
t_env = StreamTableEnvironment.create(env)

t_env.execute_sql("""
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
)
""")

t_env.execute_sql("""
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
)
""")

t_env.execute_sql("""
INSERT INTO events_sink
SELECT
    event_id,
    user_id,
    event_type,
    payload,
    created_at
FROM events_source
""")
