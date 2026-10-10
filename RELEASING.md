# Releasing

Releases are prepared by an agent, reviewed by a human, and published from a clean macOS checkout of the default branch.

## Prepare the changelog

Ask an agent to prepare `vX.Y.Z`. The agent must start from the repository evidence:

```bash
make changelog-context VERSION=vX.Y.Z
```

The agent updates only the new top section of `CHANGELOG.md` and must:

- describe user-visible outcomes rather than copy commit subjects;
- group related implementation commits into one useful bullet;
- use clear headings such as `Added`, `Changed`, `Fixed`, or `Removed` when they help;
- link every bullet to its verified merged GitHub pull request, or to a GitHub commit when no pull request exists;
- call out breaking changes explicitly;
- preserve all existing release sections.

Use commit messages, changed-file evidence, and PR metadata to understand impact. Never infer a PR association without evidence. Review the generated section, then commit it before running release checks.

## Validate and publish

For a standalone readiness assessment, run `make release-check VERSION=vX.Y.Z`.
To prepare and publish a release, run these commands in order:

```bash
make release-dry-run VERSION=vX.Y.Z
make release VERSION=vX.Y.Z
```

`release-check` validates the clean worktree, version, changelog, tests, documentation, module metadata, formatting, and version-stamped binary. `release-dry-run` builds both macOS archives and checksums, extracts the approved changelog section as release notes, and renders the Homebrew formula without remote writes.

Both dry-run and publish invoke `scripts/release-check.sh` first, after argument parsing, as the shared readiness gate. Publish then requires `main`; dry-run permits other branches with a warning. The execution script rechecks tag conflicts after building, before creating a tag or reporting the dry-run plan.

For CI validation, use `make release-check-ci` (or `scripts/release-check.sh --ci`). It validates the top changelog version even when that tag exists; release mode additionally requires a new tag and traceable changelog bullets.

The automatic `ci` workflow owns PR and push validation: it runs this gate once, followed by the focused lifecycle race tests and snapshot benchmarks. The separate `release-check` workflow runs only on manual dispatch. The gate's full package test run includes script tests; `make test-scripts` remains available for focused local iteration.

The final command creates and pushes the tag, publishes the GitHub Release with the approved changelog section, and updates the configured Homebrew tap.

`HOMEBREW_TAP_BRANCH` selects the existing tap branch to check out and update (default: `main`). The formula commit is based on that branch, even when the tap's default branch differs.

## Changelog policy

- Keep concrete release headings in the form `## [vX.Y.Z] - YYYY-MM-DD`.
- Do not add an `Unreleased` section.
- Treat the reviewed changelog section as the source of truth for GitHub release notes.
