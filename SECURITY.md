# Security Policy

## Supported Code

Security fixes are expected to land on the current default branch first.

Unless a future release process says otherwise:

- the latest code on the default branch is the supported version
- older snapshots, forks, and unpublished local modifications are not supported

## Reporting a Vulnerability

Do not open a public GitHub issue with exploit details for an unpatched
vulnerability.

Use one of these paths:

1. GitHub private vulnerability reporting for this repository, if it is enabled.
2. Any dedicated private security contact path published by the maintainers.

If neither private path is available yet, open a minimal public issue asking for
a private reporting route and do not include sensitive details, proof-of-concept
code, tokens, or reproduction steps.

## What to Include

When reporting a vulnerability, include:

- the affected version, branch, or commit if known
- the component or file involved
- clear reproduction steps
- impact and expected blast radius
- any relevant logs or traces with secrets removed

## Response Expectations

This repository does not currently publish formal response SLAs.

When maintainers adopt a formal security process, this file should be updated
with:

- acknowledgment targets
- remediation timelines
- release and disclosure expectations

## Coordinated Disclosure

Please avoid public disclosure until a fix or mitigation is available and the
project has had a reasonable opportunity to respond.
