# Documentation policy

This document uses ASD-STE100 Simplified Technical English.

## Purpose

This policy defines the live documentation set for `dns-update`.
The policy prevents a documentation file from escaping review.

`docs/documentation-inventory.json` classifies each live documentation surface.
The repository test compares that inventory with the files in the repository.
The test fails when a live surface has no classification.

## ASD-STE100 scope

Use ASD-STE100 Issue 9 for first-party operator and contributor text.
Use active voice and short sentences.
Use the imperative form for instructions.
Do not use contractions.
Do not use a semicolon to join clauses.

The automated check sets a maximum of 25 words for one prose sentence.
It finds common passive forms that use `be` plus a past participle.
It ignores fenced code, inline code, URLs, and file-format syntax.
It checks only the `ste` and `generated` inventory classes.
A maintainer must review the remaining ASD-STE100 rules manually.

ASD-STE100 Rule 1.12 permits technical names.
This repository uses product names, command names, code names, paths, and file formats as technical names.

The `ste` inventory class contains first-party text that a person reads.
Each file in that class declares its use of ASD-STE100 Simplified Technical English.

## Generated files

Edit only the canonical files in `docs/agents/**`.
Run this command after each canonical change:

```sh
go run ./cmd/agentdocgen
```

Do not edit these generated projections by hand:

- `AGENTS.md`
- `CLAUDE.md`
- `GEMINI.md`
- `.agents/skills/**`
- `.claude/skills/**`
- `.gemini/commands/**`

The inventory maps each generated file to its canonical source.
The generation test fails when a projection differs from its source.

## Structured files

The `structured` class contains machine-readable documentation contracts.
These files do not contain continuous operator prose.
Their schemas and repository tests control their content.

## Verbatim exclusions

The `verbatim` class contains text that this project must preserve.
Do not rewrite this text to ASD-STE100.

The exclusions are:

- legal license text
- third-party policy text and its attribution
- historical change records
- package provenance records

The RPM `%changelog` section has the same historical exclusion.
The remaining RPM spec is package source, not a documentation surface.

## Supported interface scope

Read `docs/FUNCTIONS.md` for the supported operator and maintainer interface.
Read `docs/user-interface.json` for the code-to-documentation map.
The map covers flags, environment variables, config fields, modes, and exit codes.
It also covers behavior, limitations, schedulers, installers, and maintainer helpers.

Internal Go identifiers are not a public API.
The supported interface excludes internal test scripts and sourced shell libraries.
