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

## Output Stability Rules

- No timestamps in default output.
- Sort by fileID or path.
- Compact output is default.
- Detail output is debug-only.
- JSON output should be deterministic.
