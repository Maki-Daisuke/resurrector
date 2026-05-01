---
name: resurrector-release
description: End-to-end Resurrector release workflow. Use when releasing a new Resurrector version (e.g. v1.1.4): bump release metadata in package.json / ui/frontend/package.json / ui/wails.json, commit, create and push the git tag, wait for GitHub Actions to publish the release ZIP, capture the installer URL and SHA256 from the GitHub Release API (never by downloading), update / validate / submit the WinGet manifest under manifests/y/Yanother/Resurrector/, and publish GitHub release notes.
---

# Resurrector Release

You are the release operator for Resurrector. Execute the end-to-end release workflow safely and deterministically.

## Scope

- Inputs from user:
  - Target version tag like `v1.1.0`
- Responsibilities:
  - Update version metadata files for the target release.
  - Commit version changes.
  - Create and push the git tag.
  - Wait for GitHub Actions to publish the release ZIP.
  - Resolve ZIP URL and SHA256 **from the GitHub Release API/page only** (never by downloading and hashing locally).
  - Create/update WinGet manifest files under `manifests/` for the same version, **strictly after** the release asset and its SHA256 are available.
  - Run WinGet manifest validation and submission.
  - Draft and **publish** GitHub release notes after explicit user confirmation.

## Required Guardrails

Group the rules below by category. Each category lists the only constraints that apply to it; you do not need to remember rules across categories.

### Version handling

- Never guess a version. Parse and validate semver from user input (`vX.Y.Z` or `X.Y.Z`).

### Working tree & git operations

- Refuse to continue if the working tree has unrelated dirty changes that affect release safety.
- Use non-interactive commands only.
- Never use destructive git commands like `git reset --hard`.

### WinGet manifest ordering

- Do not rename, edit, or create any file under `manifests/y/Yanother/Resurrector/` until Step 5 has produced both the `InstallerUrl` and the `InstallerSha256` for the new tag. The Step 6 `git mv` rename and edits are then permitted as the _first_ manifest changes, never before. If you feel tempted to "prepare manifests ahead of time," stop.

### Installer SHA256 sourcing

- The agent itself must never compute the installer SHA256 by downloading the ZIP and hashing it. The hash MUST come from the GitHub Release asset metadata via Step 5's polling script. Local downloads can race with re-uploads and produce stale hashes.
- If the user explicitly instructs the agent to fall back to local hashing (because the script cannot recover for some external reason), that user-issued override takes precedence; record it in the conversation and proceed. Without such an explicit user instruction, do not local-hash.

### Confirmations before irreversible steps

Ask the user for an explicit confirmation **separately for each** of the following actions; one confirmation does not cover the others:

- Pushing the release commit
- Pushing the tag
- Submitting the WinGet manifest
- Publishing release notes (overwrites the GitHub Release body)

## Workflow

1. Validate input and normalize versions.

- Accept `vX.Y.Z` or `X.Y.Z`.
- Derive:
  - `TAG=vX.Y.Z`
  - `VERSION=X.Y.Z`

2. Preflight checks.

- Confirm current branch and clean status for release files.
- Verify expected files exist:
  - `package.json`
  - `ui/frontend/package.json`
  - `ui/wails.json`
  - `.github/workflows/release.yml`
  - `manifests/`
- Ensure `gh` and git are available.

3. Update local release metadata.

- Bump `version` in:
  - `package.json`
  - `ui/frontend/package.json`
- Bump `info.productVersion` in:
  - `ui/wails.json`
- Do NOT touch other version-like strings unless they are in the three files above. In particular, leave alone:
  - `core/version.go` and `ui/version.go` (`Version = "dev"` is overwritten at build time via `-ldflags`)
  - `pnpm-lock.yaml` and other lockfiles (transitive dependency versions, unrelated)
  - Documentation/CHANGELOG-style references to historical versions
- Show exact file diffs before commit.

4. Commit and tag.

- Commit message format:
  - ` chore: bump version to vX.Y.Z`
- Create annotated tag `vX.Y.Z`.
- Push commit and tag to origin after confirmation.

5. Wait for GitHub Release asset and capture URL + SHA256.

- Do not start this step until the tag has been pushed in Step 4.
- **Always use the bundled polling script** [`scripts/wait-release.ps1`](scripts/wait-release.ps1) (relative to this skill directory) to wait for the asset and resolve metadata. Do not implement ad-hoc waits, downloads, or hash calculations inline.
  - Invocation (run from the repo root):
    ```pwsh
    pwsh -NoProfile -File .agents/skills/resurrector-release/scripts/wait-release.ps1 -Tag vX.Y.Z
    ```
  - Optional flags: `-TimeoutSeconds`, `-IntervalSeconds`, `-AssetPattern`, `-Repo`.
  - The script polls `gh api repos/<owner>/<repo>/releases/tags/<tag>` until the
    `resurrector-vX.Y.Z-windows-amd64.zip` asset exists **and** its `digest`
    field is published, then prints a single JSON line to stdout:
    ```json
    {
      "tag": "vX.Y.Z",
      "assetName": "...",
      "url": "...",
      "sha256": "<UPPERCASE_HEX>"
    }
    ```
  - Parse that JSON: `url` → InstallerUrl, `sha256` → InstallerSha256.
- The user may explicitly notify that the ZIP is ready; treat that as a hint to run the script, not as a substitute for running it.
- **Do NOT download the ZIP to compute the hash locally.** The script intentionally relies on GitHub's published `digest`. If the script exits non-zero (timeout, missing digest, etc.), stop and ask the user; do not fall back to local hashing without explicit approval.
- **Handling delays or failures:** if the script exits non-zero, do not silently retry forever. Diagnose first:
  - Timeout (deadline reached, no asset uploaded yet): inspect the release workflow with `gh run list --workflow release.yml --branch main --limit 5` and `gh run view <run-id>`. Report the failing step to the user and ask whether to wait longer (re-run the script with a higher `-TimeoutSeconds`) or to abort and fix the workflow first.
  - Asset uploaded but `digest` missing: this is unusual; wait one or two more polling intervals manually, then if still missing, report to the user and ask for instructions (per the Installer SHA256 sourcing rule, do not local-hash unless the user explicitly tells you to).
  - `gh` auth or network errors: surface the exact error to the user; do not improvise credentials.
- Record both values; they are the only inputs allowed for the manifest update in Step 6.

6. Update WinGet manifest files (only after Step 5 succeeded).

- Preconditions (verify before any file change):
  - Tag `vX.Y.Z` is pushed.
  - GitHub Release for `vX.Y.Z` exists and contains the windows-amd64 ZIP asset.
  - `InstallerUrl` and `InstallerSha256` from Step 5 are both in hand.
- If any precondition fails, stop. Do not pre-rename directories or pre-edit manifests "to save time."
- Reuse previous manifest directory by renaming it with git history preserved:
  - Find previous version directory under `manifests/y/Yanother/Resurrector/`.
  - Rename with `git mv` from previous version to `X.Y.Z`.
  - Example: `git mv manifests/y/Yanother/Resurrector/A.B.C manifests/y/Yanother/Resurrector/X.Y.Z`
- If previous version directory cannot be identified, stop and ask user before creating anything manually.
- Ensure these files exist and are updated consistently:
  - `Yanother.Resurrector.yaml`
  - `Yanother.Resurrector.locale.en-US.yaml`
  - `Yanother.Resurrector.installer.yaml`
- Update fields:
  - `PackageVersion: X.Y.Z` in all files
  - `InstallerUrl` with `vX.Y.Z` path and matching ZIP file name
  - `InstallerSha256` with the SHA256 value from GitHub Release

7. Validate and submit WinGet manifest.

- Run these commands:
  - `winget validate --manifest <manifest-file-or-directory>`
  - `wingetcreate submit <manifest-file-or-directory>`
- If tools are missing, stop and provide exact install commands, then continue after user confirmation.
- Submit after explicit confirmation.

8. Draft and publish release notes.

- This step is mandatory and must not be skipped. The release is not considered done until the GitHub Release body is updated.
- Determine previous tag before `vX.Y.Z` (e.g. via `git tag --sort=-v:refname` or git history).
- Collect commits between previous tag and `vX.Y.Z` with `git log <prev>..vX.Y.Z`.
- Draft GitHub release note text with:
  - Highlights / Features
  - Fixes and improvements
  - Documentation
  - Internal / tooling (optional)
  - Breaking changes (if any)
  - Full changelog link: `https://github.com/Maki-Daisuke/resurrector/compare/<prev>...vX.Y.Z`
- Show the draft to the user and ask for confirmation.
- After confirmation, publish with:
  - `gh release edit vX.Y.Z --notes-file <path>`
- Verify the release page reflects the new body before reporting completion.

9. Report results.

- Return concise summary:
  - Updated files
  - Commit SHA
  - Tag
  - Release asset URL
  - SHA256
  - Validation result
  - Submission result / PR URL
  - Release notes URL (the published GitHub Release page)

## Output Format

Use this exact structure:

- Release: `vX.Y.Z`
- Commit: `<sha>`
- Tag: `<tag>`
- Asset URL: `<url>`
- Asset SHA256: `<sha256>`
- Updated files:
  - `<path>`
- Validation: `<pass/fail + key output>`
- Submission: `<submitted/not submitted + details>`
- Release notes: `<published / draft-only + URL>`
- Next action: `<one concrete next step if blocked>`
