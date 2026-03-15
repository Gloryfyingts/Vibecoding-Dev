# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Data engineering project building pipelines with Apache Flink, Apache Iceberg, S3-compatible storage, and data ingestion from open APIs. May include Rust/Go components. The repository also contains AI agent definitions for structured development workflows.

## Agent-Driven Workflow

This project uses a strict agent pipeline defined in `agents/`. The expected flow for any task is:

1. **planner** (Opus) — creates `task_plan.md` with scope, execution order, risks, and definition of done. No code is written until the plan is approved.
2. **de-coder** (inherit) — implements the plan. Requires DDL context before writing any SQL. Produces a `REPORT.MD` in `.claude/docs/`.
3. **reviewer** (Sonnet) — read-only review against checklist + definition of done from `task_plan.md`. Produces `errors.md` if issues found.
4. **local-repo-devops** (Sonnet) — Docker/infra setup. Produces `INFRA.md` in project root.

Key rule: every task must have a `task_plan.md` with a definition of done before implementation begins.

## Code Conventions

### SQL
- CTEs over subqueries
- Explicit column lists (never `SELECT *`)
- Partition filters on every query to partitioned tables
- `COALESCE`/`IFNULL` for nullable columns in calculations
- `snake_case` for all aliases and column names
- Never fabricate DDL or guess column names — always read actual schema first

### General
- No comments in code — documentation goes to `.claude/docs/`
- `snake_case` naming everywhere
- Docker images must be pinned to specific versions (never `latest`)
- All credentials and ports in `.env` files, never hardcoded
- Never commit `.env` files
- Never run `docker-compose down -v` without explicit user approval

## Review Checklist (Critical — blocks merge)

- SQL: missing partition filters, cartesian products, `SELECT *`, implicit type casts, unbounded queries
- DAG: missing `default_args`, hardcoded connections, dependency cycles, no timeouts
- Python: unhandled exceptions in critical paths, resource leaks, SQL injection risks
- Breaking changes to existing interfaces without backward compatibility
