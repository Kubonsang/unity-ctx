---
name: release-check
description: Use before considering a unity-ctx task complete.
---

# Release Check Skill

Before final response:

1. Run `go test ./...`.
2. Run `go run ./cmd/unity-ctx --help`.
3. Run at least one command against testdata when relevant.
4. Confirm no mutation command writes without `--write`.
5. Confirm compact output is stable.
6. Confirm docs changed if command behavior changed.
7. Summarize:
   - changed files
   - tests run
   - known limitations
