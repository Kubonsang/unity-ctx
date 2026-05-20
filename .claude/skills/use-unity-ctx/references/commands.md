# unity-ctx Command Reference

## Global Shape

```bash
unity-ctx <namespace> <command> <file> [flags]
```

Namespaces: `scene` | `prefab` | `asset`

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | OK / WARN / UNKNOWN only |
| `1` | ERROR condition |
| `2` | Tool execution error |

## Output Prefix Meanings

| Prefix | Meaning |
|--------|---------|
| `OK` | 성공, 이상 없음 |
| `WARN` | 성공이지만 주의 필요 (예: 배치 후보가 겹침) |
| `ERROR` | 실패, 중단 |
| `UNKNOWN` | 정보 부족으로 결정 불가 (예: GUID 미제공) |
| `DRY_RUN` | dry-run 결과 (파일 미변경) |
| `WRITE` | 실제 파일 변경 완료 |
| `FOUND` | 쿼리 결과 있음 |
| `OMITTED` | 토큰 예산 초과로 생략 |
| `INDEX_STALE` | 인덱스가 파일보다 오래됨 |
| `CANDIDATE` | suggest 배치 후보 |
| `PLAN` | patch 계획 라인 |
| `PATCH_OUT` | suggest --out 결과 라인 |
| `SCENES` / `PREFABS` | impact 결과 라인 |

---

## Read-only Commands

### summarize

씬/프리팹/에셋의 컴팩트 개요 (오브젝트 수, 컴포넌트 타입, PrefabInstance 등).

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx asset summarize Assets/Configs/EnemyConfig.asset
```

### query

이름 / fileID / 타입으로 오브젝트 필터링.

```bash
unity-ctx scene query Stage01.unity --id 12003
unity-ctx scene query Stage01.unity --name Chair
unity-ctx scene query Stage01.unity --type GameObject
unity-ctx prefab query Enemy.prefab --type NavMeshAgent
```

Flags: `--id` | `--name` | `--type`

### inspect

특정 오브젝트의 컴포넌트 필드 상세 조회.

```bash
unity-ctx scene inspect Stage01.unity --id 12003 --component BoxCollider
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset inspect EnemyConfig.asset
```

Flags: `--id` | `--name` | `--component`

### get

단일 필드 값 조회.

```bash
unity-ctx scene get Stage01.unity --id 12003 --component Rigidbody --field mass
unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
unity-ctx asset get EnemyConfig.asset --field maxHealth
```

Required: `--component`, `--field` (asset은 `--component` 불필요)

### bench

raw 파일 대비 summarize / context-pack의 토큰 절감량 측정.

```bash
unity-ctx scene bench Assets/Scenes/Stage01.unity
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety"
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety" --json
```

- `--task` 없으면 raw vs summarize만 측정
- `--task` 있으면 context-pack도 측정

---

## Placement Flow Commands

### scene scan

Unity Editor를 통해 씬의 bounds manifest 생성. **Editor가 해당 프로젝트를 열고 실행 중이어야 한다.**

```bash
unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --out /tmp/Stage01.bounds.json

unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --prefabs Assets/Prefabs/Chair.prefab,Assets/Prefabs/Table.prefab \
  --out /tmp/Stage01.bounds.json
```

Required: `--mode editor`, `--project`, `--out`
Optional: `--prefabs` (쉼표 구분), `--json`

Output:
```
OK mode=editor project=/Users/me/MyUnityProject scene=Assets/Scenes/Stage01.unity out=/tmp/Stage01.bounds.json objects=2 prefabs=2 source=editor
```

### scene check

특정 위치에 프리팹 배치 시 겹침 여부 확인.

```bash
unity-ctx scene check Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --position 5,0,0
```

Required: `--manifest`, `--prefab`, `--position` (x,y,z)

Output:
```
OK manifest=... prefab=... position=5,0,0 overlap_ids=none
WARN manifest=... prefab=... position=0.8,0,0 overlap_ids=1000,2000
```

### scene suggest

배치 후보 위치 랭킹. **읽기 전용.** 실제 씬 변경은 apply로.

```bash
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --align grid \
  --count 4

# patch 파일까지 바로 생성 (v0.5d)
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --prefab-guid abc-guid-123 \
  --out chair.patch.json \
  --pick 1
```

Required: `--manifest`, `--prefab`, `--near` (fileID 또는 오브젝트 이름)
Optional: `--count` (기본 4, 최대 4), `--align floor|grid` (기본 floor), `--json`
Patch output: `--out`, `--pick` (기본 1), `--prefab-guid` — `--pick`과 `--prefab-guid`는 `--out` 없이 사용 불가

Output:
```
OK manifest=... prefab=... near=1000 align=floor count=4 candidates=4 clear=4 warn=0
CANDIDATE rank=1 direction=east position=1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
CANDIDATE rank=2 direction=west position=-1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01
...
PATCH_OUT rank=1 file=chair.patch.json status=WARN candidate_status=OK
```

> `PATCH_OUT status`(patch 시맨틱)와 `candidate_status`(suggest 시맨틱)는 다르다.
> suggest는 앵커를 겹침 체크에서 제외하므로 `candidate_status=OK`여도 `status=WARN`일 수 있다.

### scene patch

배치 patch 계획 생성 (읽기 전용 플래너).

```bash
unity-ctx scene patch Stage01.unity \
  --op place_prefab \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --position 5,0,0 \
  --prefab-guid abc-guid-123
```

Required: `--op place_prefab`, `--manifest`, `--prefab`, `--position`
Optional: `--prefab-guid`, `--json`

- `--prefab-guid` 없으면 `UNKNOWN ... NEED_PREFAB_GUID` 반환
- 씬 파일 미변경

### scene diff

저장된 patch 파일 내용 요약 확인.

```bash
unity-ctx scene diff Stage01.unity --patch chair.patch.json
```

Required: `--patch`

Output:
```
OK patch=... op=place_prefab append_ops=2 reserved_fileIDs=2002,2003
WARN patch=... op=place_prefab overlap_ids=2000 append_ops=2 reserved_fileIDs=2002,2003
UNKNOWN patch=... op=place_prefab reason=NEED_PREFAB_GUID append_ops=2 reserved_fileIDs=2002,2003
```

### scene apply

patch를 씬에 적용. `UNKNOWN` 상태 patch는 적용 불가.

```bash
# dry-run (기본)
unity-ctx scene apply Stage01.unity --patch chair.patch.json

# 실제 적용
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write
```

Required: `--patch`
Optional: `--write`, `--json`

- `--write` 없으면 dry-run
- 쓰기 전 `.bak` 자동 생성
- 쓰기 후 재파싱으로 fileID 검증

Output:
```
DRY_RUN patch=... op=place_prefab append_ops=2 changed=1 verified=1
WRITE backup=Stage01.unity.bak patch=... op=place_prefab append_ops=2 changed=1 verified=1
```

---

## Mutation Commands

### asset set

`.asset` / `.mat` 파일 필드 수정.

```bash
# dry-run
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300

# 실제 적용
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write

# fileID 지정
unity-ctx asset set EnemyConfig.asset --id 11400000 --field moveSpeed --value 4.0 --write
```

Required: `--field`, `--value`
Optional: `--id`, `--write`, `--json`

Output:
```
DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1
WRITE backup=EnemyConfig.asset.bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1
OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1
```

### prefab impact

프리팹이 참조되는 씬/프리팹 목록. 수정 전 영향 범위 파악에 사용.

```bash
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject

unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --scenes Assets/Scenes/BossRoom.unity,Assets/Scenes/Stage01.unity
```

Required: `--project`
Optional: `--scenes` (쉼표 구분), `--json`

- nested 순회 depth cap: 3
- depth 초과 시 `WARN IMPACT_DEPTH_LIMIT` 추가 출력

Output:
```
OK prefab=... guid=... scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

### prefab set

프리팹 필드 수정. **impact 확인 후 진행 권장.**

```bash
# dry-run (impact 자동 포함)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field moveSpeed \
  --value 4.0

# 실제 적용 (--ack-impact 필수)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field moveSpeed \
  --value 4.0 \
  --write --ack-impact
```

Required: `--project`, `--id`, `--field`, `--value`
Optional: `--write`, `--ack-impact`, `--json`

- targeting은 fileID 전용 (`--name`, `--component` 미지원)
- dry-run 출력에 impact 요약 + `ack_required=1` 포함
- `--write` 없이 `--ack-impact` 사용 불가
- `--write` 있고 `--ack-impact` 없으면 `ERROR set requires --ack-impact for prefab writes`

Output:
```
DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 ...
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```
