# Push Skill: Sync from Vibecoding-Dev AI branch into Vibecoding-Original

You are syncing changes FROM Vibecoding-Dev AI branch INTO Vibecoding-Original. Follow every step in order. Do not skip steps. Never run git commands inside Vibecoding-Original.

## Input

`$ARGUMENTS` is the branch name to sync. It must be either `stage` or `main`.

## Step 1: Validate input

If `$ARGUMENTS` is empty, not `stage`, and not `main`, print the following and stop immediately:

```
Usage: /push <branch>
  branch: "main" or "stage"

  main  -> merges AI/Master into Original/Master, then copies to Vibecoding-Original main
  stage -> merges AI/Stage into Original/Stage, then copies to Vibecoding-Original stage
```

## Step 2: Set variables based on branch

If `$ARGUMENTS` is `main`:
- `AI_BRANCH` = `AI/Master`
- `ORIGINAL_BRANCH` = `Original/Master`
- `MAIN_REPO_BRANCH` = `main`
- `OTHER_AI_BRANCH` = `AI/Stage`

If `$ARGUMENTS` is `stage`:
- `AI_BRANCH` = `AI/Stage`
- `ORIGINAL_BRANCH` = `Original/Stage`
- `MAIN_REPO_BRANCH` = `stage`
- `OTHER_AI_BRANCH` = `AI/Master`

Paths (always use these exact quoted values in bash commands):
- `DEV_REPO` = `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev`
- `MAIN_REPO` = `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original`

## Step 3: Verify Dev repo is on AI/<branch>

Check the current branch in Dev repo:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git branch --show-current
```

If not on `<AI_BRANCH>`, switch to it:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH>
```

If switching fails (uncommitted changes), report the error and stop:
```
Cannot switch to <AI_BRANCH>. You have uncommitted changes in Dev repo.
Please commit or stash them first, then run /push <ARGUMENTS> again.
```

## Step 4: Verify Original repo is on the correct branch

Read the file `$MAIN_REPO/.git/HEAD` using the Read tool.

The file content will be one of:
- `ref: refs/heads/<branch-name>` — repo is on a branch
- A commit hash — repo is in detached HEAD state

If the branch name in HEAD matches `<MAIN_REPO_BRANCH>`, proceed.

If HEAD is a commit hash (detached), print:
```
Vibecoding-Original is in detached HEAD state. Please run:
  cd <MAIN_REPO>
  git checkout <MAIN_REPO_BRANCH>
```
and stop.

If HEAD points to a different branch, print:
```
Vibecoding-Original is on branch '<actual-branch>', but you requested '<MAIN_REPO_BRANCH>'.
Please go to Vibecoding-Original and run:
  git checkout <MAIN_REPO_BRANCH>
Then run /push <ARGUMENTS> again.
```
and stop.

## Step 5: Check for uncommitted changes in Original repo

Since you cannot run git in Original repo, warn the user:

Print:
```
WARNING: Make sure Vibecoding-Original has no uncommitted work you want to keep.
The push operation will overwrite all files in that repo's working tree.
```

Then ask: "Does Vibecoding-Original have any uncommitted changes that should be preserved? (yes/no)"

If the user says "yes", print:
```
Aborting. Please commit or stash changes in Vibecoding-Original first.
```
and stop.

## Step 6: Merge AI/<branch> into Original/<branch> inside Dev repo

This step propagates your development work onto the clean Original mirror branch.
AI artifacts must NEVER exist on Original/ branches.

Checkout the Original branch in Dev repo:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <ORIGINAL_BRANCH>
```

Merge the AI branch into it:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git merge <AI_BRANCH> -m "sync: merge <AI_BRANCH> into <ORIGINAL_BRANCH>"
```

If the merge exits with a non-zero code (conflicts), print:
```
Merge conflict detected when merging <AI_BRANCH> into <ORIGINAL_BRANCH>.
Conflicting files:
  <list of conflicting files>

Please resolve the conflicts in Dev repo, then run:
  git add <resolved-files>
  git commit

Then run /push <ARGUMENTS> again.
```
Do NOT abort the merge automatically. Stop here and let the user resolve.

## Step 6b: Strip AI artifacts from Original/<branch>

The merge in Step 6 brought AI artifacts onto the Original branch. Remove them immediately so Original/ branches stay clean. Run:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && rm -rf .claude CLAUDE.md task_plan.md errors.md INFRA.md prompt.md REPORT.MD
```

Stage and commit only if there are changes:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git add -A && (git diff --cached --quiet || git commit -m "sync: strip AI artifacts from <ORIGINAL_BRANCH>")
```

## Step 7: Copy files from Dev Original/<branch> to Vibecoding-Original

First, delete all files in Vibecoding-Original except `.git/`. Use this exact command:
```bash
find "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original" -maxdepth 1 -mindepth 1 -not -name '.git' -exec rm -rf {} +
```

Then copy all files from Dev repo working tree (currently on `<ORIGINAL_BRANCH>`) to Vibecoding-Original, excluding `.git/` and all AI artifacts. The AI-artifact exclusion is a safety net — the Original/ branch should already contain no AI artifacts, but exclude them anyway:

```bash
find "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" -maxdepth 1 -mindepth 1 \
  -not -name '.git' \
  -not -name '.claude' \
  -not -name 'CLAUDE.md' \
  -not -name 'task_plan.md' \
  -not -name 'errors.md' \
  -not -name 'INFRA.md' \
  -not -name 'prompt.md' \
  -not -name 'REPORT.MD' \
  | while read src; do
      filename=$(basename "$src")
      cp -r "$src" "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original/$filename"
    done
```

## Step 8: Switch Dev repo back to AI/<branch>

```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH>
```

## Step 8b: Sync AI artifacts to <OTHER_AI_BRANCH>

This step keeps AI artifacts (except CLAUDE.md) identical across both AI/ branches.

First, try to checkout the other AI branch:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <OTHER_AI_BRANCH>
```

If checkout fails (uncommitted changes would be overwritten), print:
```
WARNING: Could not switch to <OTHER_AI_BRANCH> — it may have uncommitted changes.
Skipping AI artifact sync to <OTHER_AI_BRANCH>.
Please commit or stash changes on <OTHER_AI_BRANCH>, then manually sync AI artifacts.
```
Skip to Step 9.

If checkout succeeds, check for uncommitted changes:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git status --porcelain
```

If the output is NOT empty (there are uncommitted changes), print:
```
WARNING: <OTHER_AI_BRANCH> has uncommitted changes. Skipping AI artifact sync.
Please commit or stash changes on <OTHER_AI_BRANCH> first.
```
Switch back to AI_BRANCH and skip to Step 9:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH>
```

If clean, copy AI artifacts from `<AI_BRANCH>` (excluding CLAUDE.md) into the working tree:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH> -- .claude task_plan.md errors.md INFRA.md prompt.md REPORT.MD 2>/dev/null; true
```

Stage and commit only if there are changes:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git add -A && (git diff --cached --quiet || git commit -m "sync: AI artifacts from <AI_BRANCH> into <OTHER_AI_BRANCH>")
```

Switch back to `<AI_BRANCH>`:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH>
```

## Step 9: Report results and remind user

List what changed in Vibecoding-Original by comparing with git (you cannot run git in that repo, so list the files you copied):

Print a summary:
```
Push complete for branch: <ARGUMENTS>

  Dev AI branch        : <AI_BRANCH>
  Dev Original branch  : <ORIGINAL_BRANCH>  (AI/<branch> merged in, AI artifacts stripped)
  Other AI branch      : <OTHER_AI_BRANCH>  (AI artifacts synced / skipped — state why)
  Vibecoding-Original  : files updated on branch <MAIN_REPO_BRANCH>

Files copied to Vibecoding-Original:
  <list each file/directory you copied>

AI artifacts synced to <OTHER_AI_BRANCH>:
  <list each AI artifact synced, or "skipped — <reason>">

Dev repo is now back on: <AI_BRANCH>

ACTION REQUIRED — go to Vibecoding-Original and run:
  git add -A
  git commit -m "your commit message here"
  git push
```
