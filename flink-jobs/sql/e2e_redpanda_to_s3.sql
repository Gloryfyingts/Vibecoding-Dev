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
    'format' = 'json',
    'sink.rolling-policy.file-size' = '1MB',
    'sink.rolling-policy.rollover-interval' = '10s',
    'sink.rolling-policy.check-interval' = '5s'
);

INSERT INTO events_sink
SELECT
    event_id,
    user_id,
    event_type,
    payload,
    created_at
FROM events_source;
