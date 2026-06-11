# Command Contracts

## Global Shape

```bash
unity-ctx <namespace> <command> <file> [flags]
```

Namespaces:

- `scene`
- `prefab`
- `asset`

## Exit Codes

- `0`: OK / WARN / UNKNOWN only
- `1`: ERROR condition
- `2`: tool execution error

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

Dry-run output:

```text
DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1
```

Write output:

```text
WRITE backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1
```

No-op write output:

```text
OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1
```

Committed-write failure output:

```text
ERROR WRITE_COMMITTED backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=0 err=...
```

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
ERROR suggest does not accept --id, --name, --type, --component, --field, --value, --write, --project, --scenes, --prefabs, --position, --op, --task, --focus, --max-tokens, --patch, --ack-impact, or --mode
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

Dry-run output:

```text
DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

Write output:

```text
WRITE backup=Assets/Prefabs/Enemy.prefab.bak field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

Write-gate failure:

```text
ERROR set requires --ack-impact for prefab writes
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

Compact output examples:

```text
DRY_RUN patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1
WRITE backup=Stage01.unity.bak patch=patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1
```

Error output:

```text
ERROR apply requires --patch
ERROR apply supports only --view compact
ERROR PATCH_STATUS_UNRESOLVED status=UNKNOWN reason=NEED_PREFAB_GUID
ERROR APPLY_VERIFY_FAILED expected_objects=2 actual_objects=1
ERROR patch scene mismatch file=Stage01.unity patch_file=OtherScene.unity
```

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

## Output Stability Rules

- No timestamps in default output.
- Sort by fileID or path.
- Compact output is default.
- Detail output is debug-only.
- JSON output should be deterministic.
