#!/usr/bin/env python3
"""Check stage Greenplum DDL for changes and regenerate init.sql if needed.

Connects to stage Greenplum, extracts the current DDL, compares it with
the saved reference, and regenerates init.sql if anything changed.

Requires STAGE_GP_PASSWORD or GP_PASS in .env (skips gracefully if missing).
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

# Project root relative to this script
ROOT = Path(__file__).resolve().parent.parent

# Ensure project root is on sys.path so we can import from scripts/
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def load_env():
    """Load .env file from project root."""
    env_path = ROOT / ".env"
    if not env_path.exists():
        return
    for line in env_path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            key, _, value = line.partition("=")
            key = key.strip()
            value = value.strip()
            if key and key not in os.environ:
                os.environ[key] = value


def extract_ddl_from_stage():
    """Connect to stage GP and extract DDL (Parts 1-4, 6 from extract script)."""
    # Import here so the script can skip gracefully if psycopg2 is not installed
    try:
        import psycopg2
        import psycopg2.extras
    except ImportError:
        print("WARNING: psycopg2 not installed. Install with: pip install psycopg2-binary")
        return None

    host = os.environ.get("STAGE_GP_HOST") or os.environ.get("HOST", "<YOUR_GP_HOST>")
    port = int(os.environ.get("STAGE_GP_PORT") or os.environ.get("PORT", "5432"))
    user = os.environ.get("GP_USER") or os.environ.get("GP_LOGIN", "<YOUR_GP_USER>")
    password = os.environ.get("STAGE_GP_PASSWORD") or os.environ.get("GP_PASS", "")
    database = os.environ.get("GP_DATABASE") or os.environ.get("DB", "<YOUR_DB_NAME>").lower()

    if not password:
        return None

    print(f"Connecting to stage GP at {host}:{port}...")
    conn = psycopg2.connect(
        host=host,
        port=port,
        user=user,
        password=password,
        database=database,
        options="-c default_transaction_read_only=on",
        connect_timeout=10,
    )

    ddl_parts = []

    try:
        with conn.cursor() as cur:
            cur.execute("SET statement_timeout = 300000")

            # Part 1: Schemas
            print("  Extracting schemas...")
            cur.execute("""
                SELECT 'CREATE SCHEMA IF NOT EXISTS ' || nspname || ';' AS ddl
                FROM pg_namespace
                WHERE nspname IN ('core', 'staging', 'kafka_fs', 'kafka_do', 'kafka_st', 'meta')
                ORDER BY nspname
            """)
            for row in cur.fetchall():
                ddl_parts.append(row[0])
            ddl_parts.append("")

            # Part 2: Tables
            print("  Extracting tables...")
            cur.execute("""
                WITH table_list AS (
                    SELECT n.nspname AS schemaname, c.relname AS tablename, c.oid AS table_oid
                    FROM pg_class c
                    JOIN pg_namespace n ON n.oid = c.relnamespace
                    WHERE n.nspname IN ('core', 'staging', 'kafka_fs', 'kafka_do', 'kafka_st', 'meta')
                      AND c.relkind = 'r'
                      AND c.relname NOT LIKE '%_1_prt_%'
                    ORDER BY n.nspname, c.relname
                ),
                columns_agg AS (
                    SELECT t.schemaname, t.tablename, t.table_oid,
                        string_agg(
                            '    ' || a.attname || ' ' || pg_catalog.format_type(a.atttypid, a.atttypmod)
                            || CASE WHEN a.attnotnull THEN ' NOT NULL' ELSE '' END
                            || CASE WHEN ad.adbin IS NOT NULL THEN ' DEFAULT ' || pg_get_expr(ad.adbin, ad.adrelid) ELSE '' END,
                            E',\\n' ORDER BY a.attnum
                        ) AS column_defs
                    FROM table_list t
                    JOIN pg_attribute a ON a.attrelid = t.table_oid
                    LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
                    WHERE a.attnum > 0 AND NOT a.attisdropped
                    GROUP BY t.schemaname, t.tablename, t.table_oid
                ),
                dist_info AS (
                    SELECT dp.localoid AS table_oid,
                        CASE dp.policytype
                            WHEN 'p' THEN 'DISTRIBUTED BY (' ||
                                (SELECT string_agg(a.attname, ', ' ORDER BY unnest_ord)
                                 FROM unnest(dp.distkey) WITH ORDINALITY AS u(attnum, unnest_ord)
                                 JOIN pg_attribute a ON a.attrelid = dp.localoid AND a.attnum = u.attnum) || ')'
                            WHEN 'r' THEN 'DISTRIBUTED REPLICATED'
                            ELSE 'DISTRIBUTED RANDOMLY'
                        END AS dist_clause
                    FROM gp_distribution_policy dp
                    WHERE dp.localoid IN (SELECT table_oid FROM table_list)
                ),
                storage_info AS (
                    SELECT c.oid AS table_oid, array_to_string(reloptions, ', ') AS storage_opts
                    FROM pg_class c
                    WHERE c.reloptions IS NOT NULL
                      AND c.oid IN (SELECT table_oid FROM table_list)
                ),
                check_constraints AS (
                    SELECT con.conrelid AS table_oid,
                        string_agg('    CONSTRAINT ' || con.conname || ' ' || pg_get_constraintdef(con.oid), E',\\n') AS check_defs
                    FROM pg_constraint con
                    WHERE con.contype = 'c'
                      AND con.conrelid IN (SELECT table_oid FROM table_list)
                    GROUP BY con.conrelid
                )
                SELECT
                    E'-- Table: ' || c.schemaname || '.' || c.tablename || E'\\n'
                    || 'CREATE TABLE IF NOT EXISTS ' || c.schemaname || '.' || c.tablename || E' (\\n'
                    || c.column_defs
                    || COALESCE(E',\\n' || ck.check_defs, '')
                    || E'\\n)'
                    || COALESCE(E'\\nWITH (' || s.storage_opts || ')', '')
                    || COALESCE(E'\\n' || d.dist_clause, '')
                    || E';\\n'
                FROM columns_agg c
                LEFT JOIN dist_info d ON d.table_oid = c.table_oid
                LEFT JOIN storage_info s ON s.table_oid = c.table_oid
                LEFT JOIN check_constraints ck ON ck.table_oid = c.table_oid
                ORDER BY c.schemaname, c.tablename
            """)
            for row in cur.fetchall():
                ddl_parts.append(row[0])
            ddl_parts.append("")

            # Part 3: Functions
            print("  Extracting functions...")
            cur.execute("""
                SELECT
                    'CREATE OR REPLACE FUNCTION ' || n.nspname || '.' || p.proname
                    || '(' || pg_get_function_arguments(p.oid) || ')'
                    || E'\\nRETURNS ' || pg_get_function_result(p.oid)
                    || E'\\nLANGUAGE ' || l.lanname
                    || CASE WHEN p.provolatile = 'v' THEN E'\\nVOLATILE'
                            WHEN p.provolatile = 's' THEN E'\\nSTABLE'
                            WHEN p.provolatile = 'i' THEN E'\\nIMMUTABLE'
                            ELSE '' END
                    || E'\\nAS $$\\n' || p.prosrc || E'\\n$$;\\n'
                FROM pg_proc p
                JOIN pg_namespace n ON n.oid = p.pronamespace
                JOIN pg_language l ON l.oid = p.prolang
                WHERE n.nspname IN ('core', 'staging', 'kafka_fs', 'kafka_do', 'kafka_st', 'meta')
                  AND l.lanname IN ('plpgsql', 'sql')
                ORDER BY n.nspname, p.proname
            """)
            for row in cur.fetchall():
                ddl_parts.append(row[0])
            ddl_parts.append("")

            # Part 4: Indexes
            print("  Extracting indexes...")
            cur.execute("""
                SELECT pg_get_indexdef(i.indexrelid) || ';' AS ddl,
                       ic.relname AS index_name
                FROM pg_index i
                JOIN pg_class c ON c.oid = i.indrelid
                JOIN pg_class ic ON ic.oid = i.indexrelid
                JOIN pg_namespace n ON n.oid = c.relnamespace
                WHERE n.nspname IN ('core', 'staging', 'kafka_fs', 'kafka_do', 'kafka_st', 'meta')
                  AND NOT i.indisprimary
                ORDER BY n.nspname, c.relname
            """)
            extracted_index_names = set()
            for row in cur.fetchall():
                ddl_parts.append(row[0])
                extracted_index_names.add(row[1])
            ddl_parts.append("")

            # Part 6: Unique constraints & foreign keys
            # Skip unique constraints whose name matches an already-extracted index
            # (unique constraints are backed by unique indexes with the same name)
            print("  Extracting constraints...")
            cur.execute("""
                SELECT
                    'ALTER TABLE ' || n.nspname || '.' || c.relname
                    || ' ADD CONSTRAINT ' || con.conname || ' '
                    || pg_get_constraintdef(con.oid) || ';',
                    con.conname,
                    con.contype
                FROM pg_constraint con
                JOIN pg_class c ON c.oid = con.conrelid
                JOIN pg_namespace n ON n.oid = c.relnamespace
                WHERE n.nspname IN ('core', 'staging', 'kafka_fs', 'kafka_do', 'kafka_st', 'meta')
                  AND con.contype IN ('u', 'f')
                ORDER BY n.nspname, c.relname
            """)
            for row in cur.fetchall():
                constraint_name = row[1]
                constraint_type = row[2]
                # Skip unique constraints already covered by a CREATE UNIQUE INDEX
                if constraint_type == 'u' and constraint_name in extracted_index_names:
                    continue
                ddl_parts.append(row[0])

    finally:
        conn.close()

    return "\n".join(ddl_parts)


def main() -> None:
    load_env()

    password = os.environ.get("STAGE_GP_PASSWORD") or os.environ.get("GP_PASS", "")
    if not password:
        print("WARNING: STAGE_GP_PASSWORD (or GP_PASS) not set in .env — skipping DDL check.")
        print("  Set it in .env to enable automatic DDL comparison.")
        sys.exit(0)

    ref_dir = ROOT / "local_tests" / "seed-data" / "greenplum" / "reference"
    ref_dir.mkdir(parents=True, exist_ok=True)
    ref_path = ref_dir / "ddl_stage.sql"

    # Load existing reference
    old_ddl = ""
    if ref_path.exists():
        old_ddl = ref_path.read_text(encoding="utf-8")

    # Extract current DDL from stage
    new_ddl = extract_ddl_from_stage()
    if new_ddl is None:
        print("WARNING: Could not connect to stage GP — skipping DDL check.")
        sys.exit(0)

    # Compare
    if new_ddl.strip() == old_ddl.strip():
        print("No DDL changes detected on stage.")
        sys.exit(0)

    # Changes detected
    print("\nDDL changes detected on stage!")
    ref_path.write_text(new_ddl, encoding="utf-8")
    print(f"  Updated: {ref_path}")

    # Show what changed (table-level diff)
    from scripts.convert_ddl import extract_objects
    old_objects = extract_objects(old_ddl) if old_ddl else {"tables": set()}
    new_objects = extract_objects(new_ddl)

    added = sorted(new_objects["tables"] - old_objects.get("tables", set()))
    removed = sorted(old_objects.get("tables", set()) - new_objects["tables"])
    if added:
        print(f"  New tables: {', '.join(added)}")
    if removed:
        print(f"  Removed tables: {', '.join(removed)}")
    if not added and not removed:
        print("  (Column/function/index changes — no tables added or removed)")

    # Regenerate init.sql
    print("\nRegenerating init.sql from updated stage DDL...")
    from scripts.convert_ddl import convert_and_generate
    convert_and_generate(source="stage")


if __name__ == "__main__":
    main()
