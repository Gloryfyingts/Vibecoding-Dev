# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Data engineering project building pipelines with Apache Flink, Apache Iceberg, S3 storage, and open API data ingestion. Rust/Go may be introduced later. The repo includes AI agent definitions for Claude Code to orchestrate development.

## Agent Workflow

This repo uses four custom Claude Code agents (`.claude/agents/`) that enforce a strict development loop:

1. **planner** (opus) — Must be invoked FIRST for any task. Produces `task_plan.md` with scope, execution order, risks, and a strict definition of done. Waits for user approval before any code is written.
2. **de-coder** — Writes SQL, ETL scripts, Spark jobs, Airflow DAGs. Reads `task_plan.md` before starting. Reports to `.claude/docs/REPORT.MD`.
3. **local-repo-devops** (sonnet) — Docker, docker-compose, databases, Airflow setup. Reports to `INFRA.md` in project root.
4. **reviewer** (sonnet) — Read-only review after every code change. Checks against `task_plan.md` definition of done. Outputs `errors.md` if issues found.

The enforced sequence is: **plan → code → review**. Never skip planning. Never skip review.

## Key Rules

- **No comments in code** — documentation goes to `.claude/docs/`
- **Never fabricate DDL** — always read actual CREATE TABLE statements before writing SQL. If DDL is missing, stop and ask.
- **No `SELECT *`** — use explicit column lists
- **CTEs over subqueries**
- **Partition filters required** on every query to partitioned tables
- **COALESCE/IFNULL** for nullable columns in calculations
- **snake_case** for all aliases and column names
- **Docker images must be pinned** — never use `latest` tag
- **Credentials in `.env` only** — never hardcode in docker-compose or code; never commit `.env`
- **Never run `docker-compose down -v`** without explicit user approval
- **`task_plan.md` must have a definition of done** — agents will refuse to work without it
