#!/usr/bin/env bash
set -euo pipefail

DB_URL="${DATABASE_URL:-postgresql://pipeline:pipeline@localhost:5432/pipeline}"
WAIT_SECONDS="${WAIT_SECONDS:-60}"

echo "Waiting ${WAIT_SECONDS}s for data ingestion..."
sleep "$WAIT_SECONDS"

FAILED=0

check_table() {
    local table=$1
    local min_rows=${2:-1}
    local count
    count=$(psql "$DB_URL" -t -A -c "SELECT COUNT(*) FROM crypto.${table}")
    if [ "$count" -ge "$min_rows" ]; then
        echo "PASS: crypto.${table} has ${count} rows (>= ${min_rows})"
    else
        echo "FAIL: crypto.${table} has ${count} rows (expected >= ${min_rows})"
        FAILED=1
    fi
}

echo "Checking crypto tables..."

check_table "symbols" 3
check_table "agg_trades" 1
check_table "orderbook_snapshots" 1
check_table "orderbook_levels" 1
check_table "ticker_24hr" 1
check_table "ingest_state" 1

echo ""
echo "Checking data quality..."

BID_COUNT=$(psql "$DB_URL" -t -A -c "SELECT COUNT(*) FROM crypto.orderbook_levels WHERE side = 'bid'")
ASK_COUNT=$(psql "$DB_URL" -t -A -c "SELECT COUNT(*) FROM crypto.orderbook_levels WHERE side = 'ask'")

if [ "$BID_COUNT" -gt 0 ] && [ "$ASK_COUNT" -gt 0 ]; then
    echo "PASS: orderbook has both bid (${BID_COUNT}) and ask (${ASK_COUNT}) levels"
else
    echo "FAIL: orderbook missing bid (${BID_COUNT}) or ask (${ASK_COUNT}) levels"
    FAILED=1
fi

SYMBOL_COUNT=$(psql "$DB_URL" -t -A -c "SELECT COUNT(DISTINCT symbol) FROM crypto.ticker_24hr")
if [ "$SYMBOL_COUNT" -ge 3 ]; then
    echo "PASS: ticker has data for ${SYMBOL_COUNT} symbols"
else
    echo "FAIL: ticker has data for ${SYMBOL_COUNT} symbols (expected >= 3)"
    FAILED=1
fi

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "ALL CHECKS PASSED"
    exit 0
else
    echo "SOME CHECKS FAILED"
    exit 1
fi
