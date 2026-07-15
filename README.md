# unity-ctx

**Go CLI that gives AI coding agents a token-safe read/write interface to Unity scenes, prefabs, and assets — no raw YAML, no silent corruption.**

[![CI](https://github.com/Kubonsang/unity-ctx/actions/workflows/ci.yml/badge.svg)](https://github.com/Kubonsang/unity-ctx/actions/workflows/ci.yml)

[한국어](README.ko.md) | English

---

Unity's serialization format is hostile to automation: a single scene file can exceed an agent's token budget, editing raw YAML by hand risks corrupt serialization, and there is no safe dry-run path. `unity-ctx` fixes this with a query-first command surface designed for AI agents.

## Who is unity-ctx for?

unity-ctx is a **context layer**, not an editor replacement. Two distinct users:

- **AI coding agents** — automated callers that need compact context, unambiguous output prefixes, and dry-run-first mutation. unity-ctx is built for them.
- **Human developers** — keep using the Unity Editor for scene authoring. unity-ctx does not compete with the Editor; it makes the *automated* path safe.

If your agent is iterating on a Unity project, unity-ctx's whole job is making each read and write legible without blowing the token budget or corrupting a scene file.

## Problems Solved

| Problem | Solution |
|---|---|
| Scene files exceed token budget | `summarize` and `context-pack` emit compact, token-bounded output |
| Raw YAML is unsafe to edit | All mutations are dry-run by default; `--write` required to commit |
| Object names are ambiguous | `query` resolves names to fileIDs; mutations target by fileID |
| Prefab GUID unknown | Returns `UNKNOWN` with `NEED_PREFAB_GUID` — never guesses |
| Prefab blast radius unknown | `prefab impact` scans all referencing scenes and prefabs before mutation |
| Placement position is guesswork | `scene suggest` ranks candidates near an anchor using a bounds manifest |
| No way to preview a scene write | `scene diff` summarizes a patch plan before `scene apply` commits it |
| Token cost of a file is unknown | `bench` measures raw vs summarize vs context-pack reduction |

## Installation

**From source (requires Go 1.22+):**

```bash
go install github.com/Kubonsang/unity-ctx/cmd/unity-ctx@latest
```

Or build locally:

```bash
git clone https://github.com/Kubonsang/unity-ctx.git
cd unity-ctx
go build -o unity-ctx ./cmd/unity-ctx
```

No external runtime dependencies.

> **Building from source (v0.6+):** unity-ctx depends on the
> [unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) safety
> kernel (version pinned in `go.mod`), fetched automatically from
> GitHub by the Go toolchain — no manual setup. The produced binary remains a
> single static executable with no runtime dependencies.

## Supported File Types

| Extension | Namespace |
|---|---|
| `.unity` | `scene` |
| `.prefab` | `prefab` |
| `.asset`, `.mat` | `asset` |

## Using with Your Own Unity Project

unity-ctx requires no configuration file. You pass file paths directly on the command line.

**Read commands** — point to any `.unity`, `.prefab`, or `.asset` file anywhere on disk:

```bash
unity-ctx scene summarize /Users/me/MyUnityProject/Assets/Scenes/GameLevel.unity
unity-ctx prefab inspect /Users/me/MyUnityProject/Assets/Prefabs/Player.prefab --component Rigidbody
unity-ctx asset get /Users/me/MyUnityProject/Assets/Configs/GameConfig.asset --field startingHealth
```

**Mutation commands that need `--project`** (`prefab impact`, `prefab set`) — pass the Unity project root, the directory that contains `Assets/`, `ProjectSettings/`, and `Packages/`. The prefab path must be under that root's `Assets/` tree:

```bash
unity-ctx prefab impact Assets/Prefabs/Player.prefab \
  --project /Users/me/MyUnityProject

unity-ctx prefab set Assets/Prefabs/Player.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 --field moveSpeed --value 5.0
```

**Getting a prefab's GUID** — required for `scene patch` and `scene suggest --out`. `meta guid` reads it from the `.meta` file next to the prefab (and `patch`/`suggest` auto-resolve it the same way when `--prefab-guid` is omitted):

```bash
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project /Users/me/MyUnityProject
# OK guid=abc123def456... file=... meta=...
```

## Safety Integration (v0.6)

Every write path is validated by the
[unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) safety
kernel — a lossless Unity YAML block parser plus fileID graph integrity
checker — at three points:

```text
pre_check   target file before planning   → ERROR blocks (BLOCKED, exit 3)
temp_check  candidate bytes before commit → ERROR blocks (BLOCKED, exit 3)
   --write  atomic write with .bak backup
final_check re-read file after commit     → ERROR reports WRITE_COMMITTED (exit 1) with the backup path
```

- A file that is already structurally broken (duplicate fileIDs, missing
  component blocks, mismatched back-references, ...) is never mutated.
- `WARN` findings (unknown class IDs, unsupported shapes) are surfaced on the
  summary line and in `CHECK` detail lines but do not block.
- `BLOCKED` is a safety verdict, not a tool failure — do not work around it by
  editing the YAML directly.
- `refs` exposes the kernel's PPtr/GUID reference evidence
  (`unity-ctx prefab refs Enemy.prefab --json`) for blast-radius analysis.

**Placement pipeline** — `scene scan` requires the Unity Editor to be running with the project open. All other placement commands (`suggest`, `patch`, `diff`, `apply`) work without the Editor:

```bash
# 1. With Editor open — generate bounds manifest
unity-ctx scene scan Assets/Scenes/GameLevel.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --out /tmp/GameLevel.bounds.json

# 2–4. No Editor needed
unity-ctx scene suggest Assets/Scenes/GameLevel.unity \
  --manifest /tmp/GameLevel.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near SpawnPoint_01 \
  --prefab-guid abc123def456 \
  --out /tmp/chair.patch.json

unity-ctx scene diff Assets/Scenes/GameLevel.unity --patch /tmp/chair.patch.json
unity-ctx scene apply Assets/Scenes/GameLevel.unity --patch /tmp/chair.patch.json --write
```

> Paths passed to unity-ctx can be absolute or relative to your current working directory. Running commands from the project root (`cd /Users/me/MyUnityProject`) lets you use short `Assets/...` paths throughout.

## Spatial geometry v0.9

`scene scan --mode editor --geometry detailed` asks the Unity Editor to emit Spatial Manifest v2: Collider-first compound local OBB drafts, semantic contact frames, and planar surface candidates. Scanned geometry is unreviewed until the Unity workflow or an approved matching contract verifies it. `scene check` then proves rotated overlap and finite-surface contact gap/penetration/support, and `scene suggest --align wall --surface-id ...` returns deterministic candidates from approved contact requirements. Manifest v1 remains supported for existing AABB workflows; contact requests against it return `UNKNOWN NEED_GEOMETRY_V2`.

The MCP server adds read-only `unity_spatial_check` and `unity_suggest_wall` tools. Unity remains the final scene authority, raw FBX files are not parsed by Go, and no mutation tool is added to MCP.

## Human-reviewed Spatial Contracts

Reusable asset and `SupportedBy` interaction contracts are separate from scene snapshot manifests. They store normalized compound OBBs, named contact frames, simultaneous contact requirements, revisions, dependency hashes, capture hashes, deterministic evidence, and the local human review.

```bash
unity-ctx spatial validate Library/DungeonDecorator/SpatialDrafts/banner.spatial.json
unity-ctx spatial diff --current Assets/SpatialContracts/Assets/<guid>.spatial.json --draft Library/DungeonDecorator/SpatialDrafts/banner.spatial.json
unity-ctx spatial apply --current Assets/SpatialContracts/Assets/<guid>.spatial.json --draft Library/DungeonDecorator/SpatialDrafts/banner.spatial.json
```

The public CLI can validate, diff, record non-approval feedback, and dry-run an apply. It cannot create `Approved` evidence or execute `spatial apply --write`. Only the local human-review bridge can call the authorized approval/write APIs after verifying its one-time evidence. Geometry, interaction, or capture changes invalidate technical and human evidence. MCP exposes neither approval nor mutation.

## Surface Arrangement contracts

`unity-ctx arrangement validate <file>` strictly validates the stored `spec_hash`. `unity-ctx arrangement hash <file>` instead recomputes the normalized replacement when an edited draft still contains its old hash. Arrangement IDs are portable ASCII identifiers up to 128 characters and `edge_margin` is limited to 0–100 meters, keeping Go and Unity canonical hashes identical without exponent or Unicode ordering ambiguity.

## Commands

### `unity-ctx scene summarize`

Compact overview of a scene: object count, component types, PrefabInstance list.

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx asset summarize Assets/Configs/EnemyConfig.asset
```

---

### `unity-ctx scene query`

Filter objects by name, fileID, or component type. Use this to resolve a name to a fileID before targeting mutations.

```bash
unity-ctx scene query Stage01.unity --name Table_01
unity-ctx scene query Stage01.unity --id 1000
unity-ctx scene query Stage01.unity --type NavMeshAgent
```

Output:
```
FOUND id=1000 name=Table_01 type=GameObject
```

---

### `unity-ctx scene inspect`

Component fields for a specific object.

```bash
unity-ctx scene inspect Stage01.unity --id 1000 --component Rigidbody
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset inspect EnemyConfig.asset
```

---

### `unity-ctx scene get`

Single field value by fileID.

```bash
unity-ctx scene get Stage01.unity --id 1000 --component Rigidbody --field mass
unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
unity-ctx asset get EnemyConfig.asset --field maxHealth
```

---

### `unity-ctx scene context-pack`

Assembles a token-budgeted context bundle for an agent task. Emits `OMITTED` lines when the budget is exhausted.

```bash
unity-ctx scene context-pack Stage01.unity --task "place a chair near Table_01" --max-tokens 4000
```

---

### `unity-ctx bench`

Measures token reduction: raw file vs summarize vs context-pack. Uses `ceil(utf8_bytes / 4)` as the token estimate — no external tokenizer required.

```bash
unity-ctx scene bench Assets/Scenes/Stage01.unity
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety"
```

`context-pack` is measured only when `--task` is provided.

#### Benchmarks

Real numbers from `bench` against the repo fixtures. Token estimate is
`ceil(utf8_bytes / 4)`, so these are reproducible with no tokenizer:

```bash
unity-ctx scene bench testdata/scenes/simple_scene.unity \
  --task "place a chair near Table_01"
unity-ctx prefab bench testdata/prefabs/enemy.prefab \
  --task "inspect placement safety"
```

| Fixture | raw tokens | summarize | context-pack (`--task`) |
|---|---|---|---|
| `testdata/scenes/simple_scene.unity` | 175 | 22 (−87%) | 50 (−71%) |
| `testdata/prefabs/enemy.prefab` | 186 | 21 (−89%) | 36 (−81%) |

These fixtures are tiny (sub-1 KB, hand-authored), so the absolute token counts
are small — the point is the *ratio*. `summarize` already strips ~88% of the
tokens a raw read would cost, and `context-pack` stays token-bounded while
keeping task-relevant context. On real Unity scenes (tens of KB to multiple MB)
the same code path produces far larger reductions, because raw YAML grows with
object count while `summarize`/`context-pack` output stays compact.

---

### `unity-ctx scene scan`

Generates a bounds manifest by querying the Unity Editor. **Requires the Unity Editor to be running with the project open.**

```bash
unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --out /tmp/Stage01.bounds.json

# Limit to specific prefabs
unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --prefabs Assets/Prefabs/Chair.prefab,Assets/Prefabs/Table.prefab \
  --out /tmp/Stage01.bounds.json
```

Required: `--mode editor`, `--project`, `--out`

Output:
```
OK mode=editor project=/Users/me/MyUnityProject scene=Assets/Scenes/Stage01.unity out=/tmp/Stage01.bounds.json objects=12 prefabs=3 source=editor
```

---

### `unity-ctx scene suggest`

Ranks placement candidates near an anchor object. Read-only — does not write the scene. With `--out`, also writes a patch artifact ready for `diff` and `apply`.

```bash
# Rank candidates
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --align grid \
  --count 4

# Write patch for the top candidate (v0.5d)
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --prefab-guid abc-guid-123 \
  --out chair.patch.json \
  --pick 1

# Wall-backed furniture uses its approved wall and floor requirements
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.spatial.json \
  --prefab Assets/Props/Bookcase.fbx \
  --align wall --surface-id wall-north \
  --contact wall-backed --count 4
```

Required: `--manifest`, `--prefab`, and either `--near` or `--align wall --surface-id ID`.
Optional: `--count` (default 4, max 4), `--align floor|grid|wall` (default `floor`), wall-only `--contact wall-backed|wall-mounted`, `--out`, `--pick` (default 1), `--prefab-guid`.

`--pick` and `--prefab-guid` require `--out`.

Output:
```
OK manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab near=1000 align=floor count=4 candidates=4 clear=4 warn=0
CANDIDATE rank=1 direction=east position=1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=2 direction=west position=-1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=3 direction=north position=0,0,1.4 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=4 direction=south position=0,0,-1.4 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
PATCH_OUT rank=1 file=chair.patch.json status=WARN candidate_status=OK
```

> `PATCH_OUT status` is the patch planner result (anchor included in overlap check). `candidate_status` is the suggest result (anchor excluded). They can differ for the same position — `candidate_status=OK` with `status=WARN` is normal.

---

### `unity-ctx scene patch`

Generates a prefab placement patch plan without writing to the scene.

```bash
unity-ctx scene patch Stage01.unity \
  --op place_prefab \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --position 5,0,0 \
  --prefab-guid abc-guid-123
```

Required: `--op place_prefab`, `--manifest`, `--prefab`, `--position`
Optional: `--prefab-guid`, `--project`, `--json`

With `--project`, the prefab GUID is auto-resolved from `<prefab>.meta` when `--prefab-guid` is omitted. If it still cannot be resolved, the command returns `UNKNOWN ... NEED_PREFAB_GUID` — it never guesses. You can also look the GUID up explicitly with `unity-ctx meta guid`.

Output:
```
OK op=place_prefab manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=5,0,0 overlap_ids=none reserved_fileIDs=2002,2003
PLAN prefab_guid="abc-guid-123" append_ops=append:1:2002:GameObject,append:4:2003:Transform
```

---

### `unity-ctx scene diff`

Summarizes a persisted patch plan. Always run this before `apply`.

```bash
unity-ctx scene diff Stage01.unity --patch chair.patch.json
```

Output:
```
OK patch=chair.patch.json op=place_prefab append_ops=2 reserved_fileIDs=2002,2003
WARN patch=chair.patch.json op=place_prefab overlap_ids=2000 append_ops=2 reserved_fileIDs=2002,2003
UNKNOWN patch=chair.patch.json op=place_prefab reason=NEED_PREFAB_GUID append_ops=2 reserved_fileIDs=2002,2003
```

---

### `unity-ctx scene apply`

Applies a patch plan to the scene. Dry-run by default; `UNKNOWN` patches are refused.

```bash
# Dry-run (default)
unity-ctx scene apply Stage01.unity --patch chair.patch.json

# Commit
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write
```

`--write` creates `Stage01.unity.bak` before modifying the file, then reparses the result and verifies appended fileIDs before reporting success.

Output:
```
DRY_RUN patch=chair.patch.json op=place_prefab append_ops=2 changed=1 verified=1
WRITE backup=Stage01.unity.bak patch=chair.patch.json op=place_prefab append_ops=2 changed=1 verified=1
```

---

### `unity-ctx scene reposition`

Moves an existing scene object: rewrites a Transform's `m_LocalPosition` in
place. `--id` is the **Transform** fileID (class 4 or RectTransform 224), not
the GameObject. Dry-run by default.

```bash
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --write
```

Output:
```
DRY_RUN id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 pre_check=OK temp_check=OK
WRITE backup=Stage01.unity.bak id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK
```

Only the three axis numbers change — separators, comments, and every other byte
survive. A non-transform target (`UNSUPPORTED_TARGET_CLASS`) or a field that is
not exactly `{x, y, z}` of numbers (`FIELD_NOT_VECTOR3`) is refused rather than
mangled.

---

### `unity-ctx scene reparent` (v2 patch)

Moves a Transform under a new parent — updates the target's `m_Father` and both
parents' `m_Children` atomically. Flows through the same patch → diff → apply
pipeline using a v2 `ops[]` patch. `--new-parent 0` moves the target to the
scene root.

```bash
unity-ctx scene patch Stage01.unity --op reparent --id 4001 --new-parent 4002 --json > reparent.patch.json
unity-ctx scene diff  Stage01.unity --patch reparent.patch.json
unity-ctx scene apply Stage01.unity --patch reparent.patch.json --write --ack-impact
```

A reparent that would create a cycle is refused before any write
(`BLOCKED phase=plan code=WOULD_CREATE_CYCLE`). With `--project`, apply also
reports inbound cross-file references (`WARN REPARENT_HAS_INBOUND_REFS`) —
informational only, since the moved fileID stays valid.

---

### `unity-ctx scene delete` (v2 patch)

Removes a GameObject and its components from a scene (whole subtree with
`--cascade`), unlinking it from the parent's `m_Children` or the scene's
`SceneRoots`. `--id` is the **GameObject** fileID.

```bash
unity-ctx scene patch Stage01.unity --op delete --id 1001 --cascade --json > delete.patch.json
unity-ctx scene diff  Stage01.unity --patch delete.patch.json
unity-ctx scene apply Stage01.unity --patch delete.patch.json --write --ack-impact --project /path/to/project
```

Deletes are guarded harder than reparent because removal dangles references:
deleting an object with children requires `--cascade`
(`BLOCKED WOULD_ORPHAN_CHILDREN`), prefab-instance content is never raw-deleted
(`STRIPPED_IN_SUBTREE`), a surviving same-file reference blocks
(`IN_FILE_REFERENCED`), and `--write` **requires `--project`** so every
committed delete is cross-file-verified — any inbound or unaccountable
reference blocks the write (`BLOCKED code=CROSS_FILE_REFERENCED`).

---

### `unity-ctx asset set`

Sets a field value in a `.asset` or `.mat` file. Dry-run by default.

```bash
# Dry-run
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300

# Commit
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write

# Target by fileID
unity-ctx asset set EnemyConfig.asset --id 11400000 --field moveSpeed --value 4.0 --write
```

Output:
```
DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1
WRITE backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1
OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1
```

`.bak` is created only when a write actually changes the file. `changed=0` returns success without touching the filesystem.

---

### `unity-ctx prefab impact`

Scans which scenes and prefabs reference a prefab. Run this before `prefab set` to understand blast radius. Nested traversal is capped at depth 3.

```bash
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject

# Limit scope to specific scenes
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --scenes Assets/Scenes/BossRoom.unity,Assets/Scenes/Stage01.unity
```

Required: `--project`
Optional: `--scenes` (comma-separated), `--json`

Output:
```
OK prefab=Assets/Prefabs/Enemy.prefab guid=abc123 scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

When the depth cap is reached:
```
WARN IMPACT_DEPTH_LIMIT prefab=Assets/Prefabs/Enemy.prefab depth=3 more_possible=true
```

---

### `unity-ctx prefab set`

Sets a prefab field value. Dry-run output includes an impact summary. Writes require `--ack-impact`.

```bash
# Dry-run (impact summary included automatically)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field moveSpeed \
  --value 4.0

# Commit
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field moveSpeed \
  --value 4.0 \
  --write --ack-impact
```

Required: `--project`, `--id`, `--field`, `--value`

Targeting is fileID-only. `--name` and `--component` are not supported for `prefab set`.

Output (dry-run):
```
DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1 pre_check=OK temp_check=OK
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

> `scene apply`, `prefab set`, and `asset set` all carry `pre_check`/`temp_check`/`final_check` fields and refuse unsafe writes — see [Safety Integration](#safety-integration-v06) above.

---

### `unity-ctx meta guid`

Resolves a prefab/asset GUID from its sibling `.meta` file. Never guesses.

```bash
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project /Users/me/MyUnityProject
```

Output:
```
OK guid=3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b file=Assets/Prefabs/Chair.prefab meta=Assets/Prefabs/Chair.prefab.meta
NEED_PREFAB_GUID file=Assets/Prefabs/Chair.prefab reason=meta_not_found
```

---

### `unity-ctx refs`

Read-only PPtr/GUID reference evidence for a file, backed by the safety kernel. Lets an agent trace what a file points at without reading raw YAML.

```bash
unity-ctx prefab refs Assets/Prefabs/Enemy.prefab
unity-ctx scene refs Assets/Scenes/Stage01.unity --json
```

Output:
```
OK refs file=Assets/Prefabs/Enemy.prefab count=2 warnings=0
REF block=1000 class=GameObject field=m_Component[0].component file_id=2000
REF block=3000 class=MonoBehaviour field=m_Script file_id=11500000 guid=a1b2c3d4e5f60718293a4b5c6d7e8f90 type=3
```

`--json` adds a `refs` payload with `references[]`, a `warnings` count, and `issues[]` (warning detail).

---

### `unity-ctx validate`

Read-only fileID graph integrity check — the same check that gates every write path, run on its own so an agent can confirm a file is sound *before* editing.

```bash
unity-ctx prefab validate Assets/Prefabs/Enemy.prefab
unity-ctx scene validate Stage01.unity --json
```

`OK`/`WARN` exit `0`; `ERROR` (broken graph) exits `1`.

---

### `unity-ctx changes`

Structural diff of a file against its `<file>.bak` — what the last committed `set`/`apply` changed — by matching blocks on fileID.

```bash
unity-ctx asset changes EnemyConfig.asset
```

Reports `ADDED`/`REMOVED`/`CHANGED` per object. Exit `1` if no backup exists.

---

### `unity-ctx restore`

Recover a file from its `<file>.bak`, undoing the last committed write. Atomic; reports the restored content's integrity (`check=`).

```bash
unity-ctx asset restore EnemyConfig.asset
```

---

### `unity-ctx deps`

Forward asset dependencies: the GUIDs a file references, resolved to asset paths under `--project`. `--out` writes a Graphviz graph.

```bash
unity-ctx prefab deps Assets/Prefabs/Enemy.prefab --project /path/to/project
unity-ctx scene deps Stage01.unity --project . --out deps.dot
```

Unresolved GUIDs are reported as `UNKNOWN`, never guessed.

---

### `unity-ctx mcp`

Run an [MCP](https://modelcontextprotocol.io) server over stdio so MCP hosts (Claude Code, etc.) call unity-ctx's read-only commands as native tools.

```bash
unity-ctx mcp
claude mcp add unity-ctx -- unity-ctx mcp
```

Exposes read-only tools (`unity_summarize`, `unity_validate`, `unity_refs`, `unity_query`, `unity_get`, `unity_deps`, `unity_impact`). Mutations stay behind the CLI's `--write` contract.

## Output Prefixes

Every command produces a single-prefix first line. Automated callers can branch on the prefix without parsing the rest.

| Prefix | Meaning |
|---|---|
| `OK` | Success, no warnings |
| `WARN` | Success with a condition to review (e.g., placement overlap) |
| `ERROR` | Failure — command did not complete |
| `UNKNOWN` | Insufficient information to proceed (e.g., missing GUID) |
| `DRY_RUN` | Mutation previewed, no file written |
| `WRITE` | File written and verified |
| `FOUND` | Query matched at least one object |
| `OMITTED` | Token budget exhausted, content was skipped |
| `CANDIDATE` | One ranked placement suggestion |
| `PLAN` | Patch plan detail line |
| `PATCH_OUT` | Patch artifact written by `suggest --out` |
| `SCENES` / `PREFABS` | Impact analysis result lines |
| `BLOCKED` | Write refused by a graph-integrity failure (`code=GRAPH_CHECK_FAILED`) |
| `CHECK` | Per-phase safety-check detail line |
| `REF` | Reference evidence line from `refs` |
| `NEED_PREFAB_GUID` | GUID could not be resolved from `.meta` |

Exit codes: `0` = OK / WARN / UNKNOWN / NEED_PREFAB_GUID, `1` = ERROR, `2` = tool execution / usage error, `3` = BLOCKED. `BLOCKED` exits `3` — a distinct code so an agent never mistakes a safety refusal (file untouched) for a completed write; `NEED_PREFAB_GUID` stays `0` as a missing precondition, not a refusal.

## Recommended Agent Flow

```
# Inspect a scene
unity-ctx scene summarize Stage01.unity
unity-ctx scene query Stage01.unity --name Table_01   # → get fileID
unity-ctx scene inspect Stage01.unity --id 1000 --component Rigidbody

# Place a prefab (GUID auto-resolved from .meta via --project)
unity-ctx scene scan Stage01.unity --mode editor --project /path/to/project --out stage01.bounds.json
unity-ctx scene suggest Stage01.unity --manifest stage01.bounds.json --prefab Chair.prefab --near 1000 --project /path/to/project --pick 1 --out chair.patch.json
unity-ctx scene diff Stage01.unity --patch chair.patch.json
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write   # runs pre/temp/final graph checks

# Modify a prefab field
unity-ctx prefab impact Enemy.prefab --project /path/to/project
unity-ctx prefab set Enemy.prefab --project /path/to/project --id 11400000 --field moveSpeed --value 4.0
unity-ctx prefab set Enemy.prefab --project /path/to/project --id 11400000 --field moveSpeed --value 4.0 --write --ack-impact
```

## Design Principles

- **No raw YAML in prompts** — every command emits compact, structured text
- **Dry-run first** — `set` and `apply` require explicit `--write` to modify files
- **UNKNOWN over guessing** — uncertain states (`NEED_PREFAB_GUID`, `AMBIGUOUS_NAME`) are reported, not assumed
- **fileID targeting** — mutations target by fileID; name-based targeting emits `WARN` or `ERROR AMBIGUOUS_NAME`
- **Stable output** — all commands produce deterministic output; tests may assert exact strings
- **No external runtime dependencies** — core commands require only Go 1.22+

## Development

```bash
# Run all tests
go test ./...

# Build
go run ./cmd/unity-ctx --help
```

## Documentation

| Audience | Document |
|----------|----------|
| Full command contract — flags, output, exit codes | [`docs/COMMANDS.md`](docs/COMMANDS.md) |
| AI agent **using** the CLI — operating manual | [`docs/AGENT-USAGE.md`](docs/AGENT-USAGE.md) |
| AI agent **contributing to** the codebase | [`AGENTS.md`](AGENTS.md) |
| Testing guide | [`docs/TESTING.md`](docs/TESTING.md) |
| Roadmap | [`docs/ROADMAP.md`](docs/ROADMAP.md) |

## Known Limitations

These are current gaps, documented honestly.

**`scene scan` requires Unity Editor.**
`scan` is the only command that requires a running Unity Editor instance. All other commands — including `suggest`, `patch`, `diff`, `apply`, `impact`, and all read commands — work without the Editor. If the Editor is not running, `scan` fails with `ERROR SCAN_EDITOR_FAILED`.

**Wall suggestions require reviewed v2 evidence.**
`--align wall` requires a reviewed `SurfacePatch` and an approved, GUID/dependency-hash-matched Spatial Contract. Missing or stale evidence returns `UNKNOWN`; it never falls back to AABB or an invented contact policy.

**`prefab set` does not support `--name` or `--component` targeting.**
Only `--id` (fileID) is accepted. Use `prefab inspect` to find the fileID first.

**`scene scan --mode` only supports `editor`.**
Standalone bounds generation without the Editor is not yet implemented.

**Nested prefab traversal is capped at depth 3.**
`prefab impact` and `prefab set` emit `WARN IMPACT_DEPTH_LIMIT` when the cap is reached. References beyond depth 3 may exist and are not reported.

**`scene reposition` edits the raw Transform, not prefab-instance overrides.**
For an object that is a prefab instance, the effective position lives in
`PrefabInstance.m_Modifications`; repositioning the raw Transform may have no
visual effect. Works as expected on plain (non-instance) scene objects.

**Structural mutations are scene-only and one op per patch.**
`reposition`/`reparent`/`delete` operate on `.unity` files only; a v2 patch
carries exactly one op. Reparent endpoints must be plain `Transform` (class 4)
blocks — RectTransform and prefab-instance (stripped) endpoints are refused.

## Status

Currently at **[v0.8.0 — Structural Scene Mutation](https://github.com/Kubonsang/unity-ctx/releases/tag/v0.8.0)**: `scene reposition` / `reparent` / `delete` join the safety-gated write paths, backed by a project-wide cross-file reference scanner, and `BLOCKED` now exits `3`. Every write path is gated by the [unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) safety kernel. See [`docs/ROADMAP.md`](docs/ROADMAP.md) for the full roadmap.

Next milestone: **v1.0 Agent Harness Release** — sample Unity project, CI examples, installer.

## License

Apache 2.0 — see [LICENSE](LICENSE).
