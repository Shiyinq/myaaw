---
name: myaaw-pull-requests
description: Open or update the dev→main pull request for the Myaaw repo (Shiyinq/myaaw). Use when writing or editing the PR title and English description, collecting the diff since main, checking test results, and verifying the PR body. This skill never merges; releases are handled by the myaaw-release skill.
---

# Myaaw pull requests

Applies to the integration branch `dev` and the release branch `main` in `Shiyinq/myaaw`.

## Boundaries

- This skill ends when the PR is created or updated.
- Never merge, approve, or enable auto-merge. The maintainer merges manually.
- Tagging and publishing belong to the `myaaw-release` skill, and only run after the PR is `MERGED`.

## Title

- English, sentence case, descriptive.
- **No** conventional-commit prefix (`feat:`, `fix:`, `chore:`, …) and no trailing `...`.
- Name the whole scope, not one sub-feature. Example:
  `Agent loop v2, multi-agent runtime, A2A, control plane API, approvals, observability, and computer use`
- Keep it under roughly 120 characters.

## Description

Cover **every commit from the last `main` commit to the branch tip**, not just the newest batch.

1. Collect the facts first:
   ```bash
   git fetch origin main --tags
   git rev-list --count origin/main..dev
   git --no-pager diff --shortstat origin/main...dev
   git --no-pager log --oneline origin/main..dev
   git --no-pager log --merges --oneline origin/main..dev
   ```
2. Read `docs/README.md` and skim the relevant guides so each area summary matches the code.
3. Start from `templates/pr-body.md`. Required sections:
   - `## Summary` with the scope numbers (commits, files, additions/deletions).
   - One section per area: agent core, multi-agent, A2A, control plane, onboarding, backup, skills, providers, channels, approvals, observability, computer use, vision, CLI/tooling, docs.
   - `## Configuration and migration`
   - `## Breaking and behavior changes` (numbered)
   - `## Testing` (only what you actually ran)
   - `## Known limitations`
4. If `dev` diverged from `main` (common after a rewrite), say so and tell reviewers to read by area instead of by commit.
5. Run the validation you claim:
   ```bash
   go build ./... && go vet ./... && go test ./...
   ```

## Create or update

```bash
# create (base main, head dev)
gh pr create --base main --head dev --title "<title>" --body-file .pr_body.md

# update
gh pr view <number> --json state
gh pr edit <number> --title "<title>" --body-file .pr_body.md

# verify
gh pr view <number> --json state,title,baseRefName,headRefName,body \
  --jq '.state, .title, "\(.headRefName)→\(.baseRefName)", (.body | length)'
```

Write the body to `.pr_body.md` in the repo root, use it, then delete it. Do not leave the temp file behind.

## Guardrails

- Do not create a PR unless the user asks.
- Do not merge, approve, auto-merge, or assume a merge happened.
- Do not claim a test passed unless you ran it.
- Do not leave `.pr_body.md` behind.
- Keep only `dev` and `main` on `origin`; delete merged feature branches, never the long-lived ones.

## Handoff to releases

A release needs a merged PR. Check state before doing anything else:

```bash
gh pr view <number> --json state,mergedAt,mergedBy,autoMergeRequest,mergeCommit
```

If `state` is `OPEN`, stop and tell the user the PR is waiting to be merged. If it is `MERGED`, switch to the `myaaw-release` skill.
