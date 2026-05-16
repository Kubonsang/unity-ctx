---
name: use-unity-ctx
description: Use when implementing or modifying unity-ctx features. Defines the core workflow and safety principles.
---

# Use unity-ctx

## Core Principle

unity-ctx exists to stop AI agents from reading or editing raw Unity YAML.

Prefer:

- summarize
- query
- inspect
- get
- context-pack

Avoid:

- raw full-file output
- heuristic guesses
- mutation without dry-run
- name-based mutation

## Standard Implementation Loop

1. Read docs/SRS.md and relevant command docs.
2. Identify the command contract.
3. Implement the smallest behavior that satisfies the contract.
4. Add or update testdata fixtures.
5. Add tests for stdout, stderr, and exit code.
6. Run `go test ./...`.
7. Report changed commands and remaining limitations.

## Done Means

- Stable output
- Covered by tests
- No Unity Editor required for unit tests
- Errors use explicit codes/messages
