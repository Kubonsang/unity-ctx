# unity-ctx SRS Rev 5 — Codex-ready

## 1. Definition

`unity-ctx` is a token-aware Unity Context Provider for AI coding agents.

It helps agents inspect `.unity`, `.prefab`, `.asset`, and `.mat` files without reading full raw Unity YAML into the prompt. It provides compact context, query-first inspection, dry-run-first mutation, deterministic output, and safety rules for future scene/prefab/asset editing.

## 2. Core Principles

- **No Raw YAML Reading**: agents should not read or paste whole Unity YAML files.
- **Token Budget**: read commands must support compact output and token-aware limits.
- **Query-first**: use `summarize`, `query`, `inspect`, `get`, `context-pack` before broad reads.
- **Read-only MVP first**: v0.1 implements read-only context features only.
- **Dry-run-first mutation**: all file-changing commands default to dry-run; `--write` is required.
- **fileID-first targeting**: mutation targets should use fileID; name fallback must be explicit.
- **UNKNOWN is not OK**: uncertain states must be reported, not guessed.
- **Verifiable Index**: index uses `file_hash`, not stale cache.
- **Deterministic output**: output must be stable and testable.

## 3. Target Files

| File | Namespace | Purpose |
|---|---|---|
| `.unity` | `unity-ctx scene ...` | scenes, GameObjects, components, PrefabInstances |
| `.prefab` | `unity-ctx prefab ...` | prefab hierarchy and component values |
| `.asset`, `.mat` | `unity-ctx asset ...` | ScriptableObjects, Materials, settings |

## 4. MVP Scope

### Include in v0.1

- Unity YAML block parser
- `scene`, `prefab`, `asset` namespaces
- `summarize`
- `query`
- `inspect`
- `get`
- `--view tiny|compact|detail`
- `--json` when feasible
- small test fixtures under `testdata/`

### Exclude from v0.1

- mutation
- `set --write`
- `patch/apply`
- `remove_object`
- `suggest`
- Editor integration
- FBX parser fallback
- OBB
- impact analysis

## 5. Command Priority

| Version | Commands |
|---|---|
| v0.1 | `summarize`, `query`, `inspect`, `get` |
| v0.2 | `index`, `context-pack`, `bench` |
| v0.3 | `set` dry-run, `asset set --write` |
| v0.4 | `scan --mode editor`, `check`, `patch place_prefab`, `apply`, `diff` |
| v0.5 | `prefab impact`, `prefab set`, basic `suggest` |
| v1.0 | SKILL docs, AGENTS integration, installers, sample Unity project, testplay-runner example |

## 6. Token Measurement

Use the dependency-free estimate:

```text
estimated_tokens = ceil(utf8_bytes / 4)
```

`context-pack` must emit `OMITTED` when content is dropped due to token budget.

Example:

```text
OMITTED reason=token_budget nearby=12 warnings=3 components=5
NEXT_QUERY unity-ctx scene query Stage01.unity --near Table_01 --radius 6
```

## 7. Output Levels

| View | Purpose | Agent use |
|---|---|---|
| `tiny` | minimal identity/value | allowed |
| `compact` | default stable agent output | allowed |
| `detail` | debug only, includes YAML keys | not automatic |
| `json` | automation/CI | allowed |

## 8. Parser Requirements

The parser must:

- split Unity YAML by `--- !u!<classID> &<fileID>`
- preserve fileID
- preserve unknown blocks
- map common classIDs:
  - 1 GameObject
  - 4 Transform
  - 20 Camera
  - 23 MeshRenderer
  - 33 MeshFilter
  - 54 Rigidbody
  - 65 BoxCollider
  - 114 MonoBehaviour
  - 1001 PrefabInstance
- support scalar, bool, int, float, Vector2/3/4-like values
- support dot notation
- report `FIELD_NOT_FOUND`, `AMBIGUOUS_NAME`, `UNKNOWN_COMPONENT` explicitly

## 9. Inspector Commands

### inspect

```bash
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset inspect EnemyConfig.asset
unity-ctx scene inspect Stage01.unity --id 12003 --component BoxCollider
```

### get

```bash
unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
unity-ctx asset get EnemyConfig.asset --field maxHealth
unity-ctx scene get Stage01.unity --id 12003 --component Rigidbody --field mass
```

## 10. Mutation Policy for Future Versions

All mutation commands must be dry-run-first.

```bash
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300
# DRY_RUN EnemyConfig.maxHealth 200 -> 300

unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write
# WRITE backup=EnemyConfig.asset.bak changed=1
```

Rules:

- `--write` required for actual file changes
- create `.bak`
- reparse file after write
- verify with `get`, `query`, or `diff`
- `UNKNOWN` blocks write unless user explicitly approves
- name-based mutation requires `--allow-name-fallback`
- ambiguous names are errors
- `remove_object` is unsafe by default

## 11. Exit Codes

| Code | Meaning |
|---:|---|
| 0 | OK / WARN / UNKNOWN / NEED_PREFAB_GUID |
| 1 | ERROR condition (incl. post-write graph corruption) |
| 2 | tool execution / usage error |
| 3 | BLOCKED — safety refused the mutation before any write; file untouched |

`UNKNOWN` may return 0 but must not imply permission to write. `BLOCKED` is a
distinct exit code (`3`) so a refused mutation is never mistaken for success.

## 12. testplay-runner Integration

`unity-ctx` handles static context, inspection, and safe serialized-data mutation.  
`testplay-runner` handles Play Mode/runtime verification.

Recommended loop:

```text
unity-ctx context-pack / get / inspect
→ unity-ctx set or patch/apply dry-run
→ unity-ctx set --write or apply --write
→ unity-ctx get / query / diff
→ testplay-runner quick or scene-smoke
```

`prefab impact` should recommend extra tests, not replace required smoke tests.

## 13. Acceptance Tests

- scene summarize returns stable compact output
- prefab inspect/get returns expected fields
- asset get returns ScriptableObject field
- duplicate names produce `AMBIGUOUS_NAME`
- unknown classID is preserved
- index detects `INDEX_STALE`
- context-pack emits `OMITTED` when token budget is exceeded
- set without `--write` does not modify files
- remove unsafe returns `ERROR REMOVE_UNSAFE`

## 14. Codex Direction

Codex must start with v0.1 only.

Starter prompt:

```text
Read AGENTS.md and docs/SRS.md first.
Implement v0.1 only.
Do not implement mutation yet.
Focus on Unity YAML block parsing, summarize, query, inspect, get, and stable compact output.
Add tests using testdata fixtures.
Run go test ./... before final response.
If the SRS is ambiguous, choose the safer behavior and document the assumption.
```
