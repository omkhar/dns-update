# Maintainer Notes

This repository is published as `dns-update`.

## Recommended GitHub Repository Settings

Apply these settings in GitHub before accepting outside contributions:

- Default branch: `main`
- Enable squash merge
- Disable merge commits
- Automatically delete head branches after merge
- Require pull requests before merging to `main`
- Require approval and conversation resolution before merge
- Protect `main` from force-pushes and direct pushes
- Enable Dependabot alerts
- Enable Dependabot security updates
- Enable private vulnerability reporting

## Suggested Required Checks

If you protect `main`, require these checks:

- `CI / Go`
- `CodeQL / Analyze`
- `Dependency Review / dependency-review`
- `zizmor / Analyze workflows`

## Releases

- Public releases are versioned with SemVer tags such as `v1.0`.
- The GitHub release workflow publishes assets from signed tags only.
- Keep `CHANGELOG.md`, `debian/changelog`, and `packaging/rpm/dns-update.spec`
  aligned before cutting a release tag.

## Review Flow

- Ask contributors to open one pull request per logical change.
- Prefer short-lived topic branches such as `feat/...`, `fix/...`, `docs/...`,
  `chore/...`, or `security/...`.
- Use squash merge so the pull request title becomes the final commit subject.
- Cut release tags from `main` only.

## Ownership

This repository intentionally does not ship `CODEOWNERS` yet because there is no
stable maintainer map in the repo. Add `CODEOWNERS` as soon as the maintainer
set is known and keep the ownership narrow around security-sensitive paths such
as:

- `.github/`
- `internal/config/`
- `internal/provider/`
- `internal/securefile/`
- `internal/httpclient/`
