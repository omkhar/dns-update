# Limitations

This document uses ASD-STE100 Simplified Technical English.

## Concurrency

`dns-update` reads provider state before it writes a change.
It does not provide a distributed lock.
Do not run two writers for the same record name at the same time.
Cloudflare does not give this tool a compare-and-swap operation for the full reconciliation.

## Provider and record scope

Cloudflare is the only implemented provider.
The tool manages only A and AAAA records.
It does not manage CNAME, MX, TXT, SRV, or other record types.

The configuration must identify one Cloudflare zone.
Use a token that can edit DNS records only in that zone.

## Probe evidence

The default probes request public HTTPS endpoints.
An HTTP probe is valid only for a loopback or `localhost` test endpoint.

The tool stops reconciliation when a required probe fails.
Optional partial-failure mode changes this rule for one successful address family.
During normal reconciliation, the tool does not delete a record after a failed probe.
The explicit `-delete` mode skips probes and deletes the selected managed record families.

`-force-push` does not bypass probe validation.
It only replaces matching managed records when the selected probe has valid evidence.

## Scheduling

The binary performs one reconciliation and exits.
It does not include a long-running scheduler.
Use systemd, launchd, or Windows Task Scheduler for recurring execution.

Each scheduler can start a new process on another host.
The tool cannot prevent concurrent writers on different hosts.

## Packages

Linux is the only platform with native packages in this repository.
macOS and Windows use the release archive plus the platform scheduler helper.

A Linux package does not install a live configuration or token.
It installs examples that an operator must copy and replace.

## Validation limits

Configuration validation checks syntax, names, URLs, paths, and local token-file protections.
It does not validate live credentials.
It does not prove that the zone identifier and token identify the same Cloudflare account.

The tests use local doubles for provider and network behavior.
Run the documented scheduler health check on each target host after deployment.

## Workflow runtime contract

The workflow runtime check reads a restricted YAML form.
It rejects unsupported YAML keys, aliases, anchors, tags, and flow-style values.
It reads external actions only from jobs and steps that GitHub Actions can run.
It reads integration container pins only from the systemd matrix entries.

Each tracked tool installation must use its exact canonical run block.
The check rejects a full tracked tool name outside these blocks.
The check does not interpret arbitrary shell syntax.
It cannot detect a tracked tool name that a workflow builds from separate string fragments.
Do not build tracked tool names from separate string fragments.
