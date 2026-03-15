# Pull Skill: Sync from Vibecoding-Original into Vibecoding-Dev

You are syncing changes FROM Vibecoding-Original INTO Vibecoding-Dev. Follow every step in order. Do not skip steps.

## Input

`$ARGUMENTS` is the branch name to sync. It must be either `stage` or `main`.

## Step 1: Validate input

If `$ARGUMENTS` is empty, not `stage`, and not `main`, print the following and stop immediately:

```
Usage: /pull <branch>
  branch: "main" or "stage"

  main  -> syncs Original/Master and AI/Master
  stage -> syncs Original/Stage and AI/Stage
```

## Step 2: Set variables based on branch

If `$ARGUMENTS` is `main`:
- `ORIGINAL_BRANCH` = `Original/Master`
- `AI_BRANCH` = `AI/Master`
- `MAIN_REPO_BRANCH` = `main`

If `$ARGUMENTS` is `stage`:
- `ORIGINAL_BRANCH` = `Original/Stage`
- `AI_BRANCH` = `AI/Stage`
- `MAIN_REPO_BRANCH` = `stage`

Paths (always use these exact quoted values in bash commands):
- `DEV_REPO` = `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev`
- `MAIN_REPO` = `/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original`

## Step 3: Confirm git pull was run

Ask the user this question and wait for their response before proceeding:

"Have you already run `git pull` on branch `<MAIN_REPO_BRANCH>` inside Vibecoding-Original? (yes/no)"

If the user says anything other than "yes", print:

```
Aborting. Please go to Vibecoding-Original and run:
  git checkout <MAIN_REPO_BRANCH>
  git pull
Then run /pull <ARGUMENTS> again.
```

and stop.

## Step 4: Verify Original repo is on the correct branch

Read the file `$MAIN_REPO/.git/HEAD` using the Read tool.

The file content will be one of:
- `ref: refs/heads/<branch-name>` — this means the repo is on a branch
- A commit hash — this means the repo is in detached HEAD state

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
Then run /pull <ARGUMENTS> again.
```
and stop.

## Step 5: Checkout Original/<branch> in Dev repo

Run:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <ORIGINAL_BRANCH>
```

## Step 6: Remove all tracked files from Dev working tree

Remove every file that is tracked in the current branch, leaving only `.git/`. Run:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git ls-files | xargs rm -f
```

## Step 7: Copy all files from Vibecoding-Original into Dev repo

Copy every file and directory from `$MAIN_REPO` into `$DEV_REPO`, excluding `.git/`. Run:
```bash
cp -r "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original/." "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/" && rm -rf "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/.git"
```

Note: The `.git` directory of Dev repo was restored by the last command but we must NOT have deleted the Dev repo's `.git`. Use this safer approach instead:

```bash
find "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Original" -maxdepth 1 -not -name '.git' -not -name '.' | while read src; do
  filename=$(basename "$src")
  cp -r "$src" "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev/$filename"
done
```

## Step 8: Stage and commit changes in Dev repo

Run:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git add -A && git status
```

Check if there are any staged changes. If `git status` shows "nothing to commit", report "No changes detected in Original repo — nothing to commit on <ORIGINAL_BRANCH>." and skip the commit.

If there are changes, commit:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git commit -m "sync: pull from <MAIN_REPO_BRANCH> $(date +%Y-%m-%d_%H:%M)"
```

## Step 9: Checkout AI/<branch> and merge

Run:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git checkout <AI_BRANCH>
```

Then merge:
```bash
cd "/c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev" && git merge <ORIGINAL_BRANCH> -m "merge: <ORIGINAL_BRANCH> into <AI_BRANCH> $(date +%Y-%m-%d_%H:%M)"
```

If the merge exits with a non-zero code (conflicts), report all conflicting files and print:
```
Merge conflicts detected. Please resolve them manually in:
  /c/Users/Wildberries(Work)/Desktop/VBGuide/Vibecoding-Dev

Then run:
  git add <resolved-files>
  git commit

Do NOT run git merge --abort unless you want to discard the sync.
```
Do not abort the merge automatically.

## Step 10: Report results

Print a summary:
```
Pull complete for branch: <ARGUMENTS>

  Original repo branch : <MAIN_REPO_BRANCH>
  Dev Original branch  : <ORIGINAL_BRANCH>  (synced)
  Dev AI branch        : <AI_BRANCH>         (merged)

Files synced: <list files added/modified/deleted from git diff>
Merge result: <clean / conflicts in: file1 file2 ...>
```
