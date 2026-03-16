# INFRA.md — Two-Repo Workflow Setup Report

## Date

2026-03-15

## Task Completed

Branch structure, pull/push skill files, and CLAUDE.md Two-Repo Workflow section created as specified in `task_plan.md`.

---

## Branch Structure (Vibecoding-Dev)

All 4 branches are established and contain correct content.

| Branch | Files | Purpose |
|---|---|---|
| `AI/Master` | `.claude/agents/` (4 files), `.claude/commands/pull.md`, `.claude/commands/push.md`, `CLAUDE.md`, `README.md`, `factorial.py`, `fibonacci.py`, `palindrome.py` | Main development branch with all AI artifacts |
| `AI/Stage` | Same as AI/Master | Stage development branch with all AI artifacts |
| `Original/Master` | `README.md`, `factorial.py`, `fibonacci.py`, `palindrome.py` | Clean mirror of Vibecoding-Original `main` — no AI artifacts |
| `Original/Stage` | `README.md`, `factorial.py`, `fibonacci.py`, `palindrome.py` | Clean mirror of Vibecoding-Original `stage` — no AI artifacts |

Dev repo is currently on: `AI/Master`

---

## Skill Files Created

- `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.claude/commands/pull.md`
- `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.claude/commands/push.md`

### How to use /pull

```
/pull main    # syncs Vibecoding-Original main -> Original/Master -> AI/Master
/pull stage   # syncs Vibecoding-Original stage -> Original/Stage -> AI/Stage
```

The skill will:
1. Ask if you ran `git pull` in Vibecoding-Original
2. Verify Vibecoding-Original is on the correct branch
3. Checkout Original/<branch> in Dev repo, delete tracked files, copy from Vibecoding-Original
4. Commit changes on Original/<branch>
5. Merge Original/<branch> into AI/<branch>

### How to use /push

```
/push main    # merges AI/Master -> Original/Master, copies to Vibecoding-Original main
/push stage   # merges AI/Stage -> Original/Stage, copies to Vibecoding-Original stage
```

The skill will:
1. Verify Dev repo is on AI/<branch>
2. Verify Vibecoding-Original is on the correct branch
3. Merge AI/<branch> into Original/<branch> inside Dev repo
4. Delete all files in Vibecoding-Original (except .git/), copy from Dev Original/<branch>
5. Switch Dev repo back to AI/<branch>
6. Remind you to commit and push in Vibecoding-Original

---

## CLAUDE.md

Updated with `## Two-Repo Workflow` section documenting both repos, all 4 branches, the git pipeline, and the AI artifacts exclusion list.

File: `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/CLAUDE.md`

---

## Paths

| Resource | Path |
|---|---|
| Dev repo | `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev` |
| Original repo | `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original` |
| pull skill | `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.claude/commands/pull.md` |
| push skill | `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.claude/commands/push.md` |
| CLAUDE.md | `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/CLAUDE.md` |
