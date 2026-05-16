---
name: token-budget
description: Use when implementing summarize, context-pack, compact output, omitted sections, or benchmarks.
---

# Token Budget Skill

## Token Estimate

Use:

```text
estimated_tokens = ceil(utf8_bytes / 4)
```

Do not add tokenizer dependencies.

## Output Levels

- tiny: minimal identity and value
- compact: default agent-friendly output
- detail: debug only
- json: structured automation output

## context-pack Rules

- FOCUS is never omitted.
- Omit lower-priority sections first.
- If anything is omitted, output `OMITTED`.
- Provide a `NEXT_QUERY` suggestion when useful.

## OMITTED Example

```text
OMITTED reason=token_budget nearby=12 warnings=3 components=5
NEXT_QUERY unity-ctx scene query Stage01.unity --near Table_01 --radius 6
```

## Prohibited

- dumping full YAML
- using detail output as default
- hiding omission from the user/agent
