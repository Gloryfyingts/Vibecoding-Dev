import argparse
import os
import re
import sys
from pathlib import Path

try:
    from dotenv import load_dotenv
except ImportError:
    print(
        "python-dotenv is not installed. "
        "Run: pip install -r requirements.txt",
        file=sys.stderr,
    )
    sys.exit(1)

import clickhouse_connect


def parse_args():
    parser = argparse.ArgumentParser(
        description="Extract ClickHouse DDL into an idempotent SQL file"
    )
    parser.add_argument("--host", default=None)
    parser.add_argument("--port", type=int, default=None)
    parser.add_argument("--user", default=None)
    parser.add_argument("--password", default=None)
    parser.add_argument("--env-file", default=None)
    parser.add_argument("--output", default="clickhouse_ddl_dump.sql")
    parser.add_argument("--databases", default=None)
    parser.add_argument("--exclude-dbs", default=None)
    return parser.parse_args()


def load_credentials(args):
    script_dir = Path(__file__).resolve().parent
    env_file_path = args.env_file if args.env_file else str(script_dir / ".env")

    if not Path(env_file_path).is_absolute():
        env_file_path = str(script_dir / env_file_path)

    if Path(env_file_path).exists():
        load_dotenv(env_file_path, override=False)

    host = args.host if args.host is not None else os.environ.get("CLICKHOUSE_HOST", "localhost")
    port = args.port if args.port is not None else int(os.environ.get("CLICKHOUSE_PORT", "18123"))
    user = args.user if args.user is not None else os.environ.get("CLICKHOUSE_USER", "default")
    password = args.password if args.password is not None else os.environ.get("CLICKHOUSE_PASSWORD", "")

    return host, port, user, password


def connect(host, port, user, password):
    return clickhouse_connect.get_client(
        host=host,
        port=port,
        username=user,
        password=password,
        interface="http",
    )


def detect_clusters(client):
    result = client.query(
        "SELECT cluster, shard_num, replica_num, host_name "
        "FROM system.clusters "
        "WHERE cluster NOT LIKE 'test\\_%' "
        "ORDER BY cluster, shard_num, replica_num"
    )
    clusters = {}
    for row in result.result_rows:
        cluster_name = row[0]
        if cluster_name not in clusters:
            clusters[cluster_name] = []
        clusters[cluster_name].append({
            "shard_num": row[1],
            "replica_num": row[2],
            "host_name": row[3],
        })
    return clusters


def resolve_cluster(clusters, client):
    if not clusters:
        return None
    if len(clusters) == 1:
        return list(clusters.keys())[0]

    try:
        result = client.query(
            "SELECT engine_full "
            "FROM system.tables "
            "WHERE engine = 'Distributed' "
            "  AND database NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA', 'default') "
            "LIMIT 100"
        )
        for row in result.result_rows:
            engine_full = row[0]
            for cluster_name in clusters:
                if cluster_name in engine_full:
                    return cluster_name
    except Exception as exc:
        print(
            f"Warning: failed to resolve cluster from Distributed tables: {exc}",
            file=sys.stderr,
        )

    cluster_names = sorted(clusters.keys())
    print(
        f"Warning: multiple clusters detected ({', '.join(cluster_names)}), "
        f"using '{cluster_names[0]}'",
        file=sys.stderr,
    )
    return cluster_names[0]


def get_macros(client):
    try:
        result = client.query("SELECT macro, substitution FROM system.macros")
        return {row[0]: row[1] for row in result.result_rows}
    except Exception as exc:
        print(
            f"Warning: could not read system.macros, "
            f"ReplicatedMergeTree paths will not be re-parameterized: {exc}",
            file=sys.stderr,
        )
        return {}


def get_databases(client, include, exclude):
    system_dbs = ("system", "information_schema", "INFORMATION_SCHEMA", "default")
    result = client.query(
        "SELECT name FROM system.databases ORDER BY name"
    )
    databases = [
        row[0]
        for row in result.result_rows
        if row[0] not in system_dbs
    ]

    if include:
        include_set = {db.strip() for db in include.split(",")}
        databases = [db for db in databases if db in include_set]

    if exclude:
        exclude_set = {db.strip() for db in exclude.split(",")}
        databases = [db for db in databases if db not in exclude_set]

    return databases


def get_show_create(client, object_type, full_name):
    try:
        result = client.query(f"SHOW CREATE {object_type} {full_name}")
        if result.result_rows:
            return result.result_rows[0][0]
    except Exception as exc:
        print(
            f"Warning: SHOW CREATE {object_type} {full_name} failed: {exc}",
            file=sys.stderr,
        )
    return None


def inject_if_not_exists(ddl):
    check_pattern = re.compile(
        r"^CREATE\s+(?:TABLE|DATABASE|DICTIONARY|(?:MATERIALIZED\s+)?VIEW)"
        r"\s+IF\s+NOT\s+EXISTS\b",
        re.IGNORECASE,
    )
    if check_pattern.match(ddl):
        return ddl

    inject_pattern = re.compile(
        r"^(CREATE\s+(?:TABLE|DATABASE|DICTIONARY|(?:MATERIALIZED\s+)?VIEW))"
        r"(\s+)",
        re.IGNORECASE,
    )
    return inject_pattern.sub(r"\1 IF NOT EXISTS\2", ddl, count=1)


def inject_on_cluster_database(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+DATABASE\s+(?:IF\s+NOT\s+EXISTS\s+)?`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_table(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?`?[\w]+`?\.`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_view(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?`?[\w]+`?\.`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_dictionary(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+DICTIONARY\s+(?:IF\s+NOT\s+EXISTS\s+)?`?[\w]+`?\.`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_function(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_settings_profile(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+SETTINGS\s+PROFILE\s+`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def inject_on_cluster_row_policy(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+ROW\s+POLICY\s+(?:IF\s+NOT\s+EXISTS\s+)?`?[\w]+`?)\s+(ON\s+)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name} \2", ddl, count=1)


def inject_on_cluster_quota(ddl, cluster_name):
    if not cluster_name:
        return ddl

    pattern = re.compile(
        r"(CREATE\s+QUOTA\s+`?[\w]+`?)",
        re.IGNORECASE,
    )
    return pattern.sub(rf"\1 ON CLUSTER {cluster_name}", ddl, count=1)


def reparameterize_replicated_paths(ddl, macros):
    shard_val = macros.get("shard")
    replica_val = macros.get("replica")
    if not shard_val or not replica_val:
        return ddl

    def replace_engine_args(match):
        full = match.group(0)
        full = full.replace(f"/{shard_val}/", "/{shard}/")
        full = full.replace(f"'{replica_val}'", "'{replica}'")
        return full

    return re.sub(
        r"Replicated\w*MergeTree\((?:'[^']*'|[^)])*\)",
        replace_engine_args,
        ddl,
    )


def make_or_replace_function(ddl):
    pattern = re.compile(r"CREATE\s+FUNCTION", re.IGNORECASE)
    return pattern.sub("CREATE OR REPLACE FUNCTION", ddl, count=1)


def build_db_filter_clause(databases):
    escaped = ", ".join(f"'{db.replace(chr(39), chr(39)*2)}'" for db in databases)
    return f"database IN ({escaped})"


def extract_databases(client, cluster_name, databases):
    statements = []
    for db in databases:
        ddl = f"CREATE DATABASE IF NOT EXISTS {db}"
        ddl = inject_on_cluster_database(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_local_tables(client, cluster_name, macros, databases):
    if not databases:
        return []

    db_filter = build_db_filter_clause(databases)
    result = client.query(
        f"SELECT database, name "
        f"FROM system.tables "
        f"WHERE {db_filter} "
        f"  AND engine NOT IN ('View', 'MaterializedView', 'Distributed') "
        f"  AND NOT startsWith(name, '.inner') "
        f"ORDER BY database, name"
    )

    statements = []
    for row in result.result_rows:
        database, table_name = row[0], row[1]
        ddl = get_show_create(client, "TABLE", f"`{database}`.`{table_name}`")
        if not ddl:
            continue
        ddl = inject_if_not_exists(ddl)
        ddl = inject_on_cluster_table(ddl, cluster_name)
        ddl = reparameterize_replicated_paths(ddl, macros)
        statements.append(ddl)
    return statements


def extract_views(client, cluster_name, databases):
    if not databases:
        return []

    db_filter = build_db_filter_clause(databases)
    result = client.query(
        f"SELECT database, name, engine "
        f"FROM system.tables "
        f"WHERE {db_filter} "
        f"  AND engine IN ('View', 'MaterializedView') "
        f"  AND NOT startsWith(name, '.inner') "
        f"ORDER BY database, engine, name"
    )

    statements = []
    for row in result.result_rows:
        database, view_name = row[0], row[1]
        ddl = get_show_create(client, "TABLE", f"`{database}`.`{view_name}`")
        if not ddl:
            continue
        ddl = inject_if_not_exists(ddl)
        ddl = inject_on_cluster_view(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_distributed_tables(client, cluster_name, databases):
    if not databases:
        return []

    db_filter = build_db_filter_clause(databases)
    result = client.query(
        f"SELECT database, name "
        f"FROM system.tables "
        f"WHERE {db_filter} "
        f"  AND engine = 'Distributed' "
        f"ORDER BY database, name"
    )

    statements = []
    for row in result.result_rows:
        database, table_name = row[0], row[1]
        ddl = get_show_create(client, "TABLE", f"`{database}`.`{table_name}`")
        if not ddl:
            continue
        ddl = inject_if_not_exists(ddl)
        ddl = inject_on_cluster_table(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_dictionaries(client, cluster_name, databases):
    if not databases:
        return []

    db_filter = build_db_filter_clause(databases)
    try:
        result = client.query(
            f"SELECT database, name "
            f"FROM system.dictionaries "
            f"WHERE {db_filter} "
            f"ORDER BY database, name"
        )
    except Exception:
        return []

    statements = []
    for row in result.result_rows:
        database, dict_name = row[0], row[1]
        ddl = get_show_create(client, "DICTIONARY", f"`{database}`.`{dict_name}`")
        if not ddl:
            continue
        ddl = inject_if_not_exists(ddl)
        ddl = inject_on_cluster_dictionary(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_functions(client, cluster_name):
    try:
        result = client.query(
            "SELECT name, create_query "
            "FROM system.functions "
            "WHERE origin = 'SQLUserDefined' "
            "ORDER BY name"
        )
    except Exception:
        return []

    statements = []
    for row in result.result_rows:
        func_name, create_query = row[0], row[1]
        if create_query:
            ddl = create_query
        else:
            ddl = get_show_create(client, "FUNCTION", func_name)
            if not ddl:
                continue
        ddl = make_or_replace_function(ddl)
        ddl = inject_on_cluster_function(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_settings_profiles(client, cluster_name):
    try:
        result = client.query(
            "SELECT name "
            "FROM system.settings_profiles "
            "WHERE storage = 'local_directory' "
            "ORDER BY name"
        )
    except Exception:
        return []

    statements = []
    for row in result.result_rows:
        profile_name = row[0]
        ddl = get_show_create(client, "SETTINGS PROFILE", f"`{profile_name}`")
        if not ddl:
            continue
        ddl = inject_on_cluster_settings_profile(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_row_policies(client, cluster_name):
    try:
        result = client.query(
            "SELECT short_name, database, table "
            "FROM system.row_policies "
            "WHERE storage = 'local_directory' "
            "ORDER BY short_name"
        )
    except Exception:
        return []

    statements = []
    for row in result.result_rows:
        policy_name, database, table = row[0], row[1], row[2]
        ddl = get_show_create(
            client,
            "ROW POLICY",
            f"`{policy_name}` ON `{database}`.`{table}`",
        )
        if not ddl:
            continue
        ddl = inject_on_cluster_row_policy(ddl, cluster_name)
        statements.append(ddl)
    return statements


def extract_quotas(client, cluster_name):
    try:
        result = client.query(
            "SELECT name "
            "FROM system.quotas "
            "WHERE storage = 'local_directory' "
            "ORDER BY name"
        )
    except Exception:
        return []

    statements = []
    for row in result.result_rows:
        quota_name = row[0]
        ddl = get_show_create(client, "QUOTA", f"`{quota_name}`")
        if not ddl:
            continue
        ddl = inject_on_cluster_quota(ddl, cluster_name)
        statements.append(ddl)
    return statements


def ensure_trailing_semicolon(statement):
    stripped = statement.rstrip()
    if not stripped.endswith(";"):
        stripped += ";"
    return stripped


def format_output(sections):
    parts = []
    for header, statements in sections:
        if not statements:
            continue
        parts.append(f"-- === {header} ===")
        parts.append("")
        for stmt in statements:
            parts.append(ensure_trailing_semicolon(stmt))
            parts.append("")
    if parts and parts[-1] == "":
        parts = parts[:-1]
    return "\n".join(parts) + "\n"


def main():
    args = parse_args()

    try:
        host, port, user, password = load_credentials(args)
    except Exception as exc:
        print(f"Error loading credentials: {exc}", file=sys.stderr)
        sys.exit(1)

    try:
        client = connect(host, port, user, password)
        client.query("SELECT 1")
    except Exception as exc:
        print(
            f"Error: cannot connect to ClickHouse at {host}:{port} - {exc}",
            file=sys.stderr,
        )
        sys.exit(1)

    try:
        clusters = detect_clusters(client)
        cluster_name = resolve_cluster(clusters, client)
        macros = get_macros(client)
        databases = get_databases(client, args.databases, args.exclude_dbs)
    except Exception as exc:
        print(f"Error querying system tables: {exc}", file=sys.stderr)
        sys.exit(1)

    sections = [
        ("Databases", extract_databases(client, cluster_name, databases)),
        ("Tables (Local Engines)", extract_local_tables(client, cluster_name, macros, databases)),
        ("Views and Materialized Views", extract_views(client, cluster_name, databases)),
        ("Distributed Tables", extract_distributed_tables(client, cluster_name, databases)),
        ("Dictionaries", extract_dictionaries(client, cluster_name, databases)),
        ("User-Defined Functions", extract_functions(client, cluster_name)),
        ("Settings Profiles", extract_settings_profiles(client, cluster_name)),
        ("Row Policies", extract_row_policies(client, cluster_name)),
        ("Quotas", extract_quotas(client, cluster_name)),
    ]

    output = format_output(sections)

    try:
        output_path = Path(args.output)
        output_path.write_text(output, encoding="utf-8")
        print(f"DDL written to {output_path.resolve()}")
    except Exception as exc:
        print(f"Error writing output file: {exc}", file=sys.stderr)
        sys.exit(1)

    sys.exit(0)


if __name__ == "__main__":
    main()
