---
name: command-contract
description: Use when adding or changing CLI commands, flags, output format, JSON format, or exit codes.
---

# Command Contract Skill

## Required for Every Command

Document:

- command syntax
- required args
- optional flags
- stdout format
- stderr format
- exit codes
- JSON output if supported

## Exit Codes

- 0: OK / WARN / UNKNOWN only
- 1: ERROR condition
- 2: tool execution error

## Output Rules

- Compact output is default.
- Output must be stable and testable.
- Do not include timestamps in default stdout.
- Do not include nondeterministic ordering.
- Sort output by fileID or path unless specified.
- Warnings must start with `WARN`.
- Errors must start with `ERROR`.
- Unknowns must start with `UNKNOWN`.

## Testing

Every command must have tests for:

- success
- missing file
- invalid args
- not found
- ambiguous match when applicable
