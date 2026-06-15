# Dev Smoke Test

A concrete, end-to-end recipe to confirm a build of `unity-ctx` works across all
namespaces. Every command below runs against the committed fixtures under
`testdata/`, so no Unity Editor and no external project are required.

Each step lists the **expected first-line prefix** — automated callers branch on
that prefix, so a wrong prefix means the build is broken. The example output was
captured from a real run against the repo fixtures.

## 0. Build

```bash
go build -o /tmp/unity-ctx ./cmd/unity-ctx
```

Expect no output and exit `0`. (You can also use `go run ./cmd/unity-ctx ...`
in place of `/tmp/unity-ctx` below; the smoke uses a built binary for speed.)

## 1. Three-namespace smoke

Exercises the `scene`, `prefab`, and `asset` namespaces with one read command each.

### scene summarize → `OK`

```bash
/tmp/unity-ctx scene summarize testdata/scenes/simple_scene.unity
```

```text
OK SCENE file=testdata/scenes/simple_scene.unity game_objects=2 components=2 unknown=0
```

### prefab refs → `OK`

```bash
/tmp/unity-ctx prefab refs testdata/prefabs/enemy.prefab
```

```text
OK refs file=testdata/prefabs/enemy.prefab count=4 warnings=0
REF block=1000 class=GameObject field=m_Component[0].component file_id=2000
REF block=1000 class=GameObject field=m_Component[1].component file_id=3000
REF block=1000 class=GameObject field=m_Component[2].component file_id=4000
REF block=3000 class=MonoBehaviour field=m_Script file_id=11500000 guid=a1b2c3d4e5f60718293a4b5c6d7e8f90 type=3
```

### asset get → `OK`

```bash
/tmp/unity-ctx asset get testdata/assets/enemy_config.asset --field maxHealth
```

```text
OK field=maxHealth value=200
```

## 2. Write dry-run → `DRY_RUN`

Confirms the mutation path plans a change and runs the pre/temp graph checks
**without** touching the file (no `--write`).

```bash
/tmp/unity-ctx asset set testdata/assets/enemy_config.asset --field maxHealth --value 300
```

```text
DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK
```

The fixture is left unchanged (still `maxHealth: 200`) because no `--write` was
passed. Re-running `asset get` should still report `value=200`.

## 3. Meta GUID → `OK`

Confirms `.meta` GUID resolution against the bundled impact-project fixture.

```bash
/tmp/unity-ctx meta guid testdata/impact/project/Assets/Prefabs/Enemy.prefab \
  --project testdata/impact/project
```

```text
OK guid=fake_enemy_guid file=testdata/impact/project/Assets/Prefabs/Enemy.prefab meta=testdata/impact/project/Assets/Prefabs/Enemy.prefab.meta
```

> The fixture's `.meta` carries the placeholder `guid: fake_enemy_guid`; a real
> project returns the actual 32-char hex GUID. The point of the smoke is the
> `OK` prefix and that the GUID is read (never guessed).

## 4. Full test suite → all `ok`

```bash
go test ./...
```

Every package prints `ok` (or `[no test files]` for `cmd/unity-ctx` and
`internal/core`):

```text
?   	unity-ctx/cmd/unity-ctx	[no test files]
ok  	unity-ctx/internal/app	...
ok  	unity-ctx/internal/bench	...
ok  	unity-ctx/internal/cli	...
ok  	unity-ctx/internal/parser	...
...
```

The `internal/cli` integration suite is the slowest (~2 min); the rest finish in
a few seconds each.

## Pass criteria

| Step | Expected first-line prefix |
|---|---|
| 0. build | (no output, exit 0) |
| 1a. scene summarize | `OK` |
| 1b. prefab refs | `OK` |
| 1c. asset get | `OK` |
| 2. write dry-run | `DRY_RUN` |
| 3. meta guid | `OK` |
| 4. `go test ./...` | every package `ok` |

If any prefix differs from the table above, or any package fails `go test`, the
build is not healthy — do not ship it.
