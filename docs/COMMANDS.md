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

## Output Stability Rules

- No timestamps in default output.
- Sort by fileID or path.
- Compact output is default.
- Detail output is debug-only.
- JSON output should be deterministic.
