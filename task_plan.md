# Task: Two-Repo Workflow -- Branch Structure, Pull/Push Skills, CLAUDE.md Update

## Context

Two repositories:
- **Vibecoding-Original** (`C:\Users\Wildberries(Work)\Desktop\VBGuide\Vibecoding-Original`) -- clean showcase repo, no AI artifacts, remote `https://github.com/Gloryfyingts/Vibecoding-Original.git`, branches: `main`, `stage`. NO git commands by Claude ever.
- **Vibecoding-Dev** (`C:\Users\Wildberries(Work)\Desktop\VBGuide\Vibecoding-Dev`) -- dev repo, AI artifacts allowed, remote `https://github.com/Gloryfyingts/Vibecoding-Dev.git`, currently only has `main` branch. All git commands allowed here.

Current Dev repo `main` branch contents: `.claude/agents/` (4 agent files), `CLAUDE.md`, `README.md`, `prompt.md`.
Current Original repo contents (both branches identical): `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md`.

---

## Scope

### Files to CREATE

1. **`.claude/commands/pull.md`** -- Claude Code Skill that syncs changes FROM Vibecoding-Original INTO Vibecoding-Dev. Accepts `$ARGUMENTS` as branch name (`stage` or `main`). The skill prompt instructs Claude to:
   - Validate `$ARGUMENTS` is either `stage` or `main`; abort if invalid
   - Map branch names: `stage` -> `Original/Stage` + `AI/Stage`; `main` -> `Original/Master` + `AI/Master`
   - Ask user to confirm they have run `git pull` in Vibecoding-Original on the correct branch; refuse to proceed until confirmed
   - Determine which branch Original repo is currently on by reading `Vibecoding-Original/.git/HEAD` and following the ref; if it does not match the requested branch, instruct user to `git checkout <branch>` in Original repo and abort
   - In Dev repo: `git checkout Original/<branch>`
   - Sync files: delete all tracked non-`.git` files in Dev repo working tree, then copy all files (excluding `.git/`) from Original repo into Dev repo working tree using `rsync` with `--delete` flag (or equivalent `cp -r` + cleanup approach)
   - `git add -A && git commit -m "sync: pull from Original/<branch> $(date)"` (skip commit if nothing changed)
   - `git checkout AI/<branch>`
   - `git merge Original/<branch> -m "merge: Original/<branch> into AI/<branch> $(date)"`
   - Report what happened (files added/modified/deleted, merge result)

2. **`.claude/commands/push.md`** -- Claude Code Skill that syncs changes FROM Vibecoding-Dev AI/ branch INTO Vibecoding-Original. Accepts `$ARGUMENTS` as branch name (`stage` or `main`). The skill prompt instructs Claude to:
   - Validate `$ARGUMENTS` is either `stage` or `main`; abort if invalid
   - Map branch names: `stage` -> `AI/Stage` + `Original/Stage`; `main` -> `AI/Master` + `Original/Master`
   - Verify Dev repo is currently on the correct `AI/<branch>` or switch to it
   - Determine which branch Original repo is currently on by reading `Vibecoding-Original/.git/HEAD`; if it does not match the requested branch, instruct user to `git checkout <branch>` in Original repo and abort
   - **Merge AI/ into Original/ in Dev repo**: `git checkout Original/<branch>`, `git merge AI/<branch>`. This propagates dev changes to the clean branch. If conflicts, report and let user resolve.
   - Delete all files in Original repo working tree (excluding `.git/`)
   - Copy all files from Dev repo `Original/<branch>` working tree into Original repo (already clean; safety-net exclusion of `.claude/`, `CLAUDE.md`, `task_plan.md`, `errors.md`, `INFRA.md`, `prompt.md`, `REPORT.MD`, `.git/`)
   - Switch Dev repo back to `AI/<branch>`
   - Report what files were added/modified/deleted in Original repo
   - Remind user they need to `git add`, `git commit`, and `git push` in Original repo manually

### Files to MODIFY

3. **`CLAUDE.md`** -- Add a new section `## Two-Repo Workflow` after the existing `## Project Overview` section. Content to add (compressed context):
   ```
   ## Two-Repo Workflow

   This project uses two repositories:
   - **Vibecoding-Original** (`../Vibecoding-Original`) -- clean showcase repo, no AI artifacts. Remote: `https://github.com/Gloryfyingts/Vibecoding-Original.git`. Branches: `main`, `stage`. Claude must NEVER run git commands in this repo.
   - **Vibecoding-Dev** (this repo) -- development repo with AI artifacts. Branches: `AI/Master`, `AI/Stage`, `Original/Master`, `Original/Stage`.

   Branch mapping:
   - `Original/Master` = mirror of Vibecoding-Original `main` (no AI artifacts)
   - `Original/Stage` = mirror of Vibecoding-Original `stage` (no AI artifacts)
   - `AI/Master` = development branch for main features (has AI artifacts)
   - `AI/Stage` = development branch for stage features (has AI artifacts)

   Git pipeline:
   1. User runs `git pull` in Vibecoding-Original
   2. `/pull <branch>` copies files into `Original/<branch>`, then merges into `AI/<branch>`
   3. Development happens on `AI/<branch>`
   4. `/push <branch>` copies non-AI files from `AI/<branch>` to Vibecoding-Original
   5. User manually commits and pushes in Vibecoding-Original

   AI artifacts exclusion list (never copied to Original): `.claude/`, `CLAUDE.md`, `task_plan.md`, `errors.md`, `INFRA.md`, `prompt.md`, `REPORT.MD`
   ```

### Git operations (to be executed as part of implementation, not as files)

4. **Branch setup in Dev repo** -- Must be done BEFORE the skills can be used:
   - Rename current `main` branch to `AI/Master` (it already has AI artifacts)
   - Create `Original/Master` as orphan branch, clean it, copy files from Vibecoding-Original `main`, commit
   - Create `Original/Stage` as orphan branch, clean it, copy files from Vibecoding-Original `stage`, commit
   - Create `AI/Stage` from `Original/Stage`, then merge in AI artifacts from `AI/Master` (or cherry-pick the AI artifact commits)
   - Verify all 4 branches exist and have correct content

---

## Execution Order

### Phase 1: Branch setup in Dev repo (prerequisite for everything)

1. **Rename `main` to `AI/Master`** -- This preserves all existing commits, AI artifacts, and agent files. Must happen first because all other branches are derived from or related to this one. Commands:
   ```bash
   cd /c/Users/Wildberries\(Work\)/Desktop/VBGuide/Vibecoding-Dev
   git branch -m main AI/Master
   ```

2. **Create `Original/Master` orphan branch** -- Must be an orphan so it has no AI artifact history. Depends on step 1 being done so we don't conflict with `main`.
   ```bash
   git checkout --orphan Original/Master
   git rm -rf .
   # Copy files from Vibecoding-Original (which is on main branch)
   # cp factorial.py fibonacci.py palindrome.py README.md into Dev working tree
   git add -A
   git commit -m "init: mirror of Vibecoding-Original main"
   ```

3. **Create `Original/Stage` orphan branch** -- Requires user to switch Original repo to `stage` branch first, OR since content is identical right now, we can just branch from `Original/Master`.
   ```bash
   git checkout -b Original/Stage Original/Master
   # Content is identical right now, just commit with different message
   git commit --allow-empty -m "init: mirror of Vibecoding-Original stage"
   ```
   Note: Since both branches in Original repo have identical content, branching from `Original/Master` is correct and avoids needing the user to switch branches in Original repo.

4. **Create `AI/Stage` from `Original/Stage` + AI artifacts** -- Depends on steps 1 and 3. We checkout `Original/Stage`, create `AI/Stage` from it, then merge `AI/Master` to bring in AI artifacts.
   ```bash
   git checkout -b AI/Stage Original/Stage
   git merge AI/Master --allow-unrelated-histories -m "init: merge AI artifacts into AI/Stage"
   ```

5. **Verify all 4 branches exist with correct content**:
   - `AI/Master`: has `.claude/`, `CLAUDE.md`, `README.md`, `prompt.md` (existing dev files -- does NOT yet have Original's python files; these will arrive via the first `/pull main`)
   - `AI/Stage`: has same content as `AI/Master` after merge (will get python files via first `/pull stage`)
   - `Original/Master`: has `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md` only
   - `Original/Stage`: has `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md` only

### Phase 2: Perform initial pull to populate AI branches with Original content

6. **Manually run the pull logic for `main`** -- Checkout `AI/Master`, merge `Original/Master` into it. This brings the python files into the AI branch.
   ```bash
   git checkout AI/Master
   git merge Original/Master --allow-unrelated-histories -m "init: pull Original/Master into AI/Master"
   ```

7. **Manually run the pull logic for `stage`** -- Checkout `AI/Stage`, merge `Original/Stage` into it (should already be merged from step 4, but verify).
   ```bash
   git checkout AI/Stage
   git merge Original/Stage -m "init: pull Original/Stage into AI/Stage"
   ```
   This may be a no-op if step 4 already incorporated everything.

### Phase 3: Create skill files (on AI/Master branch)

8. **Switch to `AI/Master`** -- Skills are AI artifacts, they live on AI/ branches.
   ```bash
   git checkout AI/Master
   ```

9. **Create `.claude/commands/pull.md`** -- Write the pull skill file. Depends on branch structure being ready so the skill references valid branch names.

10. **Create `.claude/commands/push.md`** -- Write the push skill file. Independent of step 9 but same branch.

11. **Update `CLAUDE.md`** -- Add the two-repo workflow section. Same branch as steps 9-10.

12. **Commit skill files and CLAUDE.md update on `AI/Master`**:
    ```bash
    git add .claude/commands/pull.md .claude/commands/push.md CLAUDE.md
    git commit -m "feat: add pull/push skills and update CLAUDE.md with two-repo workflow"
    ```

13. **Propagate skills to `AI/Stage`** -- Merge `AI/Master` into `AI/Stage` so both AI branches have the skills.
    ```bash
    git checkout AI/Stage
    git merge AI/Master -m "merge: propagate pull/push skills to AI/Stage"
    ```

### Phase 4: Verification

14. **Verify final state of all branches** -- Check that each branch has exactly the expected files.

---

## Detailed Skill Specifications

### `.claude/commands/pull.md` -- detailed behavior

The skill receives `$ARGUMENTS` which should be `stage` or `main`.

Step-by-step logic the skill must instruct Claude to perform:

1. Parse `$ARGUMENTS`. If empty or not `stage`/`main`, print usage and abort.
2. Set variables:
   - If `$ARGUMENTS` = `stage`: `ORIGINAL_BRANCH=Original/Stage`, `AI_BRANCH=AI/Stage`, `MAIN_REPO_BRANCH=stage`
   - If `$ARGUMENTS` = `main`: `ORIGINAL_BRANCH=Original/Master`, `AI_BRANCH=AI/Master`, `MAIN_REPO_BRANCH=main`
3. Define paths:
   - `DEV_REPO="/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev"`
   - `MAIN_REPO="/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original"`
4. Ask user: "Have you run `git pull` on branch `<MAIN_REPO_BRANCH>` in Vibecoding-Original? (yes/no)". If no, abort with instructions.
5. Read `$MAIN_REPO/.git/HEAD` to verify Original repo is on the correct branch. If HEAD says `ref: refs/heads/<MAIN_REPO_BRANCH>`, proceed. Otherwise, tell user to switch branches and abort.
6. In Dev repo, save current branch name, then `git checkout $ORIGINAL_BRANCH`.
7. Remove all files in Dev working tree except `.git/` directory.
8. Copy all files from `$MAIN_REPO/` to Dev working tree, excluding `$MAIN_REPO/.git/`.
9. `git add -A`. Check `git status`. If changes exist, `git commit -m "sync: pull from $MAIN_REPO_BRANCH $(date +%Y-%m-%d_%H:%M)"`. If no changes, report "no changes detected".
10. `git checkout $AI_BRANCH`.
11. `git merge $ORIGINAL_BRANCH -m "merge: $ORIGINAL_BRANCH into $AI_BRANCH $(date +%Y-%m-%d_%H:%M)"`. If merge conflicts occur, report them and let user resolve.
12. Report summary: files changed, merge result.

### `.claude/commands/push.md` -- detailed behavior

The skill receives `$ARGUMENTS` which should be `stage` or `main`.

Step-by-step logic:

1. Parse `$ARGUMENTS`. If empty or not `stage`/`main`, print usage and abort.
2. Set variables:
   - If `$ARGUMENTS` = `stage`: `AI_BRANCH=AI/Stage`, `ORIGINAL_BRANCH=Original/Stage`, `MAIN_REPO_BRANCH=stage`
   - If `$ARGUMENTS` = `main`: `AI_BRANCH=AI/Master`, `ORIGINAL_BRANCH=Original/Master`, `MAIN_REPO_BRANCH=main`
3. Define paths (same as pull skill).
4. In Dev repo, verify current branch is `$AI_BRANCH` or switch to it.
5. Read `$MAIN_REPO/.git/HEAD` to verify Original repo is on correct branch. If not, tell user to switch and abort.
6. **Merge AI/ into Original/ inside Dev repo**: `git checkout $ORIGINAL_BRANCH`, then `git merge $AI_BRANCH -m "sync: merge $AI_BRANCH into $ORIGINAL_BRANCH"`. This brings dev changes into the clean branch while the merge automatically excludes AI-only files that were never tracked on Original/ branch. If merge conflicts occur, report and let user resolve, then abort.
7. Remove all files in Original repo working tree except `.git/`.
8. Copy all files from Dev repo `$ORIGINAL_BRANCH` working tree to Original repo, EXCLUDING:
   - `.git/`
   (Note: Original/ branches already have no AI artifacts, so no further exclusion needed. But as a safety net, also exclude these if somehow present:)
   - `.claude/`
   - `CLAUDE.md`
   - `task_plan.md`
   - `errors.md`
   - `INFRA.md`
   - `prompt.md`
   - `REPORT.MD`
9. Switch Dev repo back to `$AI_BRANCH`.
10. Report what files were added/modified/deleted in Original repo.
11. Print reminder: "Now go to Vibecoding-Original and run: `git add -A && git commit -m '<message>' && git push`"

---

## Risks

1. **Risk: Renaming `main` to `AI/Master` breaks the remote tracking branch** -- The remote `origin/main` in Dev repo will no longer match a local branch. Mitigation: After rename, update the remote tracking with `git push -u origin AI/Master` or accept that Dev repo remote tracking diverges (the remote can have both `main` and `AI/Master`). Alternatively, push the new branch and set upstream. This is cosmetic and does not affect local workflow.

2. **Risk: `--allow-unrelated-histories` merge creates conflicts** -- When merging `Original/Master` (orphan) into `AI/Master`, git may see conflicts on `README.md` since both branches have different `README.md` files. Mitigation: During the merge, explicitly resolve by keeping both or choosing one version. The Dev repo README and Original repo README have different content, so a manual resolution choosing the Dev version (or combining) is needed.

3. **Risk: File copy in pull skill misses hidden files or subdirectories** -- Using `cp -r` may behave differently on Windows/Git Bash. Mitigation: Use `rsync -a --delete --exclude='.git'` if available in Git Bash, or use a careful `find`+`cp` approach. Test with subdirectories.

4. **Risk: File deletion in push skill accidentally deletes `.git/` in Original repo** -- If the cleanup step is wrong, it could destroy Original repo's git history. Mitigation: The skill must use `find $MAIN_REPO -maxdepth 1 -not -name '.git' -not -name '.' -exec rm -rf {} +` pattern that explicitly preserves `.git`. This is a critical path that must be tested.

5. **Risk: Windows paths with spaces and parentheses** -- The path `Wildberries(Work)` contains parentheses which can break bash commands. Mitigation: All paths must be properly quoted in bash commands. Use double quotes around all path variables.

6. **Risk: Original repo branch detection fails** -- Reading `.git/HEAD` may return a commit hash (detached HEAD) instead of a branch ref. Mitigation: The skill must handle this case and report an error asking user to checkout a branch.

7. **Risk: Merge conflicts when pulling Original/ into AI/** -- If the same file was modified in both Original (upstream) and AI (dev work), merge conflicts will occur. Mitigation: The skill must detect merge conflict exit code and report it clearly, leaving resolution to the user. Do not `--abort` automatically.

8. **Risk: `rsync` may not be available in Git Bash on Windows** -- Mitigation: Use `cp -r` + explicit file deletion approach instead of relying on rsync. First remove all non-.git files, then copy.

9. **Risk: Skill files not found by Claude Code** -- Claude Code looks for `.md` files in `.claude/commands/` directory. If the directory does not exist or the file naming is wrong, the skills will not appear. Mitigation: Create the `commands` directory explicitly. Verify file is named exactly `pull.md` and `push.md`.

10. **Risk: Pushing to Original repo while it has uncommitted changes** -- The push skill deletes all files in Original working tree before copying. If Original has uncommitted work, it will be lost. Mitigation: The skill should check for uncommitted changes in Original repo by reading `.git/index` status -- however, since we cannot run git in Original repo, we should warn the user to commit/stash first.

---

## Definition of Done

### Branch Structure
- [ ] Dev repo has exactly 4 branches: `AI/Master`, `AI/Stage`, `Original/Master`, `Original/Stage`
- [ ] `Original/Master` contains ONLY: `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md` (no AI artifacts)
- [ ] `Original/Stage` contains ONLY: `factorial.py`, `fibonacci.py`, `palindrome.py`, `README.md` (no AI artifacts)
- [ ] `AI/Master` contains: all Original files PLUS `.claude/`, `CLAUDE.md`, `README.md`, `prompt.md`, and the new skill files
- [ ] `AI/Stage` contains: same content as `AI/Master` (after initial merge propagation)

### Pull Skill (`.claude/commands/pull.md`)
- [ ] File exists at `.claude/commands/pull.md`
- [ ] Accepts `$ARGUMENTS` of `stage` or `main`; rejects invalid input with usage message
- [ ] Forces user confirmation that `git pull` was run in Original repo
- [ ] Verifies Original repo is on the correct branch by reading `.git/HEAD`
- [ ] Correctly maps `main` -> `Original/Master` + `AI/Master` and `stage` -> `Original/Stage` + `AI/Stage`
- [ ] Syncs files from Original repo into `Original/<branch>` in Dev repo (handles additions, edits, deletions)
- [ ] Commits changes on `Original/<branch>` with descriptive message (or skips if no changes)
- [ ] Merges `Original/<branch>` into `AI/<branch>` using `git merge`
- [ ] Reports merge conflicts if they occur without auto-aborting
- [ ] All paths with spaces/parentheses are properly quoted

### Push Skill (`.claude/commands/push.md`)
- [ ] File exists at `.claude/commands/push.md`
- [ ] Accepts `$ARGUMENTS` of `stage` or `main`; rejects invalid input with usage message
- [ ] Verifies Original repo is on the correct branch by reading `.git/HEAD`
- [ ] Correctly maps `main` -> `AI/Master` + `Original/Master` and `stage` -> `AI/Stage` + `Original/Stage`
- [ ] **Merges AI/ branch into Original/ branch in Dev repo** before copying to main repo
- [ ] Copies files from Dev repo **Original/ branch** (not AI/ branch) to Original repo working tree (handles additions, edits, deletions)
- [ ] Safety-net excludes AI artifacts: `.claude/`, `CLAUDE.md`, `task_plan.md`, `errors.md`, `INFRA.md`, `prompt.md`, `REPORT.MD`
- [ ] Does NOT delete `.git/` in Original repo
- [ ] Switches Dev repo back to AI/ branch after push
- [ ] Reminds user to manually commit and push in Original repo
- [ ] All paths with spaces/parentheses are properly quoted

### CLAUDE.md Update
- [ ] `CLAUDE.md` contains new `## Two-Repo Workflow` section
- [ ] Section documents: both repos and their paths, all 4 branch names and their purposes, branch mapping, git pipeline steps, AI artifacts exclusion list
- [ ] Existing content in `CLAUDE.md` (Project Overview, Agent Workflow, Key Rules) is preserved unchanged

### General
- [ ] No git commands are ever executed against Vibecoding-Original in any skill (only file reads/copies)
- [ ] Dev repo is left on `AI/Master` branch after all setup is complete
- [ ] All files have no comments in code (documentation in the skill prompt bodies is acceptable since they are markdown instruction files, not code)
