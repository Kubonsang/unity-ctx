---
name: safe-mutation
description: Use when implementing set, patch, apply, diff, backup, prefab impact, or any command that modifies Unity files.
---

# Safe Mutation Skill

## Core Rule

All mutation is dry-run-first.

Default:

- no file modification

Actual modification:

- requires `--write`

## Required Mutation Steps

1. Resolve target by fileID when possible.
2. Reject ambiguous name targets.
3. Show dry-run diff.
4. Create `.bak` before write.
5. Apply change.
6. Reparse file.
7. Re-read changed field or object.
8. Update index.
9. Report final summary.

## Prohibited

- direct raw YAML edits without parser/mutation layer
- mutation by name without explicit fallback flag
- deleting `.bak`
- proceeding on UNKNOWN without user approval
- `--yes` unless user explicitly allowed it

## remove_object

Treat as unsafe by default.

Return:

```text
ERROR REMOVE_UNSAFE id=<id> reason=<reason>
```
