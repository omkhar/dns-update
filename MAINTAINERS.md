# Maintainer Notes

The maintainers publish this repository as `dns-update`.
This document uses ASD-STE100 Simplified Technical English.

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

- `CI / Pull Request Reviewability`
- `CI / Lint and Static Analysis`
- `CI / Test (ubuntu-24.04)`
- `CI / Test (macos-26)`
- `CI / Test (windows-2025)`
- `CodeQL / CodeQL`
- `Dependency Review / Dependency Review`
- `zizmor / Analyze workflows`

## Releases

- Use SemVer tags such as `v1.0` for public releases.
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

The wildcard rule in `.github/CODEOWNERS` assigns every repository path to
`@omkhar`. This rule includes these security-sensitive paths:

- `.github/`
- `internal/config/`
- `internal/provider/`
- `internal/securefile/`
- `internal/httpclient/`

Update `.github/CODEOWNERS` when the maintainer set changes.
