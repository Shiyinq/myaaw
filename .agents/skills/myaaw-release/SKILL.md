---
name: myaaw-release
description: 'Cut GitHub releases for the Myaaw repo (Shiyinq/myaaw): choose the vYYYY.M.D tag, confirm the dev→main PR is already merged, tag the merge commit on main, monitor the Release Build workflow, rename the release to "Myaaw v<tag>", and verify the auto-generated notes and assets. Use only after the pull request is merged; PR work belongs to myaaw-pull-requests.'
---

# Myaaw releases and tags

Release work starts **only after** the pull request is merged. Creating or editing the PR belongs to the `myaaw-pull-requests` skill.

## Repository facts

| Item | Value |
| :--- | :--- |
| Remote | `Shiyinq/myaaw` |
| Long-lived branches | `main` (releases) and `dev` (integration) |
| Release workflow | `.github/workflows/release.yml`, triggered by pushing a `v*` tag |
| Build targets | `myaaw-linux-amd64`, `myaaw-darwin-arm64` |
| Release creator | the workflow (`softprops/action-gh-release@v2` with `generate_release_notes: true`) |
| Existing tag examples | `v2026.8.9`, `v2026.8.9.1`, `v2026.8.9.2`, `v2026.8.10` |

## Hard gate: the PR must be merged first

Run this before touching tags. Stop at the first failure.

1. Read the PR state:
   ```bash
   gh pr view <number> --json state,createdAt,mergedAt,mergedBy,autoMergeRequest,mergeCommit
   ```
2. `state` must be `MERGED`. If it is `OPEN` or `CLOSED`, **stop**: do not tag and do not create a release.
3. Do not merge the PR yourself, do not enable auto-merge, and do not assume a merge happened. A manual merge shows `mergedBy` as the maintainer account and `autoMergeRequest` as `null`.
4. `origin/main` must contain that merge commit:
   ```bash
   git fetch origin main --tags
   git --no-pager log --oneline -3 origin/main
   git merge-base --is-ancestor <merge-commit> origin/main && echo "on main"
   ```
5. Only then create the tag.

Never tag `dev` HEAD, and never create the tag/release first and merge later. A tag on an unmerged commit cannot be moved without rewriting published history.

## Tag format

- `v<YYYY>.<M>.<D>` from the release date, **without zero padding** (`v2026.8.9`, `v2026.10.30`).
- Every additional release on the same day gets a numeric suffix: `v2026.8.9.1`, `v2026.8.9.2`.
- Confirm the name is free before tagging:
  ```bash
  git ls-remote --tags origin | grep "v2026.10" || echo "free"
  ```
- Tags are **lightweight** and point at the **merge commit on `main`**:
  ```bash
  git tag v2026.10.30 <merge-commit>
  git push origin v2026.10.30
  ```

## What the tag triggers

Pushing the tag starts `Release Build`. The workflow builds both targets and creates the GitHub Release with **auto-generated notes**. Do not hand-write release notes and do not create the release manually.

```bash
gh run list --workflow release.yml --limit 3
gh run view <run-id> --json status,conclusion,jobs
gh run watch <run-id> --exit-status   # may take a few minutes
```

## Release title and body

The workflow names the release with the bare tag, so rename it to match every previous release:

```bash
gh release edit v2026.10.30 --title "Myaaw v2026.10.30"
```

Target format:

- Name: `Myaaw v<tag>`
- Body (auto-generated, never hand-written):
  ```markdown
  ## What's Changed
  * <PR title> by @Shiyinq in https://github.com/Shiyinq/myaaw/pull/<n>

  **Full Changelog**: https://github.com/Shiyinq/myaaw/compare/<previous-tag>...<new-tag>
  ```
- Assets: `myaaw-linux-amd64`, `myaaw-darwin-arm64`
- Not a draft, not a prerelease, marked **Latest**

## Verify

```bash
gh release list --limit 5
gh release view v2026.10.30 --json name,tagName,isDraft,isPrerelease,body,assets \
  --jq '{name,tagName,isDraft,isPrerelease,body,assets:[.assets[].name]}'
```

Check that `name` starts with `Myaaw `, `body` contains `## What's Changed` and a `**Full Changelog**` link, both assets are present, and the release shows as `Latest`.

## Guardrails

- Never merge, approve, or auto-merge a pull request. Only the maintainer merges.
- Never create a tag or release while the PR is still open, and never tag a commit missing from `origin/main`.
- Never tag `dev` HEAD, and never force-push or delete a published tag. Cut the next suffix (`...N.1`) instead.
- Never rewrite release notes manually; the workflow's generated notes are the source of truth.
- Do not push tags or publish releases unless the user asks.
- After a release, remind the user to restart `myaaw api` and every agent daemon; config and tool lists are read at startup.

## Troubleshooting

| Symptom | Action |
| :--- | :--- |
| Workflow failed | `gh run view <id> --log-failed`, fix on `dev`, then cut the next `.N` tag |
| Release named with the bare tag | `gh release edit <tag> --title "Myaaw <tag>"` |
| Tag already exists on origin | Pick the next suffix (`v2026.10.30.1`) |
| Notes list older PRs | Expected: GitHub compares against the previous release tag |
| `gh release view` says "release not found" | The workflow has not reached the release job yet; check the run |
| `main` lacks the merge commit | Fetch first; the PR may still be open. Do not tag until it is merged |
| PR is still `OPEN` when a release is requested | Stop and report it. Tagging now would publish an unmerged commit |
| Someone enabled auto-merge | Do not rely on it: wait until `state` is `MERGED` and the merge commit is on `origin/main` |
