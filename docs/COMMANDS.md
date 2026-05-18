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

Rules:

- `patch` is implemented only for the `scene` namespace.
- `patch` supports only `--op place_prefab`.
- `patch` supports only compact output.
- `<file>` must point to a readable scene file.
- `--position` must be exactly `x,y,z` with finite numeric values.
- The manifest scene reference must match the requested scene by exact path when possible, otherwise by normalized scene filename plus extension.
- `patch` is currently a read-only patch-plan generator. It does not write scene files.
- Without `--prefab-guid`, the planner returns `UNKNOWN ... NEED_PREFAB_GUID` and does not guess a GUID.
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

## Output Stability Rules

- No timestamps in default output.
- Sort by fileID or path.
- Compact output is default.
- Detail output is debug-only.
- JSON output should be deterministic.
