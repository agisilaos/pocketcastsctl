# Agent sessions

One chat owns one task in its own worktree and branch. Use GitHub Issues to
track work; a small task can start from the user's request.
Report findings before editing on a review or investigation unless implementation
is already authorized. Commit when asked; land when asked. Permission to commit
does not by itself authorize a push.

## Start

Run `scripts/session start <name>` from an existing checkout. It fetches
`origin/main`, creates a sibling worktree on `fix/<name>`, and prints its path.
Open the fresh chat in that worktree. Use `git worktree list` first if the task
might already have an owner. For a clean worktree already created by Codex,
reuse it with `git fetch origin main` and
`git switch --no-track -c fix/<name> origin/main`.

For a new chat, give it a brief it can act on without reading the old one:

> - **Task and issue:** the outcome and the issue link, if any.
> - **Checkout:** the worktree path and starting commit.
> - **Context:** decisions already made, evidence, and what is out of scope.
> - **Done when:** observable behavior and how to check it.
> - **Authorization:** investigate, implement, commit, or land as requested.

## Work

Reproduce the behavior where possible and run the smallest relevant checks in
[the developer docs](../README.md#testing-notes). For a CLI behavior change,
exercise the actual command and check its output and exit status. Separate
fixture results from live Pocket Casts or browser evidence. Save logs and
captures in `~/Library/Logs/pocketcastsctl/threads/<name>/`.

Before committing, inspect your diff for unrelated changes, check the docs it
affects, and stage explicit paths. Preserve other sessions' edits; save a patch
outside the checkout if you need to set work aside, since Git stash is shared.

## Land

Once landing is authorized, fetch and rebase the committed work on the latest
`origin/main`. Review the committed range against the task and repository rules.
Get an independent review for CLI behavior and delivery tooling changes. Address
actionable findings and review again if the base or commit changes.

```sh
git fetch origin main
git rebase origin/main
base=$(git rev-parse origin/main)
reviewed=$(git rev-parse HEAD)
git log --oneline "$base..$reviewed"
git diff --check "$base..$reviewed"
# Review this exact range before running:
scripts/session land --base "$base" --reviewed "$reviewed"
```

`land` requires a clean session and the reviewed range. It takes a landing turn
shared by worktrees of this clone, checks that `main` is still the reviewed base,
and runs the same validation and race/benchmark targets as CI. The log and
stamped build go in a new evidence subdirectory. It then rechecks the checkout
and remote tip and pushes the exact SHA to `main` with a regular fast-forward
push. Use `--dry-run` to exercise the gate and push negotiation without updating
the remote.

If a check fails, read the log and fix it before trying again. If `main` moves,
fetch, rebase, and review the updated range. Separate clones do not share the
lock; Git also rejects a push that would discard remote commits. Verify that
`origin/main` contains the landed commit and check the GitHub CI run. Landing
does not publish a release.

## Close

Record the landed SHA, verification, and any outstanding work in the handoff.
Put lasting workflow lessons in the docs and track deferred work in GitHub when
authorized. After checking the worktree is clean and its branch is on
`origin/main`, remove that worktree from the primary checkout with
`git worktree remove <path>`. Delete the local branch with `git branch -d` once
Git sees it as merged. Leave the session in place if it still holds work.
