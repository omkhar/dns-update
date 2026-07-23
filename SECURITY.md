# Security Policy

This document uses ASD-STE100 Simplified Technical English.

## Supported Code

Put security fixes on the current default branch first.

Unless a future release process says otherwise:

- the latest code on the default branch is the supported version
- older snapshots, forks, and unpublished local modifications are not supported

## Reporting a Vulnerability

Do not open a public GitHub issue with exploit details for an unpatched
vulnerability.

Use one of these paths:

1. GitHub private vulnerability reporting for this repository:
   `https://github.com/omkhar/dns-update/security/advisories/new`
2. Any dedicated private security contact path published by the maintainers.

Repository security settings and policy are also published at:

- `https://github.com/omkhar/dns-update/security/policy`

If no private path is available, open a minimal public issue.
Ask for a private reporting route.
Do not include sensitive details, proof-of-concept code, tokens, or reproduction steps.

## Probe Failure Posture

`dns-update` treats single-family probe failures as reconciliation failures by
default. That fail-closed behavior keeps a failed IPv4 or IPv6 probe from
suppressing updates for the corresponding managed record family.

Set `probe.allow_partial_failure` to `true` for single-family availability.
In that mode, the command reconciles the successful family.
It leaves the failed family unchanged.
A later successful probe or an explicit `ip=none` response can change that family.

## What to Include

When reporting a vulnerability, include:

- the affected version, branch, or commit if known
- the component or file involved
- clear reproduction steps
- impact and expected blast radius
- any relevant logs or traces with secrets removed

## Response Expectations

Maintainers aim to acknowledge a complete vulnerability report within 7 days.
They aim to give a remediation or mitigation plan within 30 days.

Wait for a fix or mitigation before public disclosure.
You can disclose earlier when maintainers confirm that coordinated disclosure is safe.

## Coordinated Disclosure

Do not make a public disclosure before a fix or mitigation is available.
Give the project a reasonable time to respond.
