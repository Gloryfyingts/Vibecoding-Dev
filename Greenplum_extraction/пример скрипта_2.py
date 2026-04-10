#!/usr/bin/env python3
"""convert_ddl.py - Convert Greenplum DDL to PostgreSQL-compatible DDL."""

import re, sys, os
from pathlib import Path


def strip_gp_clauses(content):
    '''Remove GP-specific clauses from DDL.'''
    lines = content.split('\n')
    output = []
    i = 0
    while i < len(lines):
        line = lines[i]
        s = line.strip()

        # 1. Remove top-level DISTRIBUTED BY / REPLICATED (must not be indented)
        if re.match(r'^DISTRIBUTED\s+(BY\s*\(.*?\)|REPLICATED)\s*;?\s*$', line, re.IGNORECASE):
            if output:
                prev = output[-1].rstrip()
                if prev == ')' and s.endswith(';'):
                    output[-1] = ');\n'
            i += 1
            continue

        # 2. Remove top-level WITH (appendonly=...) (must not be indented)
        if re.match(r'^WITH\s*\(\s*appendonly\s*=', line, re.IGNORECASE):
            # If WITH line ends with ; and previous line is just ), add semicolon
            if s.endswith(';') and output:
                prev = output[-1].rstrip()
                if prev == ')':
                    output[-1] = ');' + chr(10)
            i += 1
            continue

        # 3. Remove bare table name lines
        if re.match(r'^[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*\s*$', s):
            i += 1
            continue

        # 2b. Remove multi-line WITH (appendonly...) inside function bodies
        #     Pattern: indented "with (" followed by appendonly= on next lines, closed by ") as"
        if re.match(r'^\s+with\s*\(\s*$', line, re.IGNORECASE):
            # Look ahead to see if next line has appendonly
            if i + 1 < len(lines) and re.search(r'appendonly\s*=', lines[i + 1], re.IGNORECASE):
                # Skip all lines until we find the closing ") as" or just ")"
                j = i + 1
                while j < len(lines):
                    sj = lines[j].strip()
                    if sj.startswith(')'):
                        # Found closing - skip from i to j inclusive
                        # But keep what comes after ) on the same line
                        rest = sj[1:].strip()
                        if rest:
                            # e.g. ") as" - we want to keep " as"
                            indent = len(lines[j]) - len(lines[j].lstrip())
                            output.append(' ' * indent + rest + chr(10))
                        i = j + 1
                        break
                    j += 1
                else:
                    # Didn't find closing, just skip this line
                    output.append(line + chr(10))
                    i += 1
                continue

        modified = line

        # 4. Indented distributed by (function bodies)
        if re.match(r'^\s+distributed\s+by\s*\(', line, re.IGNORECASE):
            if chr(39) in s:
                modified = re.sub(r'\s*distributed\s+by\s*\([^)]*\)\s*', ' ', modified, flags=re.IGNORECASE)
                ms = modified.strip()
                if ms == '':
                    i += 1
                    continue
                # Preserve string-closing characters (e.g. ';) left after removal
                if ms in (';'+chr(39), ';'+chr(39)+';', chr(39), chr(39)+';'):
                    indent = len(line) - len(line.lstrip())
                    output.append(' ' * indent + ms + '\n')
                    i += 1
                    continue
            else:
                if output and s.endswith(';'):
                    prev = output[-1].rstrip()
                    if not prev.endswith(';'):
                        output[-1] = prev + ';\n'
                i += 1
                continue

        # 5. WITH (appendonly=...) inside dynamic SQL
        sq2 = chr(39) + chr(39)
        pat5 = r'WITH\s*\(\s*appendonly\s*=\s*' + sq2
        if re.search(pat5, modified, re.IGNORECASE):
            sub5 = r'\s*WITH\s*\(\s*appendonly\s*=\s*' + sq2 + r'[^)]*\)\s*'
            modified = re.sub(sub5, ' ', modified, flags=re.IGNORECASE)

        # 6. with (appendonly=...) single quotes in function bodies
        sq1 = chr(39)
        pat6 = r'with\s*\(\s*appendonly\s*=\s*' + sq1 + r'[^' + sq1 + r']'
        if re.search(pat6, modified, re.IGNORECASE) and \
           not re.match(r'^WITH\s*\(', s, re.IGNORECASE):
            sub6 = r'\s*with\s*\(\s*appendonly\s*=\s*' + sq1 + r'[^)]*\)\s*'
            modified = re.sub(sub6, ' ', modified, flags=re.IGNORECASE)

        output.append(modified + '\n')
        i += 1

    return ''.join(output)


def extract_objects(content):
    '''Extract schema, table, function, index, constraint names.'''
    objects = {'schemas': set(), 'tables': set(), 'functions': set(), 'indexes': set(), 'constraints': set()}

    for m in re.finditer(r'CREATE\s+SCHEMA\s+IF\s+NOT\s+EXISTS\s+(\S+)\s*;', content, re.IGNORECASE):
        objects['schemas'].add(m.group(1).rstrip(';'))

    for m in re.finditer(r'CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+(\S+)\s*\(', content, re.IGNORECASE):
        objects['tables'].add(m.group(1))

    for m in re.finditer(r'CREATE\s+OR\s+REPLACE\s+FUNCTION\s+([^\s(]+)', content, re.IGNORECASE):
        objects['functions'].add(m.group(1))

    for m in re.finditer(r'CREATE\s+(?:UNIQUE\s+)?INDEX\s+(\S+)\s+ON\s+(\S+)', content, re.IGNORECASE):
        objects['indexes'].add(f"{m.group(1)} ON {m.group(2)}")

    for m in re.finditer(r'ALTER\s+TABLE\s+(\S+)\s+ADD\s+CONSTRAINT\s+(\S+)', content, re.IGNORECASE):
        objects['constraints'].add(f"{m.group(2)} ON {m.group(1)}")

    return objects


def generate_diff_report(prod_objects, stage_objects):
    '''Generate diff report comparing prod and stage DDL.'''
    rlines = []
    rlines.append("=" * 72)
    rlines.append("DDL DIFF REPORT: PROD vs STAGE")
    rlines.append("=" * 72)
    rlines.append("")

    for obj_type in ['schemas', 'tables', 'functions', 'indexes', 'constraints']:
        prod_set = prod_objects[obj_type]
        stage_set = stage_objects[obj_type]
        only_in_prod = sorted(prod_set - stage_set)
        only_in_stage = sorted(stage_set - prod_set)
        in_both = sorted(prod_set & stage_set)

        rlines.append("-" * 72)
        rlines.append(f"{obj_type.upper()}")
        rlines.append("-" * 72)
        rlines.append(f"  In both:       {len(in_both)}")
        rlines.append(f"  Only in PROD:  {len(only_in_prod)}")
        rlines.append(f"  Only in STAGE: {len(only_in_stage)}")
        rlines.append("")

        if only_in_prod:
            rlines.append(f"  Objects ONLY in PROD ({len(only_in_prod)}):" )
            for name in only_in_prod:
                rlines.append(f"    + {name}")
            rlines.append("")

        if only_in_stage:
            rlines.append(f"  Objects ONLY in STAGE ({len(only_in_stage)}):" )
            for name in only_in_stage:
                rlines.append(f"    + {name}")
            rlines.append("")

        if in_both:
            rlines.append(f"  Objects in BOTH ({len(in_both)}):" )
            for name in in_both:
                rlines.append(f"    = {name}")
            rlines.append("")

    return '\n'.join(rlines)


def generate_init_sql(pg_ddl_content, csv_table_map):
    '''Generate init.sql with DDL and COPY commands.'''
    import csv as csv_mod
    base_dir = get_base_dir()
    data_dir = base_dir / "local_tests" / "seed-data" / "greenplum" / "data"

    il = []
    il.append("-- =============================================================")
    il.append("-- init.sql - Greenplum initialization script for local Docker")
    il.append("-- Generated from Greenplum DDL with native GP syntax preserved")
    il.append("-- =============================================================")
    il.append("")
    il.append("SET client_min_messages TO WARNING;")
    il.append("")
    il.append("-- -------------------------------------------------------")
    il.append("-- DDL: Schemas, Tables, Functions, Indexes, Constraints")
    il.append("-- -------------------------------------------------------")
    il.append("")
    il.append(pg_ddl_content)
    il.append("")
    il.append("-- -------------------------------------------------------")
    il.append("-- SEED DATA: Load CSVs via COPY")
    il.append("-- -------------------------------------------------------")
    il.append("")

    for csv_file, table_name in sorted(csv_table_map.items(), key=lambda x: x[0]):
        # Read CSV header to generate column-specific COPY command
        # This prevents failures when the table has new columns not in the CSV
        col_clause = ""
        csv_path = data_dir / csv_file
        if csv_path.exists():
            with open(csv_path, encoding="utf-8", newline="") as f:
                reader = csv_mod.reader(f)
                header = next(reader, None)
                if header:
                    col_clause = " (" + ", ".join(header) + ")"
        il.append(
            f"COPY {table_name}{col_clause} FROM '/docker-entrypoint-initdb.d/data/{csv_file}' "
            f"WITH (FORMAT csv, HEADER true, DELIMITER ',');"
        )

    il.append("")
    il.append("-- Done.")
    il.append("")
    return '\n'.join(il)


CSV_TABLE_MAP = {
    "01_nm_subject.csv": "core.nm_subject",
    "02_feedbacks.csv": "core.feedbacks",
    "03_nm_rating_stat.csv": "core.nm_rating_stat",
    "04_imt_rating_stat.csv": "core.imt_rating_stat",
    "05_seller_tariffs.csv": "core.seller_tariffs",
    "06_seller_tariff_options.csv": "core.seller_tariff_options",
    "07_supplier_analytics_nm_rating.csv": "core.supplier_analytics_nm_rating",
    "08_supplier_analytics_supplier_rating.csv": "core.supplier_analytics_supplier_rating",
    "11_feedbacks_c2c.csv": "core.feedbacks_c2c",
    "12_nm_subject_c2c.csv": "core.nm_subject_c2c",
}


def get_base_dir():
    """Return project root relative to this script."""
    return Path(__file__).resolve().parent.parent


def convert_and_generate(source="stage"):
    """Convert DDL and regenerate init.sql using the given source (stage or prod).

    Returns the PG-compatible DDL content used for init.sql.
    """
    base_dir = get_base_dir()
    ref_dir = base_dir / "local_tests" / "seed-data" / "greenplum" / "reference"
    gp_dir = base_dir / "local_tests" / "seed-data" / "greenplum"

    prod_ddl_path = ref_dir / "ddl_prod.sql"
    stage_ddl_path = ref_dir / "ddl_stage.sql"
    prod_pg_path = ref_dir / "ddl_prod_pg.sql"
    stage_pg_path = ref_dir / "ddl_stage_pg.sql"
    diff_report_path = ref_dir / "ddl_diff_report.txt"
    init_sql_path = gp_dir / "init.sql"

    # Convert prod DDL
    if prod_ddl_path.exists():
        print(f"Reading {prod_ddl_path.name}...")
        prod_content = prod_ddl_path.read_text(encoding='utf-8')
        prod_pg = strip_gp_clauses(prod_content)
        prod_pg_path.write_text(prod_pg, encoding='utf-8')
        print(f"  Written: {prod_pg_path.name}")
    else:
        print(f"  Skipping prod DDL (not found: {prod_ddl_path})")
        prod_content = ""
        prod_pg = ""

    # Convert stage DDL
    if stage_ddl_path.exists():
        print(f"Reading {stage_ddl_path.name}...")
        stage_content = stage_ddl_path.read_text(encoding='utf-8')
        stage_pg = strip_gp_clauses(stage_content)
        stage_pg_path.write_text(stage_pg, encoding='utf-8')
        print(f"  Written: {stage_pg_path.name}")
    else:
        print(f"  Skipping stage DDL (not found: {stage_ddl_path})")
        stage_content = ""
        stage_pg = ""

    # Diff report (only if both exist)
    if prod_content and stage_content:
        print("\nExtracting object lists for diff report...")
        prod_objects = extract_objects(prod_content)
        stage_objects = extract_objects(stage_content)
        diff_report = generate_diff_report(prod_objects, stage_objects)
        diff_report_path.write_text(diff_report, encoding='utf-8')
        print(f"  Written: {diff_report_path.name}")

        print("\n--- DIFF SUMMARY ---")
        for obj_type in ['schemas', 'tables', 'functions']:
            ps = prod_objects[obj_type]
            ss = stage_objects[obj_type]
            print(f"  {obj_type:12s}: {len(ps & ss)} shared, {len(ps - ss)} prod-only, {len(ss - ps)} stage-only")

    # Select source DDL for init.sql
    # Use ORIGINAL Greenplum DDL (not PG-converted) because the local Docker
    # container is Greenplum (woblerr/greenplum), which supports GP syntax natively.
    # The PG-converted files are kept for reference/comparison only.
    if source == "stage":
        ddl_for_init = stage_content or prod_content
        label = "stage (GP native)" if stage_content else "prod (GP native, fallback)"
    else:
        ddl_for_init = prod_content or stage_content
        label = "prod (GP native)" if prod_content else "stage (GP native, fallback)"

    if not ddl_for_init:
        print("\nERROR: No DDL source available for init.sql generation.")
        return ""

    print(f"\nGenerating init.sql from {label} DDL...")
    init_sql = generate_init_sql(ddl_for_init, CSV_TABLE_MAP)
    init_sql_path.write_text(init_sql, encoding='utf-8')
    print(f"  Written: {init_sql_path}")

    # Verification
    print("\n--- VERIFICATION ---")
    ns = len(re.findall(r'CREATE SCHEMA', ddl_for_init, re.IGNORECASE))
    nt = len(re.findall(r'CREATE TABLE', ddl_for_init, re.IGNORECASE))
    nf = len(re.findall(r'CREATE OR REPLACE FUNCTION', ddl_for_init, re.IGNORECASE))
    print(f"  init.sql: {ns} schemas, {nt} tables, {nf} functions")

    print("\nDone! All files generated successfully.")
    return ddl_for_init


def main():
    import argparse

    parser = argparse.ArgumentParser(description="Convert Greenplum DDL to PostgreSQL-compatible DDL")
    parser.add_argument(
        "--source",
        choices=["stage", "prod"],
        default="stage",
        help="Which DDL source to use for init.sql generation (default: stage)",
    )
    args = parser.parse_args()
    convert_and_generate(source=args.source)


if __name__ == "__main__":
    main()
