# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Data Engineering project building pipelines with Flink, Iceberg, S3, open API ingestion, and possibly Rust/Go components. Repository contains AI agent definitions and will grow to include ETL code, DAGs, SQL, Docker infrastructure.

## Agent-Driven Workflow

This repo uses custom Claude Code agents (`agents/`) that enforce a strict task lifecycle:

1. **planner** (opus) — creates `task_plan.md` with scope, execution order, risks, and definition of done. Must run BEFORE any complex task. Plan requires user approval before implementation begins.
2. **de-coder** — implements SQL, ETL, Spark jobs, Airflow DAGs. Will NOT start without a definition of done in `task_plan.md` or context window.
3. **reviewer** (sonnet) — read-only review after every code change. Checks against `task_plan.md` definition of done. Outputs `errors.md` if issues found.
4. **local-repo-devops** (sonnet) — Docker/infra/environment setup. Outputs `INFRA.md` in project root.

## Key Conventions

- **No comments in code** — documentation goes to `.claude/docs/` as markdown files
- **No SELECT \*** — explicit column lists always
- **CTEs over subqueries** — wherever possible
- **snake_case** — all aliases, column names, identifiers
- **Partition filters required** — on every query to partitioned tables
- **COALESCE/IFNULL** — for nullable columns in calculations
- **DDL-first** — never write or review SQL without reading actual table schemas. If DDL is missing, stop and ask.
- **Docker images** — always pin versions, never use `latest`
- **Credentials** — in `.env` only, never hardcoded, never committed

## File Conventions

| File | Purpose |
|------|---------|
| `task_plan.md` | Current task plan with definition of done (created by planner) |
| `errors.md` | Review issues from reviewer agent |
| `INFRA.md` | Running services/ports documentation |
| `.claude/docs/REPORT.MD` | Execution report from de-coder agent |

## Critical Rules

- Never run `docker-compose down -v` without explicit user approval (destroys volumes)
- Never fabricate DDL or guess column names
- Always check port availability before starting services
- Plans must be specific and include risks/mitigations
- Reviewer is read-only — reports issues but does not fix them
