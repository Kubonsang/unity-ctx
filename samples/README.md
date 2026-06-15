# unity-ctx Samples

A small but structurally valid Unity project, `MiniDungeon/`, used to demo the
full unity-ctx agent workflow end to end. Every `.unity` / `.prefab` / `.asset`
file here passes the unity-fileid-graph safety kernel.

## Project layout

```
MiniDungeon/
  Assets/
    Scenes/
      Dungeon.unity          # Floor, SpawnPoint (+ spawner script), Crate_01, Chair_01
    Prefabs/
      Chair.prefab(.meta)    # GameObject + Transform + MonoBehaviour
      Crate.prefab(.meta)    # GameObject + Transform
    Configs/
      EnemyConfig.asset      # MonoBehaviour ScriptableObject (maxHealth, moveSpeed)
  ProjectSettings/
    ProjectVersion.txt       # m_EditorVersion (reads like a real project root)
  Dungeon.bounds.json        # placement manifest for `scene suggest` / `patch`
```

## Build

```bash
go build -o /tmp/unity-ctx ./cmd/unity-ctx
```

All commands below are run from the repository root. The leading token in each
"expected" line is the output contract prefix (`OK` / `FOUND` / `REF` / `DRY_RUN`
/ ...). A non-blocking `WARN` is fine; `BLOCKED` / `ERROR` is not.

## Demo command sequence

### 1. summarize — cheap, token-safe overview

```bash
/tmp/unity-ctx scene summarize samples/MiniDungeon/Assets/Scenes/Dungeon.unity
```
Expected: `OK SCENE file=... game_objects=4 components=5 unknown=0`

### 2. query — find objects by name or type

```bash
/tmp/unity-ctx scene query samples/MiniDungeon/Assets/Scenes/Dungeon.unity --name Crate_01
/tmp/unity-ctx scene query samples/MiniDungeon/Assets/Scenes/Dungeon.unity --type GameObject
```
Expected: `FOUND fileID=3000 type=GameObject name="Crate_01"`
and `FOUND type=GameObject matches=4 fileIDs=1000,2000,3000,4000`

### 3. refs — list every fileID/GUID reference (graph-validated)

```bash
/tmp/unity-ctx prefab refs samples/MiniDungeon/Assets/Prefabs/Chair.prefab
```
Expected: `OK refs file=... count=3 warnings=0` followed by `REF ...` lines.

### 4. impact — what depends on this prefab

```bash
/tmp/unity-ctx prefab impact samples/MiniDungeon/Assets/Prefabs/Chair.prefab --project samples/MiniDungeon
```
Expected: `OK prefab=Assets/Prefabs/Chair.prefab guid=a1b2c3d4... scenes=0 scene_refs=0 ...`

### 5. meta guid — resolve a prefab's GUID from its `.meta`

```bash
/tmp/unity-ctx meta guid samples/MiniDungeon/Assets/Prefabs/Chair.prefab --project samples/MiniDungeon
```
Expected: `OK guid=a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6 file=... meta=...`

### 6. suggest — plan placement candidates around an anchor

```bash
/tmp/unity-ctx scene suggest samples/MiniDungeon/Assets/Scenes/Dungeon.unity \
  --manifest samples/MiniDungeon/Dungeon.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near SpawnPoint --align grid --count 4
```
Expected: `OK manifest=... prefab=Assets/Prefabs/Chair.prefab near=2000 align=grid count=4 candidates=4 clear=4 warn=0`
followed by `CANDIDATE ...` lines. (`--near` takes an object name or a fileID.)

`suggest` is read-only. To hand a chosen candidate to the write path, add
`--out` (auto-resolving the prefab GUID from the project's `.meta` files):

```bash
/tmp/unity-ctx scene suggest samples/MiniDungeon/Assets/Scenes/Dungeon.unity \
  --manifest samples/MiniDungeon/Dungeon.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near SpawnPoint --align grid \
  --project samples/MiniDungeon \
  --out /tmp/chair.patch.json
```
Expected (last line): `PATCH_OUT rank=1 file=/tmp/chair.patch.json status=WARN candidate_status=OK`

> Note: `candidate_status` excludes the anchor from the overlap check, so it can
> be `OK` while the patch `status` is `WARN` (patch semantics include the anchor).
> This is the documented v0.5d behavior, not a failure.

### 7. diff — preview a patch against the scene (no write)

```bash
/tmp/unity-ctx scene diff samples/MiniDungeon/Assets/Scenes/Dungeon.unity --patch /tmp/chair.patch.json
```
Expected: `WARN patch=/tmp/chair.patch.json op=place_prefab overlap_ids=2000 append_ops=2 reserved_fileIDs=4002,4003`
(`OK`/`WARN` line — overlap is informational, not blocking.)

### 8. apply (dry-run) — safety-checked, still no write

```bash
/tmp/unity-ctx scene apply samples/MiniDungeon/Assets/Scenes/Dungeon.unity --patch /tmp/chair.patch.json
```
Expected: `DRY_RUN patch=... op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK`

Add `--write` to actually commit (creates a `.bak`, runs the final graph check):
`WRITE backup=... pre_check=OK temp_check=OK final_check=OK`. Run this against a
copy of the project if you want to keep the sample pristine.

## Token savings (measured)

The `bench` command reports real numbers for these files:

```bash
/tmp/unity-ctx scene bench samples/MiniDungeon/Assets/Scenes/Dungeon.unity
```
Example: `OK raw_tokens=389 summarize_tokens=25 summarize_ratio=0.06 ...`
— roughly a 15x reduction versus feeding the agent the raw YAML, before any
context-pack focusing.
