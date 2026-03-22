#!/bin/bash

RETRY_MAX=30
RETRY_DELAY=5
attempt=0

until clickhouse-client --host clickhouse-shard1 --port 9000 --user "${CLICKHOUSE_USER}" --password "${CLICKHOUSE_PASSWORD}" --query "SELECT name FROM system.zookeeper WHERE path='/' LIMIT 1" > /dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $RETRY_MAX ]; then
        echo "Keeper coordination not available after $RETRY_MAX attempts"
        exit 1
    fi
    sleep $RETRY_DELAY
done

output=$(clickhouse-client --host clickhouse-shard1 --port 9000 --user "${CLICKHOUSE_USER}" --password "${CLICKHOUSE_PASSWORD}" --multiquery < /init.sql 2>&1)
rc=$?

if [ $rc -ne 0 ]; then
    if echo "$output" | grep -q "FUNCTION_ALREADY_EXISTS"; then
        echo "init.sql completed (function already exists, skipping)"
        exit 0
    fi
    echo "init.sql failed with exit code $rc:"
    echo "$output"
    exit $rc
fi

echo "init.sql completed"
