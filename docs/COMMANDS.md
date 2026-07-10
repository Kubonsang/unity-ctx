# Command Contracts

## Global Shape

```bash
unity-ctx <namespace> <command> <file> [flags]
```

Namespaces:

- `scene`
- `prefab`
- `asset`

## Version

```bash
unity-ctx --version    # or -v
```

Prints `unity-ctx <version>` and exits 0. Source builds report `dev`; release
binaries embed the git tag (injected via
`-ldflags "-X github.com/Kubonsang/unity-ctx/internal/version.Version=<tag>"`).
The MCP server's `serverInfo.version` reports the same value.

## Help

```bash
unity-ctx --help                       # overview: namespaces + command lists
unity-ctx scene suggest --help         # per-command synopsis + flags
```

`--help`/`-h` anywhere on the line prints usage and exits `0`. With a known
command present it prints that command's synopsis and flags; otherwise the
general overview.

## Argument diagnostics

Malformed invocations name the real problem instead of a generic error:

```text
unity-ctx bench Stage01.unity       → ERROR "bench" is a command, not a namespace — did you omit the namespace? e.g. unity-ctx scene bench ...
unity-ctx scenez summarize x.unity  → ERROR unknown namespace "scenez" (expected scene, prefab, asset, meta, mcp)
unity-ctx scene frobnicate x.unity  → ERROR unknown command "frobnicate" for namespace "scene"
unity-ctx scene summarize           → ERROR missing file argument
```

## Exit Codes

- `0`: OK / WARN / UNKNOWN / NEED_PREFAB_GUID
- `1`: ERROR condition
- `2`: tool execution error (incl. usage errors: unknown/omitted namespace or command, missing file)
- `3`: BLOCKED — a safety check refused the mutation before any write; the file is untouched

`BLOCKED` exits `3` (not `0`) so an agent can never mistake a safety-policy
refusal for a completed write. `NEED_PREFAB_GUID` stays `0` because it is a
missing precondition, not a refused mutation. Post-write graph corruption is an
`ERROR` (exit `1`): the file was already modified.

## Output Prefixes

- `OK`
- `WARN`
- `ERROR`
- `UNKNOWN`
- `FOUND`
- `OMITTED`
- `INDEX_STALE`
- `DRY_RUN`
- `WRITE`
- `BLOCKED` — write refused by a graph-check failure (`code=GRAPH_CHECK_FAILED phase=...`); never bypass by editing raw YAML
- `CHECK` — per-phase graph-check detail line (`phase=... status=... errors=N warnings=M`)
- `REF` — one PPtr/GUID reference evidence line from `refs`
- `NEED_PREFAB_GUID` — GUID could not be resolved from `.meta`; supply `--prefab-guid` or fix the meta file

## Write Command Policy

Every write command (`asset set`, `prefab set`, `scene reposition`, `scene apply`) follows:

- dry-run first; `--write` required
- target by fileID, not name
- `pre_check` before mutation, `temp_check` before commit, `final_check` after commit
- graph-check `ERROR` blocks the write; `WARN` is surfaced but does not block
- backup path printed for every committed write
- ambiguous or unresolvable input returns `NEED_*`/`BLOCKED`/`UNKNOWN`, never a guess

## v0.1 Commands

### summarize

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx asset summarize Assets/Configs/EnemyConfig.asset
```

### query

```bash
unity-ctx scene query Stage01.unity --id 12003
unity-ctx scene query Stage01.unity --name Chair
unity-ctx scene query Stage01.unity --type GameObject
```

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

## v0.3 Mutation Slice

### asset set

```bash
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write
unity-ctx asset set EnemyConfig.asset --id 11400000 --field moveSpeed --value 4.0
```

Required flags:

- `--field`
- `--value`

Optional flags:

- `--id`
- `--write`
- `--json`
- `--view`

Rules:

- `set` is implemented only for the `asset` namespace.
- `--write` is required for actual file mutation.
- `.bak` is created only when a write actually changes the file.
- `changed=0` write requests return success without mutating the filesystem.
- `--write` and `--value` are rejected for non-`set` commands.
- `set` runs the fileid-graph integrity check on the input (`pre_check`), the
  candidate bytes (`temp_check`), and the re-read file after a committed write
  (`final_check`). A blocking `ERROR` before the write returns `BLOCKED
  code=GRAPH_CHECK_FAILED` (exit 3) without touching the file; after the write it
  returns `ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check` with
  the backup path (exit 1). `WARN` does not block.
- `final_check` is defense-in-depth: because `temp_check` already validated the
  exact bytes written, the only realistic way it fails is a **concurrent
  external modification** of the file between the write and the re-read. unity-ctx
  therefore does **not** auto-revert on `final_check` failure — silently
  restoring `.bak` would discard that external edit. It surfaces
  `WRITE_COMMITTED` and leaves recovery to an explicit `unity-ctx <ns> restore`.

Dry-run output:

```text
DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK
```

Write output:

```text
WRITE backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK
```

No-op write output:

```text
OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1 pre_check=OK temp_check=OK
```

Blocked output (exit 3, file untouched):

```text
BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check file=EnemyConfig.asset field=maxHealth
CHECK phase=pre_check status=ERROR errors=1 warnings=0
ERROR code=DUPLICATE_FILE_ID file_id=11400000 duplicates=2
```

Committed-write failure output:

```text
ERROR WRITE_COMMITTED backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=0 err=...
ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1
```

All `WRITE_COMMITTED` lines carry the standard set summary fields
(`old=`, `new=`, `type_hint=`, `changed=`, `verified=`) so agents parse one shape.

## v0.4 Foundation Slice

### scene scan

```bash
unity-ctx scene scan Stage01.unity --mode editor --project /Users/me/MyUnityProject --out /private/tmp/Stage01.bounds.json
unity-ctx scene scan Stage01.unity --mode editor --project /Users/me/MyUnityProject --prefabs Assets/Prefabs/Chair.prefab,Assets/Prefabs/Table.prefab --out /private/tmp/Stage01.bounds.json --json
```

Required flags:

- `--mode`
- `--project`
- `--out`

Optional flags:

- `--prefabs`
- `--json`

Rules:

- `scan` is implemented only for the `scene` namespace.
- `scan` supports only `--mode editor`.
- `scan` supports only compact output.
- `<file>` must point to a scene file under the provided Unity project `Assets/` tree.
- `--prefabs` is optional and comma-separated. Duplicates are ignored after normalization.
- The Editor payload scene must exactly match the resolved requested `Assets/...` scene path.
- `--json` returns the normal envelope only. The manifest artifact is written to `--out`.

Compact output:

```text
OK mode=editor project=/Users/me/MyUnityProject scene=Assets/Scenes/Stage01.unity out=/private/tmp/Stage01.bounds.json objects=2 prefabs=2 source=editor
```

Error output:

```text
ERROR scan requires --mode
ERROR scan supports only --mode editor
ERROR scan requires --project
ERROR scan requires --out
ERROR scan supports only --view compact
ERROR scene must be under project Assets/ file=/tmp/OutsideScene.unity project=/Users/me/MyUnityProject
ERROR scan payload scene mismatch requested=Assets/Scenes/Stage01.unity payload=Assets/Scenes/OtherScene.unity
ERROR SCAN_EDITOR_FAILED project=/Users/me/MyUnityProject scene=Assets/Scenes/Stage01.unity err=...
```

### scene check

```bash
unity-ctx scene check Stage01.unity --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --position 5,0,0
unity-ctx scene check Stage01.unity --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --position 0.8,0,0 --json
```

Required flags:

- `--manifest`
- `--prefab`
- `--position`

Optional flags:

- `--json`

Rules:

- `check` is implemented only for the `scene` namespace.
- `check` supports only compact output.
- `<file>` must point to a readable scene file.
- `--position` must be exactly `x,y,z` with finite numeric values.
- The manifest scene reference must match the requested scene by exact path when possible, otherwise by normalized scene filename plus extension.
- Irrelevant flags are rejected.

Compact output:

```text
OK manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=5,0,0 overlap_ids=none
WARN manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=0.8,0,0 overlap_ids=1000,2000,3000
```

Error output:

```text
ERROR check requires --manifest
ERROR check requires --prefab
ERROR check requires --position
ERROR check requires --position as x,y,z
ERROR check requires finite --position values
ERROR check supports only --view compact
ERROR manifest scene mismatch file=Stage01.unity manifest_scene=OtherScene.unity
```

## v0.4b Patch Planning Slice

### scene patch

```bash
unity-ctx scene patch Stage01.unity --op place_prefab --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --position 5,0,0
unity-ctx scene patch Stage01.unity --op place_prefab --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --position 5,0,0 --json
unity-ctx scene patch Stage01.unity --op place_prefab --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --prefab-guid guid-chair --position 5,0,0
```

Required flags:

- `--op`
- `--manifest`
- `--prefab`
- `--position`

Optional flags:

- `--json`
- `--prefab-guid`
- `--project`

Rules:

- `patch` is implemented only for the `scene` namespace.
- `patch` supports only `--op place_prefab`.
- `patch` supports only compact output.
- `<file>` must point to a readable scene file.
- `--position` must be exactly `x,y,z` with finite numeric values.
- The manifest scene reference must match the requested scene by exact path when possible, otherwise by normalized scene filename plus extension.
- `patch` is currently a read-only patch-plan generator. It does not write scene files.
- Without `--prefab-guid`, the planner first tries to resolve the GUID from the
  prefab's sibling `.meta` file (retrying under `--project` for relative paths).
  `suggest` inherits the same auto-resolve because it delegates to `patch`.
- When neither `--prefab-guid` nor a `.meta` lookup yields a GUID, the planner
  returns `UNKNOWN ... NEED_PREFAB_GUID` and does not guess a GUID.
- With `--prefab-guid`, the planner can return `OK` for clear placement or `WARN` when overlaps are detected.
- `--json` returns a deterministic envelope including `schema_version` and `patch_plan`.

Compact output examples:

```text
UNKNOWN op=place_prefab manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=5,0,0 reason=NEED_PREFAB_GUID overlap_ids=none reserved_fileIDs=2002,2003
PLAN prefab_guid=UNKNOWN append_ops=append:1:2002:GameObject,append:4:2003:Transform

OK op=place_prefab manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=5,0,0 overlap_ids=none reserved_fileIDs=2002,2003
PLAN prefab_guid="guid-chair" append_ops=append:1:2002:GameObject,append:4:2003:Transform

WARN op=place_prefab manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab position=2.1,0,-1.25 overlap_ids=2000 reserved_fileIDs=2002,2003
PLAN prefab_guid="guid-chair" append_ops=append:1:2002:GameObject,append:4:2003:Transform
```

Error output:

```text
ERROR patch requires --op
ERROR patch supports only --op place_prefab
ERROR patch requires --manifest
ERROR patch requires --prefab
ERROR patch requires --position
ERROR patch requires --position as x,y,z
ERROR patch requires finite --position values
ERROR patch supports only --view compact
ERROR manifest scene mismatch file=Stage01.unity manifest_scene=OtherScene.unity
```

## v0.5c Scene Suggest Read-Only Slice

### scene suggest

```bash
unity-ctx scene suggest Stage01.unity --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --near 1000
unity-ctx scene suggest Stage01.unity --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --near Table_01 --align grid --count 2
unity-ctx scene suggest Stage01.unity --manifest Stage01.bounds.json --prefab Assets/Prefabs/Chair.prefab --near 1000 --json
```

Required flags:

- `--manifest`
- `--prefab`
- `--near`

Optional flags:

- `--count`
- `--align`
- `--json`
- `--project` (with `--out`: auto-resolve the prefab GUID from `.meta`; see v0.5d below)

Rules:

- `suggest` is implemented only for the `scene` namespace.
- `suggest` supports only compact output.
- `suggest` is a read-only placement planner. It does not write scene files. With `--out`, it writes a patch file as a side effect (see v0.5d below).
- Actual placement still flows through `scene patch` and then `scene apply` after you choose a candidate.
- `--near` accepts either an anchor `fileID` or an exact object name from the manifest.
- Exact-name anchor matches must resolve to a single object. Ambiguous names return `ERROR AMBIGUOUS_NAME ...`.
- `--count` defaults to `4` when omitted. If provided, it must be `>= 1`. The planner emits at most `4` ranked candidates.
- `--align` defaults to `floor`.
- `suggest` supports only `--align floor|grid`.
- `--align wall` is excluded from v0.5c and is rejected.
- Compact output starts with one summary line, followed by one `CANDIDATE` line per returned suggestion.
- `--json` returns the normal envelope plus a nested `suggest` payload with `manifest`, `prefab`, `anchor`, `align`, `count`, and `candidates`.

Compact output examples:

```text
OK manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab near=1000 align=floor count=4 candidates=4 clear=4 warn=0
CANDIDATE rank=1 direction=east position=1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=2 direction=west position=-1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=3 direction=north position=0,0,1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=4 direction=south position=0,0,-1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01

WARN manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab near=1000 align=floor count=4 candidates=4 clear=0 warn=4
CANDIDATE rank=1 direction=east position=1.4,0,0 status=WARN overlap_ids=3000 anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=2 direction=west position=-1.4,0,0 status=WARN overlap_ids=4000 anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=3 direction=north position=0,0,1 status=WARN overlap_ids=5000 anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=4 direction=south position=0,0,-1 status=WARN overlap_ids=6000 anchor_id=1000 anchor_name=Table_01
```

JSON shape:

```json
{
  "status": "OK",
  "namespace": "scene",
  "command": "suggest",
  "file": "Stage01.unity",
  "view": "compact",
  "body": "OK manifest=Stage01.bounds.json prefab=Assets/Prefabs/Chair.prefab near=1000 align=grid count=2 candidates=2 clear=2 warn=0\nCANDIDATE rank=1 direction=east position=1.5,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\nCANDIDATE rank=2 direction=west position=-1.5,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01",
  "suggest": {
    "status": "OK",
    "manifest": "Stage01.bounds.json",
    "prefab": "Assets/Prefabs/Chair.prefab",
    "anchor": {
      "id": 1000,
      "name": "Table_01"
    },
    "align": "grid",
    "count": 2,
    "candidates": [
      {
        "rank": 1,
        "direction": "east",
        "position": [1.5, 0, 0],
        "status": "OK",
        "overlap_ids": []
      },
      {
        "rank": 2,
        "direction": "west",
        "position": [-1.5, 0, 0],
        "status": "OK",
        "overlap_ids": []
      }
    ]
  }
}
```

Error output:

```text
ERROR suggest requires --manifest
ERROR suggest requires --prefab
ERROR suggest requires --near
ERROR suggest requires --count >= 1
ERROR suggest supports only --align floor|grid
ERROR suggest supports only --view compact
ERROR suggest does not accept --id, --name, --type, --component, --field, --value, --write, --scenes, --prefabs, --position, --op, --task, --focus, --max-tokens, --patch, --ack-impact, or --mode
ERROR missing anchor near="Missing"
ERROR AMBIGUOUS_NAME name="Table_01" matches=2
ERROR missing prefab manifest entry for path="Assets/Prefabs/Missing.prefab"
```

## v0.5d Suggest-to-Patch Handoff

### scene suggest — patch output flags

```bash
# Write patch for top candidate (rank 1) with GUID
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --prefab-guid abc-guid-123 \
  --out chair.patch.json
# stdout: ...CANDIDATE lines...
# PATCH_OUT rank=1 file=chair.patch.json status=WARN candidate_status=OK

# Select a specific candidate rank
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --prefab-guid abc-guid-123 \
  --out chair.patch.json \
  --pick 2

# Full flow: suggest → diff → apply
unity-ctx scene diff Stage01.unity --patch chair.patch.json
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write
```

- `--out <file>` triggers patch output: writes a diff/apply-compatible patch artifact for the selected candidate rank.
- `--pick <n>` (default `1`) selects which candidate rank to write. Requires `--out`.
- `--prefab-guid <guid>` embeds the GUID in the written patch. Requires `--out`. Without it, the patch has `status=UNKNOWN`.
- `--project <root>` lets suggest auto-resolve the prefab GUID from its `.meta` file when `--prefab-guid` is omitted (relative prefab paths are retried under the project root). Same behavior as `scene patch --project`. If resolution fails, the patch stays `status=UNKNOWN` — never a guess.
- `--pick` and `--prefab-guid` are rejected when `--out` is not set.
- Without `--out`, suggest output is byte-for-byte identical to v0.5c behavior.
- The written patch file is identical in schema to `scene patch --json` output and is usable by `scene diff` and `scene apply --write`.
- `suggest` never writes to the `.unity` scene file. Scene changes are always done by `scene apply --write`.
- An `UNKNOWN` patch (no `--prefab-guid`) is for inspection/diff and cannot be applied until the GUID is known.
  It follows the existing `scene apply` safety policy for UNKNOWN patches.

### PATCH_OUT status vs candidate_status

The `PATCH_OUT` line reports two distinct statuses:

- `status` — the patch planning result from `scene patch` semantics (overlap checked against all objects including the anchor).
- `candidate_status` — the suggest planner result (anchor excluded from overlap check so the selected position remains usable).

`suggest` excludes the anchor from overlap checks; `scene patch` does not.
This means `candidate_status=OK` and `status=WARN` can appear together for the same position.
`PATCH_OUT status` is the patch status, not the candidate status.
v0.5d does not unify these two semantics.

### Error reference (v0.5d additions)

```
ERROR suggest --pick requires --out
ERROR suggest --prefab-guid requires --out
ERROR suggest requires --pick >= 1
ERROR suggest --pick N is out of range, candidates=M
```

## v0.5a Prefab Impact Foundation

### prefab impact

```bash
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject --scenes Assets/Scenes/BossRoom.unity,Assets/Scenes/Stage01.unity
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject --json
```

Required flags:

- `--project`

Optional flags:

- `--scenes`
- `--json`

Rules:

- `impact` is implemented only for the `prefab` namespace.
- `impact` supports only compact output.
- `impact` is read-only impact analysis. It does not mutate prefab, scene, or asset files.
- `<file>` must point to a prefab file under the provided Unity project `Assets/` tree.
- `--scenes` is optional and comma-separated. When provided, impact analysis limits scene hits to that scope.
- `--json` returns the normal envelope plus a nested `impact` payload.
- Nested prefab traversal is capped at depth `3`.
- When nested traversal reaches the cap and more nested references may exist, status becomes `WARN` and an additional depth-limit warning line is emitted.

Compact output examples:

```text
OK prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

Depth-limit warning suffix:

```text
WARN prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=2 scene_refs=3 prefabs=4 prefab_refs=5 nested_depth=3
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001 Assets/Prefabs/EnemyBoss.prefab refs=1 fileIDs=3000 Assets/Prefabs/EnemyUltra.prefab refs=1 fileIDs=3000 Assets/Prefabs/EnemyLegend.prefab refs=1 fileIDs=3000
WARN IMPACT_DEPTH_LIMIT prefab=Assets/Prefabs/Enemy.prefab depth=3 more_possible=true
```

JSON shape:

```json
{
  "namespace": "prefab",
  "command": "impact",
  "file": "Assets/Prefabs/Enemy.prefab",
  "view": "compact",
  "status": "OK",
  "body": "OK prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1\nSCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\nPREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001",
  "impact": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/Enemy.prefab",
    "prefab_guid": "fake_enemy_guid",
    "scene_hits": [
      {
        "path": "Assets/Scenes/BossRoom.unity",
        "references": 1,
        "file_ids": [4000]
      },
      {
        "path": "Assets/Scenes/Stage01.unity",
        "references": 2,
        "file_ids": [1000, 2000]
      }
    ],
    "prefab_hits": [
      {
        "path": "Assets/Prefabs/EnemyElite.prefab",
        "references": 2,
        "file_ids": [3000, 3001]
      }
    ],
    "depth_limit_hit": false,
    "max_nested_depth": 1
  }
}
```

Error output:

```text
ERROR impact not implemented for namespace=scene
ERROR impact supports only --view compact
ERROR impact requires --project
ERROR impact does not accept --id, --name, --type, --component, --field, --value, --write, --manifest, --prefab, --position, --op, --prefab-guid, --task, --focus, --max-tokens, --out, --mode, --prefabs, or --patch
ERROR prefab must be under project Assets/ file=/tmp/Outside.prefab project=/Users/me/MyUnityProject
ERROR prefab file not found: /Users/me/MyUnityProject/Assets/Prefabs/Missing.prefab
ERROR prefab meta not found file=/Users/me/MyUnityProject/Assets/Prefabs/Enemy.prefab
```

## v0.5b Prefab Set Impact-First

### prefab set

```bash
unity-ctx prefab set Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject --id 11400000 --field moveSpeed --value 4.0
unity-ctx prefab set Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject --id 11400000 --field moveSpeed --value 4.0 --write --ack-impact
unity-ctx prefab set Assets/Prefabs/Enemy.prefab --project /Users/me/MyUnityProject --id 11400000 --field moveSpeed --value 4.0 --json
```

Required flags:

- `--project`
- `--id`
- `--field`
- `--value`

Optional flags:

- `--write`
- `--ack-impact`
- `--json`

Rules:

- `set` is implemented for the `prefab` namespace with the command shape `unity-ctx prefab set <prefab> --project <project> --id <fileID> --field <field> --value <value>`.
- Targeting is fileID-only. `--name` and `--component` selection are not supported for `prefab set`.
- `prefab set` defaults to dry-run. Dry-run output includes the field mutation summary plus project-scoped impact summary and `ack_required`.
- Changing writes require `--write --ack-impact`. Without `--ack-impact`, changing write requests fail with `ERROR set requires --ack-impact for prefab writes`.
- `prefab set --json` may include a nested `impact` payload alongside the normal result envelope. `asset set` JSON shape is unchanged.
- The impact scan is project-scoped for the provided `--project` and reuses the same nested depth warning behavior as `prefab impact`.
- When nested traversal reaches the current depth cap, the command keeps the set summary and appends `WARN IMPACT_DEPTH_LIMIT ...`.
- `prefab set` runs the fileid-graph integrity check on the input (`pre_check`)
  and the candidate bytes (`temp_check`) before the `--ack-impact` gate, and on
  the re-read file after a committed write (`final_check`). A blocking `ERROR`
  before the write returns `BLOCKED code=GRAPH_CHECK_FAILED` (exit 3); after the
  write it returns `ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED
  phase=final_check` with the backup path (exit 1). `WARN` does not block.

Dry-run output:

```text
DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1 pre_check=OK temp_check=OK
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

Write output:

```text
WRITE backup=Assets/Prefabs/Enemy.prefab.bak field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 pre_check=OK temp_check=OK final_check=OK
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

Write-gate failure:

```text
ERROR set requires --ack-impact for prefab writes
```

Blocked output (exit 3, file untouched, judged before the ack gate):

```text
BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check file=Assets/Prefabs/Enemy.prefab id=11400000 field=moveSpeed
CHECK phase=pre_check status=ERROR errors=1 warnings=0
ERROR code=DUPLICATE_FILE_ID file_id=2000 duplicates=2
```

Depth-limit warning suffix:

```text
DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=WARN scenes=2 scene_refs=3 prefabs=1 prefab_refs=1 nested_depth=3 ack_required=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=1 fileIDs=3000
WARN IMPACT_DEPTH_LIMIT prefab=Assets/Prefabs/Enemy.prefab depth=3 more_possible=true
```

JSON shape:

```json
{
  "namespace": "prefab",
  "command": "set",
  "file": "Assets/Prefabs/Enemy.prefab",
  "view": "compact",
  "status": "OK",
  "body": "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1\nSCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\nPREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001",
  "impact": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/Enemy.prefab",
    "prefab_guid": "fake_enemy_guid",
    "scene_hits": [
      {
        "path": "Assets/Scenes/BossRoom.unity",
        "references": 1,
        "file_ids": [4000]
      },
      {
        "path": "Assets/Scenes/Stage01.unity",
        "references": 2,
        "file_ids": [1000, 2000]
      }
    ],
    "prefab_hits": [
      {
        "path": "Assets/Prefabs/EnemyElite.prefab",
        "references": 2,
        "file_ids": [3000, 3001]
      }
    ],
    "depth_limit_hit": false,
    "max_nested_depth": 1
  }
}
```

Error output:

```text
ERROR set requires --project
ERROR set requires --id
ERROR set requires non-zero --id
ERROR set requires --field
ERROR set requires --value
ERROR set requires --ack-impact for prefab writes
ERROR set does not accept --name, --type, --component, --out, --task, --focus, --max-tokens, --scenes, --mode, --prefabs, --manifest, --prefab, --position, --op, --prefab-guid, or --patch
```

## v0.4c Apply + Diff Foundation Slice

### scene diff

```bash
unity-ctx scene diff Stage01.unity --patch patches/chair_place_ok.patch.json
unity-ctx scene diff Stage01.unity --patch patches/chair_place_ok.patch.json --json
```

Required flags:

- `--patch`

Optional flags:

- `--json`

Rules:

- `diff` is implemented only for the `scene` namespace.
- `diff` supports only compact output.
- `diff` reads the persisted JSON emitted by `scene patch --json`.
- The patch file must have `schema_version=1`.
- The patch file scene reference must match the requested scene by exact path when possible, otherwise by normalized scene filename plus extension.

Compact output examples:

```text
OK patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 reserved_fileIDs=2002,2003
WARN patch=patches/chair_place_warn.patch.json op=place_prefab overlap_ids=2000 append_ops=2 reserved_fileIDs=2002,2003
UNKNOWN patch=patches/chair_place_unknown.patch.json op=place_prefab reason=NEED_PREFAB_GUID append_ops=2 reserved_fileIDs=2002,2003
```

Error output:

```text
ERROR diff requires --patch
ERROR diff supports only --view compact
ERROR patch scene mismatch file=Stage01.unity patch_file=OtherScene.unity
ERROR invalid patch file: schema_version must be 1
```

### scene apply

```bash
unity-ctx scene apply Stage01.unity --patch patches/chair_place_ok.patch.json
unity-ctx scene apply Stage01.unity --patch patches/chair_place_ok.patch.json --write
unity-ctx scene apply Stage01.unity --patch patches/chair_place_ok.patch.json --json
```

Required flags:

- `--patch`

Optional flags:

- `--write`
- `--json`

Rules:

- `apply` is implemented only for the `scene` namespace.
- `apply` supports only compact output.
- `apply` is dry-run-first. It does not write unless `--write` is provided.
- `apply` accepts only the current append-only `place_prefab` patch contract.
- `apply` creates `<scene>.bak` before any committed write.
- `apply` reparses the written scene and verifies the appended object fileIDs before reporting success.
- `apply` does not proceed on `UNKNOWN` patch status.
- `apply` runs the fileid-graph integrity check three times: on the target scene
  (`pre_check`), on the candidate bytes before commit (`temp_check`), and on the
  re-read file after a committed write (`final_check`).
- A blocking `ERROR` in `pre_check` or `temp_check` returns `BLOCKED
  code=GRAPH_CHECK_FAILED` (exit 3) and never touches the file — including dry-run.
- A blocking `ERROR` in `final_check` returns `ERROR WRITE_COMMITTED
  code=GRAPH_CHECK_FAILED phase=final_check backup=<path>` (exit 1). The write has
  already been committed; restore from the printed backup path.
- `WARN` check results do not block. The phase status is reported and `CHECK` +
  `WARN` detail lines are appended for review.

Compact output examples:

```text
DRY_RUN patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK
WRITE backup=Stage01.unity.bak patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK
```

Blocked output (exit 3, file untouched, no backup created):

```text
BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check patch=patches/chair_place_ok.patch.json file=Stage01.unity
CHECK phase=pre_check status=ERROR errors=1 warnings=0
ERROR code=DUPLICATE_FILE_ID file_id=1000 duplicates=2
```

Error output:

```text
ERROR apply requires --patch
ERROR apply supports only --view compact
ERROR PATCH_STATUS_UNRESOLVED status=UNKNOWN reason=NEED_PREFAB_GUID
ERROR APPLY_VERIFY_FAILED expected_objects=2 actual_objects=1
ERROR patch scene mismatch file=Stage01.unity patch_file=OtherScene.unity
ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=Stage01.unity.bak patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1
```

All `WRITE_COMMITTED` lines carry the standard summary fields
(`op=`, `append_ops=`, `changed=`, `verified=`) so agents can parse one shape.

JSON output adds a `safety` object when checks ran:

```json
"safety": {
  "pre_check": "OK",
  "temp_check": "OK",
  "final_check": "OK",
  "findings": []
}
```

Each finding carries `phase`, `severity`, `code`, and `detail`.
`BLOCKED` responses still include `patch_plan` and the `safety` payload, so a
JSON consumer always sees the same envelope shape regardless of verdict.

## v0.2x Bench Backfill

### bench

```bash
unity-ctx scene bench Assets/Scenes/Stage01.unity
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety"
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety" --json
```

- `bench` is a token reduction benchmark, not a performance benchmark.
- token estimation uses `ceil(utf8_bytes / 4)`.
- `bench` always measures raw vs `summarize`.
- `bench` measures `context-pack` only when `--task` is present.
- `bench` does not use an external tokenizer.
- `bench` does not use Unity Editor integration.
- `bench` requires `--view compact` (the default).

## v0.6 Safety Integration

### meta guid

```bash
unity-ctx meta guid Assets/Prefabs/Chair.prefab
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project /Users/me/MyUnityProject
unity-ctx meta guid Assets/Prefabs/Chair.prefab --json
```

Optional flags:

- `--project` — retried as `<project>/<file>` when the path is relative and not found directly
- `--json`

Rules:

- `guid` is the only command in the `meta` namespace.
- The GUID is read from the sibling `<file>.meta`; the asset file itself is not parsed.
- A missing `.meta` or missing `guid:` entry returns `NEED_PREFAB_GUID` with a
  `reason` (`meta_not_found` | `guid_missing`), exit 0. The tool never guesses.

Output:

```text
OK guid=3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b file=Assets/Prefabs/Chair.prefab meta=Assets/Prefabs/Chair.prefab.meta
NEED_PREFAB_GUID file=Assets/Prefabs/Chair.prefab reason=meta_not_found
```

### refs

```bash
unity-ctx prefab refs Assets/Prefabs/Enemy.prefab
unity-ctx scene refs Assets/Scenes/Dungeon.unity
unity-ctx asset refs Assets/Configs/EnemyConfig.asset --json
```

Optional flags:

- `--json`

Rules:

- `refs` is implemented for the `scene`, `prefab`, and `asset` namespaces.
- `refs` is read-only PPtr/GUID evidence extraction backed by the
  unity-fileid-graph safety kernel; the REF line dialect matches `uyaml refs`.
- Field paths are best-effort evidence labels, not a YAML AST path contract.
- `WARN` status means warning-only extraction issues; exit code stays 0.

Output:

```text
OK refs file=Assets/Prefabs/Enemy.prefab count=2 warnings=0
REF block=1000 class=GameObject field=m_Component[0].component file_id=2000
REF block=3000 class=MonoBehaviour field=m_Script file_id=11500000 guid=a1b2c3d4e5f60718293a4b5c6d7e8f90 type=3
```

WARN output (warning-only extraction issue, exit 0):

```text
WARN refs file=Assets/Prefabs/Enemy.prefab count=0 warnings=1
WARN code=UNKNOWN_FIELD_SHAPE file_id=11400000 message="unsupported PPtr fileID"
```

`--json` adds a `refs` payload with `references[]`
(`block_file_id`, `class`, `field`, `file_id`, `guid?`, `type?`), a `warnings`
count, and `issues[]` carrying the warning detail (`severity`, `code`,
`file_id?`, `message?`) — mirroring `uyaml refs --json` so agents can read *why*
a result is `WARN` without parsing the text body.

### validate

```bash
unity-ctx prefab validate Assets/Prefabs/Enemy.prefab
unity-ctx scene validate Assets/Scenes/Dungeon.unity --json
```

Optional flags:

- `--json`

Rules:

- `validate` is implemented for the `scene`, `prefab`, and `asset` namespaces.
- It is the **read-only** form of the unity-fileid-graph integrity check that
  gates every write path — run it to confirm a file is structurally sound before
  editing. Nothing is mutated.
- First-line status is the kernel verdict: `OK` (sound), `WARN` (non-blocking
  issues like unknown class IDs), or `ERROR` (broken graph: duplicate fileIDs,
  missing component/GameObject blocks, back-reference mismatch, ...).
- Exit codes: `OK`/`WARN` → `0`, `ERROR` → `1` (matches `uyaml check`).

Output:

```text
OK validate file=Assets/Configs/EnemyConfig.asset blocks=1 gameobjects=0 components=1 transforms=0 errors=0 warnings=0

WARN validate file=Assets/Prefabs/Enemy.prefab blocks=4 gameobjects=1 components=2 transforms=1 errors=0 warnings=1
WARN code=UNKNOWN_CLASS_ID file_id=4000 message="graph build skipped unsupported class id"

ERROR validate file=Stage01.unity blocks=3 gameobjects=1 components=1 transforms=1 errors=1 warnings=0
ERROR code=DUPLICATE_FILE_ID file_id=1000 duplicates=2
```

`--json` adds a `validate` payload with the counts plus `findings[]`
(`severity`, `code`, `detail?`).

### restore

```bash
unity-ctx asset restore EnemyConfig.asset
unity-ctx scene restore Stage01.unity --json
```

Optional flags:

- `--json`

Rules:

- `restore` is implemented for the `scene`, `prefab`, and `asset` namespaces.
- It overwrites `<file>` with its sibling `<file>.bak` — the pre-write backup
  every committed mutation (`set`/`apply`) leaves behind — recovering the
  prior state. The write is atomic.
- `check=` reports the integrity status of the restored content (`OK`/`WARN`/
  `ERROR`) so an agent knows what state it recovered to.
- Exit `0` on success; `1` if no `<file>.bak` exists or the write fails.

Output:

```text
OK restore file=EnemyConfig.asset backup=EnemyConfig.asset.bak bytes=213 check=OK
ERROR restore no backup found backup=EnemyConfig.asset.bak
```

`--json` adds a `restore` payload (`backup`, `bytes`, `check`).

### changes

```bash
unity-ctx asset changes EnemyConfig.asset
unity-ctx scene changes Stage01.unity --json
```

Optional flags:

- `--json`

Rules:

- `changes` is implemented for the `scene`, `prefab`, and `asset` namespaces.
- It diffs `<file>` against its sibling `<file>.bak` (the backup the last
  committed `set`/`apply` left) by matching blocks on fileID, reporting
  `ADDED`/`REMOVED`/`CHANGED` per object. Read-only.
- Errors with exit `1` if no `<file>.bak` exists. Output is deterministic
  (sorted by fileID).

Output:

```text
OK changes file=EnemyConfig.asset vs=EnemyConfig.asset.bak added=0 removed=0 changed=1
CHANGED fileID=11400000 type=MonoBehaviour
```

`--json` adds a `changes` payload (`backup`, `added`, `removed`, `changed`,
`edits[]` with `kind`/`file_id`/`type`).

### deps

```bash
unity-ctx prefab deps Assets/Prefabs/Enemy.prefab --project /path/to/project
unity-ctx scene deps Assets/Scenes/Dungeon.unity --project . --out deps.dot
unity-ctx asset deps Assets/Mats/Wood.mat --project . --json
```

Required flags:

- `--project`

Optional flags:

- `--out` (write a Graphviz DOT graph to the given file)
- `--json`

Rules:

- `deps` is implemented for the `scene`, `prefab`, and `asset` namespaces.
- It lists the **external asset dependencies** of a file: the GUIDs it
  references (extracted via the safety kernel) resolved to asset paths by
  scanning the project's `.meta` files. Read-only.
- A GUID with no matching `.meta` under `--project` is reported `path=UNKNOWN`
  (e.g. a script whose `.cs.meta` is absent) — never guessed.
- Output is deterministic (dependencies sorted by GUID). Exit `0`.

Output:

```text
OK deps file=Assets/Prefabs/Box.prefab project=. refs=2 resolved=1 unresolved=1
DEP guid=0123456789abcdef0123456789abcdef path=Assets/Materials/Wood.mat
DEP guid=ffffffffffffffffffffffffffffffff path=UNKNOWN
DOT_OUT file=deps.dot
```

`--out` writes a `digraph deps { ... }` (file → dependency edges; unresolved
targets shown as `guid:<g>`), pipeable to `dot -Tpng`. `--json` adds a `deps`
payload (`project`, `refs`, `resolved`, `unresolved`, `dependencies[]`).

### mcp

```bash
unity-ctx mcp
```

Runs an [MCP](https://modelcontextprotocol.io) server over stdio (newline-delimited
JSON-RPC 2.0), so MCP hosts (Claude Code, etc.) can call unity-ctx's read-only
commands as native tools instead of shelling out.

Rules:

- Takes no file argument; reads JSON-RPC requests from stdin, writes responses to stdout.
- Implements `initialize`, `tools/list`, `tools/call`, `ping`.
- Exposes **read-only** tools only — mutations stay behind the CLI's
  dry-run-first `--write` contract: `unity_summarize`, `unity_validate`,
  `unity_refs`, `unity_query`, `unity_get`, `unity_deps`, `unity_impact`.
- A tool whose underlying command exits non-zero returns `isError: true` with
  the command output as text content.

Example session (stdin → stdout):

```text
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"unity-ctx","version":"0.6.0"}}}
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"unity_validate","arguments":{"namespace":"prefab","file":"Enemy.prefab"}}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"OK validate ..."}],"isError":false}}
```

Register in Claude Code with `claude mcp add unity-ctx -- unity-ctx mcp`.

## v0.8 Structural Scene Mutation Slice

### scene reposition

```bash
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --write
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --json
```

Sets a Transform's `m_LocalPosition` to a new `x,y,z`. This is the first
*structural* scene mutation: unlike `scene apply` (append-only prefab placement),
it edits an existing block in place. It is **topology-invariant** — only the
three numeric axis tokens of the one inline `m_LocalPosition: {x, y, z}` mapping
change, so the fileID graph is untouched and the safety kernel's pre/temp/final
checks pass for any input that was already sound.

Required flags:

- `--id` — the **Transform** fileID whose `m_LocalPosition` is rewritten
  (non-zero). It must address a block whose Unity class is a transform —
  `Transform` (4) or `RectTransform` (224); any other class is refused at the
  class stage with `ERROR UNSUPPORTED_TARGET_CLASS field=m_LocalPosition id=N
  class=<id> allowed=4,224` (exit 1, file untouched), even if that block happens
  to carry its own `m_LocalPosition`. A transform-class block that lacks
  `m_LocalPosition` (e.g. a stripped prefab Transform) returns
  `ERROR FIELD_NOT_FOUND`.
- `--position x,y,z` — three comma-separated finite floats.

Optional flags:

- `--write`
- `--json`

Rules:

- `reposition` is implemented only for the `scene` namespace and only on
  `.unity` files; other namespaces return `ERROR reposition not implemented for
  namespace=<ns>` (exit 2).
- The rewrite preserves every byte of the target line except the three axis
  values: brace placement, comma/space separators, key order, and per-entry
  whitespace all survive. Non-target fields and all other blocks are byte-identical.
- It refuses any value that is not exactly `{x, y, z}` of numbers
  (`FIELD_NOT_VECTOR3`), so a misaddressed field (e.g. a Quaternion `{x,y,z,w}`)
  fails loudly rather than being mangled.
- Same three-phase graph-check + `.bak` + no-auto-revert `final_check` policy as
  `asset set` (see [Write Command Policy](#write-command-policy)).
- `changed=0` requests (position already equal) return success without mutating
  the filesystem or creating a `.bak`.
- **Limitation (by design, this slice):** it edits the addressed block's raw
  `m_LocalPosition`. For an object that is a **prefab instance**, the effective
  position override lives in that instance's `PrefabInstance.m_Modifications`,
  not the Transform block — so repositioning the raw Transform may have no
  visual effect. Stripped prefab Transforms have no `m_LocalPosition` and return
  `FIELD_NOT_FOUND`. Prefab-instance and cross-file position semantics are out of
  scope here. Works as expected on plain (non-instance) scene objects and on
  `RectTransform` (its `m_LocalPosition` is also a Vector3).

Dry-run output:

```text
DRY_RUN id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 pre_check=OK temp_check=OK
```

Write output:

```text
WRITE backup=Stage01.unity.bak id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK
```

No-op write output:

```text
OK id=1001 field=m_LocalPosition old=5,0,3 new=5,0,3 changed=0 verified=1 pre_check=OK temp_check=OK
```

Blocked output (exit 3, file untouched):

```text
BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check file=Stage01.unity id=1001 field=m_LocalPosition
CHECK phase=pre_check status=ERROR errors=1 warnings=0
ERROR code=DUPLICATE_FILE_ID file_id=1000 duplicates=2
```

### scene reparent (v2 ops[] patch)

```bash
unity-ctx scene patch Stage01.unity --op reparent --id 4001 --new-parent 4002 --json > reparent.patch.json
unity-ctx scene diff  Stage01.unity --patch reparent.patch.json
unity-ctx scene apply Stage01.unity --patch reparent.patch.json --write --ack-impact
```

Moves a Transform within one scene: sets the target's `m_Father` and updates the
old and new parents' `m_Children` atomically (one file, one `.bak`). `--new-parent
0` moves the target to the scene root.

- **v2 patch schema.** `scene patch --op reparent` emits a `schema_version: 2`
  patch with an `ops[]` array (`op: reparent`), coexisting with v1 (`place_prefab`,
  `patch_plan`); `scene diff`/`scene apply` accept both and dispatch on
  `schema_version`. One reparent op per patch (no op mixing) in this slice.
- **`m_Children` is written in Unity's real F3 form** (dash at the key's indent,
  `- {fileID: N}`); removing the last child collapses to `m_Children: []`. Other
  bytes are preserved.
- **Allowed endpoint class = `Transform` (4) only** — narrower than `reposition`
  (which also allows `RectTransform` 224). reposition is topology-invariant
  (coordinates only) so it needs no hierarchy modeling; reparent changes the
  hierarchy and relies on the kernel's symmetry/cycle modeling, which covers class
  4 only. A `RectTransform` (224), a stripped (nested prefab-instance) endpoint, or
  any non-Transform endpoint is refused:
  `BLOCKED reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=<role> id=<N> class=<C> is_stripped=<bool> allowed=4`
  (exit 3, file untouched). Editing such an endpoint's hierarchy raw would be an
  unverifiable, silently-invalid write.
- **Dry-run `plan` phase** (earliest phase): if the reparent would create a cycle
  (new parent is the target or a descendant), it is refused before any write —
  `BLOCKED phase=plan code=WOULD_CREATE_CYCLE chain=a->b->a`. `temp_check` (the
  graph-check) is the backstop behind it.
- **`--ack-impact` required** for `apply --write` (reparent changes topology).
  Same dry-run-first / `.bak` / `pre`/`temp`/`final` / no-auto-revert contract as
  `set`/`apply`. Verify asserts: target's `m_Father` == new parent; new parent
  lists the target; old parent no longer lists it.
- A stale patch (its `old_parent` no longer matches the scene) is rejected
  (`ERROR PATCH_STALE`).

**Cross-file reference report (visibility, not a block).** Pass `--project DIR`
to `scene apply` and it runs a per-mutation reverse-reference scan (no cache) of
the project — both `Assets/` and `Packages/` (embedded/local packages can hold
PPtrs) — for inbound PPtrs to the moved object, surfacing them on the result.
The scan covers the **whole moved object** — its Transform, its GameObject, and
every component — because external referrers usually point at the GameObject or a
component, not the Transform fileID. Inline PPtrs are recovered in every
serialization form (block, inline, flow sequences, multiline-flow, nested,
quoted) via a brace-aware raw scan, so no form is silently missed:

```text
... cross_file_check=ok inbound_refs=N indeterminate=M
WARN REPARENT_HAS_INBOUND_REFS count=N files=Assets/Other.unity,...
WARN REPARENT_INDETERMINATE_REFS count=M files=...
```

- These are **informational, never blocking**: reparent *moves* the object, so its
  fileID stays valid and external PPtrs are not dangled. The WARN exists so an
  agent knows the move may have semantic impact (e.g. world-space/parent-scale
  assumptions in the referencing object) — which is not a graph-integrity fact.
- `indeterminate` = files whose references could not be fully accounted for:
  a parse failure, a target-GUID mention not recovered as a structured PPtr, an
  unreadable directory/file, a **symlinked directory** (WalkDir does not descend
  it), or an anomalously **binary** object asset (`.prefab`/`.unity`/`.mat`/… that
  is not text under Force Text). Reported conservatively (never a silent "no refs")
  but still **not** a block for reparent. Baked binary `.asset` data (LightingData,
  NavMesh) is intentionally out of scan scope — it is regenerated by Unity's bake
  and flagging it would block on every project — so a (regenerable) reference held
  only in a binary `.asset` is the one documented exception to "never silent".
- This deliberately differs from `delete` (below), which *removes* the fileID and
  so **BLOCKS** on the same inbound/indeterminate signals (then the references
  genuinely dangle).
- The scan is skipped (and says so explicitly, so a passing reparent is never
  misread as "cross-file verified") with a stated reason:
  `cross_file_check=skipped reason=<no_project|no_change|no_meta|no_assets_root|scan_error>`.
  `no_change` = a no-op reparent (target already under the requested parent), so
  nothing moved and the project is not walked.
- `--project` applies only to v2 `ops[]` patches (reparent/delete). Passing it to a
  v1 `place_prefab` apply is an explicit error (`ERROR --project applies only to
  reparent (v2 ops) patches`), never silently ignored.

### scene delete (v2 ops[] patch)

```bash
unity-ctx scene patch Stage01.unity --op delete --id 1001 [--cascade] --json > delete.patch.json
unity-ctx scene diff  Stage01.unity --patch delete.patch.json
unity-ctx scene apply Stage01.unity --patch delete.patch.json --write --ack-impact --project DIR
```

Removes a GameObject and its component blocks from one scene, unlinking its
Transform from the parent's `m_Children` (one file, one `.bak`). `--id` is the
**GameObject** fileID.

- **Target is a non-stripped GameObject.** A Transform/component/other class, or a
  stripped (prefab-instance) GameObject, is refused:
  `BLOCKED reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=target id=<N> class=<C> is_stripped=<bool> allowed=1`.
- **`--cascade` removes the whole Transform subtree.** Without it, deleting an
  object that still has children is refused
  (`BLOCKED phase=plan code=WOULD_ORPHAN_CHILDREN`).
- **Plan-phase guards** (all `BLOCKED phase=plan`, exit 3, file untouched):
  `STRIPPED_IN_SUBTREE` (a prefab-instance block is in the removed set — its
  overrides live elsewhere, raw removal would corrupt the link); `PARENT_STRIPPED`
  / `PARENT_NOT_FOUND` (the parent's `m_Children` cannot be edited); and
  `IN_FILE_REFERENCED` (a surviving same-file PPtr still points at a removed
  fileID — the graph-check has no dangling validator, so this is enforced here).
- **Root objects update `SceneRoots`.** A root-level object's Transform is
  registered in the scene's `SceneRoots` (`!u!1660057539`) `m_Roots` list, not in
  any parent's `m_Children`; the delete unlinks it there too.
- **Cross-file references BLOCK** (unlike reparent's visibility-only report):
  removing the fileIDs would dangle inbound PPtrs, and an indeterminate referrer
  cannot be proven safe. On `--write` either condition yields
  `BLOCKED code=CROSS_FILE_REFERENCED ... inbound_refs=N indeterminate=M`
  (file untouched). A dry-run shows `block_on_write=1` so the block is previewed
  (including the scan-failure case `BLOCKED code=CROSS_FILE_SCAN_FAILED`).
- **Reference detection is serialization-agnostic.** Both the cross-file scan and
  the in-file dangling check combine a parsed-tree walk (block-style + clean inline
  maps) with `parser.ScanInlinePPtrs` — a brace-aware scan of the raw bytes that
  recovers inline PPtrs in *any* form the line parser renders opaque: flow
  sequences `[{...}]` (flat, multi-item, multiline-flow list items), nested
  sub-braces `{fileID: N, m_Range: {…}, guid: G}`, and quoted keys/values. A
  same-file ref is one with no guid OR the scene's own guid (so a self-qualified
  `{fileID: N, guid: <this scene>}` is caught too). The goal: no PPtr form yields a
  silent "no refs".
- **Limitation**: a *non-empty flow-style* `m_Children`/`m_Roots` on the target's
  parent (e.g. `m_Children: [{fileID: N}]`, rare in modern Unity) is not *rewritten*
  for the unlink — the op fails safe (ERROR/BLOCKED, no write), same as reparent.
  (This is about editing the parent list, not detecting references, which is robust.)
- **`--project` is REQUIRED for `--write`** (`ERROR delete --write requires
  --project`): a committed delete is always cross-file-verified. A dry-run without
  `--project` reports `cross_file_check=skipped reason=no_project` and runs only
  the in-file checks.
- **`--ack-impact` required** for `apply --write`. Same dry-run-first / `.bak` /
  `pre`/`temp`/`final` / no-auto-revert contract. Verify is an **absence
  assertion**: every removed fileID is gone and the parent no longer lists the
  target.

## Output Stability Rules

- No timestamps in default output.
- Sort by fileID or path.
- Compact output is default.
- Detail output is debug-only.
- JSON output should be deterministic.
