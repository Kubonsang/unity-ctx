# unity-ctx

A CLI tool for AI coding agents to read and edit Unity scenes, prefabs, and assets — without loading raw Unity YAML into the prompt.

Unity's serialization format is verbose. A single scene file can easily exceed an agent's token budget, and editing it by hand risks corrupt serialization. `unity-ctx` solves this by exposing compact, query-first commands that keep context small and mutations safe.

## Features

- **Read Unity files without raw YAML** — compact output, token-aware summaries
- **Query by name or fileID** — inspect specific GameObjects, components, and fields
- **Context packing** — assemble agent-ready context within a token budget
- **Safe mutation** — all writes default to dry-run; `--write` required to commit
- **Scene placement planning** — suggest placement candidates near an anchor object, write a diff/apply-compatible patch artifact
- **Prefab impact analysis** — see which scenes and prefabs reference a prefab before mutating it
- **Deterministic output** — every command produces stable, testable text

## Supported file types

| File | Namespace |
|------|-----------|
| `.unity` | `scene` |
| `.prefab` | `prefab` |
| `.asset`, `.mat` | `asset` |

## Installation

```bash
go install github.com/Kubonsang/unity-ctx/cmd/unity-ctx@latest
```

Or build from source:

```bash
git clone https://github.com/Kubonsang/unity-ctx.git
cd unity-ctx
go build ./cmd/unity-ctx
```

Requires Go 1.22+. No external runtime dependencies.

## Quick start

```bash
# Summarize a scene
unity-ctx scene summarize Stage01.unity

# Query objects by name or fileID
unity-ctx scene query Stage01.unity --name Table_01
unity-ctx scene query Stage01.unity --id 1000

# Inspect a component field
unity-ctx scene inspect Stage01.unity --name Enemy --component NavMeshAgent

# Suggest placement candidates and write a patch
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --prefab-guid abc-guid-123 \
  --out chair.patch.json
# PATCH_OUT rank=1 file=chair.patch.json status=WARN candidate_status=OK
# Note: PATCH_OUT status is the generated patch status; candidate_status is the original
# suggest candidate status. They may differ because suggest and patch use different
# overlap semantics (suggest excludes the anchor object; patch does not).

# Preview the patch before applying
unity-ctx scene diff Stage01.unity --patch chair.patch.json

# Dry-run apply (default — no file is written)
unity-ctx scene apply Stage01.unity --patch chair.patch.json

# Apply the patch
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write

# Check the impact of changing a prefab field
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab --project /path/to/MyUnityProject

# Change a prefab field (dry-run by default)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /path/to/MyUnityProject \
  --id 12345 --field moveSpeed --value 4.0

# Measure token reduction from summarize and context-pack
unity-ctx bench Assets/Scenes/Stage01.unity --task "place a chair near Table_01"
```

## Command reference

See [`docs/COMMANDS.md`](docs/COMMANDS.md) for the full command reference.

### Read commands

| Command | Description |
|---------|-------------|
| `scene summarize` | Scene overview: object count, component types, PrefabInstances |
| `scene query` | Filter objects by name, fileID, or type |
| `scene inspect` | Component fields for a specific object |
| `scene get` | Single field value by fileID |
| `scene context-pack` | Token-budgeted context bundle for an agent task |
| `prefab impact` | Which scenes and prefabs reference a prefab |
| `scene scan --mode editor` | Generate a bounds manifest via Unity Editor |
| `scene check` | Validate a bounds manifest against the scene |
| `scene patch --op place_prefab` | Plan a prefab placement (dry-run, no file write) |
| `scene diff --patch` | Summarize a persisted patch plan |
| `scene suggest` | Rank placement candidates near an anchor object |
| `bench` | Measure token reduction (raw vs summarize vs context-pack) |

### Mutation commands

All mutation commands default to dry-run. Pass `--write` to commit changes. A `.bak` backup is created automatically.

| Command | Description |
|---------|-------------|
| `asset set` | Set a field value in a `.asset` or `.mat` file |
| `prefab set` | Set a prefab field value (impact-checked, requires `--ack-impact` when impact is non-trivial) |
| `scene apply --patch` | Apply a patch plan to a scene file |

## Design principles

- **No raw YAML in prompts** — commands emit structured, compact text
- **Dry-run first** — mutations require explicit `--write`
- **UNKNOWN over guessing** — uncertain states are reported, not assumed
- **fileID targeting** — mutations target by fileID; name fallback emits a warning
- **Stable output** — all commands produce deterministic output suitable for tests

## Status

Currently at **v0.5d**. See [`docs/ROADMAP.md`](docs/ROADMAP.md) for the full roadmap.

Implemented:
- Read-only context commands (summarize, query, inspect, get, context-pack)
- Asset and prefab mutation (set, dry-run, impact-checked)
- Scene placement pipeline (scan, check, patch, diff, apply, suggest)
- suggest-to-patch handoff (`--out`, `--pick`, `--prefab-guid`)
- Token reduction benchmarking (bench)

Next milestone: **v1.0 Agent Harness Release** — SKILL docs, AGENTS.md integration guide, sample Unity project, CI examples.

## Contributing

```bash
go test ./...
```

All packages must pass before submitting changes. See [`docs/TESTING.md`](docs/TESTING.md) for the testing guide.
