-- === Databases ===

CREATE DATABASE IF NOT EXISTS analytics ON CLUSTER cluster_2s1r;

CREATE DATABASE IF NOT EXISTS inventory ON CLUSTER cluster_2s1r;

-- === Tables (Local Engines) ===

CREATE TABLE IF NOT EXISTS analytics.daily_aggregates_local ON CLUSTER cluster_2s1r
(
    `dt` Date,
    `metric_name` LowCardinality(String),
    `dimension_key` String,
    `total_count` UInt64,
    `total_sum` Float64,
    `min_value` Float64,
    `max_value` Float64,
    `avg_value` Float64,
    `unique_users` UInt64,
    INDEX idx_dimension dimension_key TYPE bloom_filter(0.01) GRANULARITY 3
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/analytics/daily_aggregates_local', '{replica}')
PARTITION BY toYYYYMM(dt)
ORDER BY (dt, metric_name)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS analytics.session_facts_local ON CLUSTER cluster_2s1r
(
    `session_id` UUID,
    `user_id` UInt64,
    `started_at` DateTime64(3),
    `ended_at` DateTime64(3),
    `page_count` UInt16,
    `total_duration_seconds` UInt32,
    `is_bounce` UInt8,
    `entry_url` String,
    `exit_url` String,
    `device_type` LowCardinality(String),
    INDEX idx_duration total_duration_seconds TYPE minmax GRANULARITY 4
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/analytics/session_facts_local', '{replica}')
PARTITION BY toYYYYMM(started_at)
ORDER BY (user_id, started_at)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS analytics.user_events_local ON CLUSTER cluster_2s1r
(
    `event_id` UInt64,
    `user_id` UInt64,
    `event_type` LowCardinality(String),
    `event_timestamp` DateTime64(3),
    `page_url` String,
    `session_id` UUID,
    `duration_seconds` Float32,
    `is_mobile` UInt8,
    `country_code` FixedString(2),
    `payload` String,
    INDEX idx_event_type event_type TYPE set(100) GRANULARITY 4,
    INDEX idx_country country_code TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/analytics/user_events_local', '{replica}')
PARTITION BY toYYYYMM(event_timestamp)
ORDER BY (user_id, event_timestamp)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS inventory.products_local ON CLUSTER cluster_2s1r
(
    `product_id` UInt64,
    `sku` String,
    `product_name` String,
    `category` LowCardinality(String),
    `subcategory` LowCardinality(String),
    `price` Decimal(18, 2),
    `weight_kg` Float32,
    `created_at` DateTime,
    `updated_at` DateTime,
    `is_active` UInt8,
    INDEX idx_sku sku TYPE bloom_filter(0.01) GRANULARITY 3,
    INDEX idx_price price TYPE minmax GRANULARITY 4
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/inventory/products_local', '{replica}')
PARTITION BY is_active
ORDER BY (category, product_id)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS inventory.stock_movements_local ON CLUSTER cluster_2s1r
(
    `movement_id` UInt64,
    `product_id` UInt64,
    `warehouse_id` UInt16,
    `movement_type` LowCardinality(String),
    `quantity` Int32,
    `movement_timestamp` DateTime64(3),
    `batch_reference` String,
    `operator_id` UInt32,
    INDEX idx_warehouse warehouse_id TYPE set(50) GRANULARITY 4
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/inventory/stock_movements_local', '{replica}')
PARTITION BY toYYYYMM(movement_timestamp)
ORDER BY (product_id, movement_timestamp)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS inventory.warehouse_snapshots_local ON CLUSTER cluster_2s1r
(
    `snapshot_date` Date,
    `warehouse_id` UInt16,
    `product_id` UInt64,
    `quantity_on_hand` Int32,
    `quantity_reserved` Int32,
    `quantity_available` Int32,
    `last_restock_date` Nullable(Date),
    `reorder_point` UInt32,
    INDEX idx_qty_available quantity_available TYPE minmax GRANULARITY 4
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/inventory/warehouse_snapshots_local', '{replica}')
PARTITION BY toYYYYMM(snapshot_date)
ORDER BY (snapshot_date, warehouse_id, product_id)
SETTINGS index_granularity = 8192;

-- === Distributed Tables ===

CREATE TABLE IF NOT EXISTS analytics.daily_aggregates ON CLUSTER cluster_2s1r
(
    `dt` Date,
    `metric_name` LowCardinality(String),
    `dimension_key` String,
    `total_count` UInt64,
    `total_sum` Float64,
    `min_value` Float64,
    `max_value` Float64,
    `avg_value` Float64,
    `unique_users` UInt64
)
ENGINE = Distributed('cluster_2s1r', 'analytics', 'daily_aggregates_local', rand());

CREATE TABLE IF NOT EXISTS analytics.session_facts ON CLUSTER cluster_2s1r
(
    `session_id` UUID,
    `user_id` UInt64,
    `started_at` DateTime64(3),
    `ended_at` DateTime64(3),
    `page_count` UInt16,
    `total_duration_seconds` UInt32,
    `is_bounce` UInt8,
    `entry_url` String,
    `exit_url` String,
    `device_type` LowCardinality(String)
)
ENGINE = Distributed('cluster_2s1r', 'analytics', 'session_facts_local', rand());

CREATE TABLE IF NOT EXISTS analytics.user_events ON CLUSTER cluster_2s1r
(
    `event_id` UInt64,
    `user_id` UInt64,
    `event_type` LowCardinality(String),
    `event_timestamp` DateTime64(3),
    `page_url` String,
    `session_id` UUID,
    `duration_seconds` Float32,
    `is_mobile` UInt8,
    `country_code` FixedString(2),
    `payload` String
)
ENGINE = Distributed('cluster_2s1r', 'analytics', 'user_events_local', rand());

CREATE TABLE IF NOT EXISTS inventory.products ON CLUSTER cluster_2s1r
(
    `product_id` UInt64,
    `sku` String,
    `product_name` String,
    `category` LowCardinality(String),
    `subcategory` LowCardinality(String),
    `price` Decimal(18, 2),
    `weight_kg` Float32,
    `created_at` DateTime,
    `updated_at` DateTime,
    `is_active` UInt8
)
ENGINE = Distributed('cluster_2s1r', 'inventory', 'products_local', rand());

CREATE TABLE IF NOT EXISTS inventory.stock_movements ON CLUSTER cluster_2s1r
(
    `movement_id` UInt64,
    `product_id` UInt64,
    `warehouse_id` UInt16,
    `movement_type` LowCardinality(String),
    `quantity` Int32,
    `movement_timestamp` DateTime64(3),
    `batch_reference` String,
    `operator_id` UInt32
)
ENGINE = Distributed('cluster_2s1r', 'inventory', 'stock_movements_local', rand());

CREATE TABLE IF NOT EXISTS inventory.warehouse_snapshots ON CLUSTER cluster_2s1r
(
    `snapshot_date` Date,
    `warehouse_id` UInt16,
    `product_id` UInt64,
    `quantity_on_hand` Int32,
    `quantity_reserved` Int32,
    `quantity_available` Int32,
    `last_restock_date` Nullable(Date),
    `reorder_point` UInt32
)
ENGINE = Distributed('cluster_2s1r', 'inventory', 'warehouse_snapshots_local', rand());
