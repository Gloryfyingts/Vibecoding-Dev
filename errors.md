# errors.md — Review Results

Verdict: NOT_APPROVED

Reviewer: reviewer agent
Date: 2026-03-15
Scope: Two-Repo Workflow task (branch setup, pull/push skills, CLAUDE.md update)

---

## Critical Issues (block merge)

### CRITICAL-1: `.claude/commands/pull.md` line 99 — Dangerous code block destroys Dev repo git history

**File**: `.claude/commands/pull.md`
**Lines**: 98-100 (first bash block in Step 7)

**Problem**: The file presents this command as the primary approach for copying files:

```bash
cp -r "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original/." "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/" && rm -rf "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.git"
```

`cp -r .` copies ALL contents of the source directory including the hidden `.git/` directory. This overwrites Dev repo's `.git/` with Original repo's `.git/`. Then `rm -rf .../.git` deletes the now-replaced `.git/` directory entirely, destroying all Dev repo commit history, branches, and configuration. Running this command is unrecoverable without a remote backup.

**Why this violates the plan**: The task_plan.md detailed pull spec (lines 183-185) describes only the safe `find`+`cp` approach (copy each top-level item individually, skipping `.git`). The dangerous `cp -r .` block was never part of the specification. It appears the coder added it as a "first attempt" and then partially corrected it with a note and safe alternative — but left the dangerous block in the file.

**Impact**: If a Claude agent executing the `/pull` skill runs the first bash block in Step 7, the Dev repo's entire git history is destroyed. This is a data-loss risk.

**Required fix**: Remove lines 98-100 (the dangerous `cp -r ... && rm -rf` block) and the note on lines 102-103 entirely. Keep only the safe `find`+`cp` approach (lines 104-109). The Step 7 section should begin directly with the safe implementation.

---

### CRITICAL-2: `.claude/commands/pull.md` Step 6 — File cleanup does not match plan specification

**File**: `.claude/commands/pull.md`
**Lines**: 91-93 (Step 6)

**Problem**: Step 6 runs:
```bash
git ls-files | xargs rm -f
```

This removes only git-tracked files. The task_plan.md pull spec (line 183) says: "Remove all files in Dev working tree except `.git/` directory." Using `git ls-files` misses untracked files that may exist in the working tree from prior state (e.g., a failed partial sync that left untracked files). If untracked files remain and are later committed via `git add -A`, they could corrupt the sync.

**Required fix**: Replace with a `find`-based approach that removes all non-`.git` content, consistent with the plan spec and matching the push skill's approach (push.md Step 7 line 132). Example:
```bash
find "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" -maxdepth 1 -mindepth 1 -not -name '.git' -exec rm -rf {} +
```

---

## Warnings (should fix)

### WARNING-1: `CLAUDE.md` line 25 — Git pipeline step 4 description diverges from task_plan.md scope section

**File**: `CLAUDE.md` line 25 vs `task_plan.md` line 62

**Problem**: CLAUDE.md correctly documents the push pipeline as:
> `/push <branch>` merges `AI/<branch>` into `Original/<branch>`, then copies to Vibecoding-Original

But `task_plan.md` scope section (line 62) still shows the OLD incorrect version:
> `/push <branch>` copies non-AI files from `AI/<branch>` to Vibecoding-Original

The task_plan.md was never updated to reflect the critical correction made during planning. CLAUDE.md is correct; task_plan.md scope is stale.

**Impact**: Future agents reading task_plan.md scope may implement push incorrectly, bypassing the merge step.

**Required fix**: Update task_plan.md line 62 to match the corrected pipeline:
```
4. `/push <branch>` merges `AI/<branch>` into `Original/<branch>`, then copies to Vibecoding-Original
```

---

### WARNING-2: `.claude/commands/push.md` line 82 — Misleading comment about reading `.git/index`

**File**: `.claude/commands/push.md`
**Lines**: 84

**Problem**: The instruction says "Read the file `$MAIN_REPO/.git/index` status by listing the working tree." The `.git/index` file is a binary file and cannot be meaningfully read to determine uncommitted changes. The instruction then correctly pivots to asking the user, but the initial sentence is technically incorrect and misleading.

**Required fix**: Remove the reference to reading `.git/index`. Replace with: "Since you cannot run git commands in Vibecoding-Original, you cannot programmatically check for uncommitted changes. Warn the user instead:"

---

## Passed Checks

- All 4 branches exist: `AI/Master`, `AI/Stage`, `Original/Master`, `Original/Stage`
- `Original/Master` is an orphan branch containing only `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md`
- `Original/Stage` branched from `Original/Master` with correct content
- `AI/Master` contains all expected files including skill files
- `AI/Stage` has AI artifacts propagated via merge from `AI/Master`
- Dev repo HEAD is on `AI/Master` after all setup
- push.md correctly implements merge of AI/ into Original/ BEFORE copying to main repo (Step 6)
- push.md correctly uses `-mindepth 1` to avoid deleting `.git/` in Original repo
- push.md safety-net exclusion list is complete: `.git/`, `.claude/`, `CLAUDE.md`, `task_plan.md`, `errors.md`, `INFRA.md`, `prompt.md`, `REPORT.MD`
- pull.md forces user confirmation of `git pull` before proceeding (Step 3)
- pull.md reads `.git/HEAD` to verify branch (Step 4)
- pull.md handles detached HEAD state
- All bash paths in both skill files use double-quoted paths with parentheses handled correctly
- CLAUDE.md Two-Repo Workflow section is present with all required content
- CLAUDE.md existing sections (Project Overview, Agent Workflow, Key Rules) are preserved
- No git commands against Vibecoding-Original in any skill

---

## Summary

| Severity | Count | Items |
|----------|-------|-------|
| Critical | 2 | pull.md dangerous block; pull.md cleanup mismatch |
| Warning  | 2 | task_plan.md stale scope; push.md misleading comment |
| Suggestion | 0 | — |

**DoD criteria failed**: 1 of 23 (pull.md Step 7 dangerous code block violates "handles additions, edits, deletions" criterion safely)

**Action required**: Fix CRITICAL-1 and CRITICAL-2 in `.claude/commands/pull.md` before this can be approved. WARNING-1 in `task_plan.md` should also be corrected to prevent future agent confusion.
