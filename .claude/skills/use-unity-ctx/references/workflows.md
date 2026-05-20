# unity-ctx Workflows

unity-ctx를 사용하는 4가지 주요 시나리오와 각 시나리오의 커맨드 시퀀스, 안티패턴.

---

## 1. 씬/프리팹 조사 (Inspect & Report)

Unity 파일의 구조와 특정 오브젝트/컴포넌트/필드를 파악할 때.

### 플로우

```bash
# 1. 전체 구조 파악
unity-ctx scene summarize Assets/Scenes/Stage01.unity

# 2. 이름/타입으로 fileID 확보
unity-ctx scene query Stage01.unity --name Chair
unity-ctx scene query Stage01.unity --type NavMeshAgent

# 3. 특정 오브젝트의 컴포넌트 상세 조회 (fileID 사용 권장)
unity-ctx scene inspect Stage01.unity --id 12003 --component Rigidbody

# 4. 단일 필드 값만 필요할 때
unity-ctx scene get Stage01.unity --id 12003 --component Rigidbody --field mass
```

prefab, asset도 동일한 패턴:
```bash
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset get EnemyConfig.asset --field maxHealth
```

### 안티패턴

- `.unity` / `.prefab` / `.asset` 파일을 직접 `Read`하거나 `cat`으로 읽는다 — raw YAML은 수만 줄에 달하고 토큰을 낭비한다.
- 이름으로만 오브젝트를 특정한다 — 씬에 같은 이름의 오브젝트가 여럿 있으면 `ERROR AMBIGUOUS_NAME`이 발생한다. `query`로 fileID를 먼저 확보할 것.

---

## 2. 프리팹 배치 (Place Prefab)

씬에 새 프리팹 인스턴스를 배치할 때. **Unity Editor가 해당 프로젝트를 열고 실행 중이어야 한다.**

### 플로우

```bash
# 1. 씬의 bounds manifest 생성 (Editor 필요)
unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --out /tmp/Stage01.bounds.json

# 2. 배치 후보 탐색 + patch 파일 생성
#    GUID는 .prefab.meta 파일의 guid 필드에서 확인
unity-ctx scene suggest Stage01.unity \
  --manifest /tmp/Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --align floor \
  --prefab-guid abc-guid-123 \
  --out /tmp/chair.patch.json

# 3. patch 내용 확인
unity-ctx scene diff Stage01.unity --patch /tmp/chair.patch.json

# 4. 실제 적용 (diff 확인 후)
unity-ctx scene apply Stage01.unity --patch /tmp/chair.patch.json --write
```

### GUID 확보 방법

프리팹 GUID는 `.prefab` 파일 옆의 `.meta` 파일에 있다:
```bash
grep "^guid:" Assets/Prefabs/Chair.prefab.meta
# guid: abc-guid-123
```

GUID를 모르면 `--prefab-guid` 없이 suggest를 실행한다. patch가 `UNKNOWN` 상태로 저장되며
`scene apply`는 UNKNOWN patch를 거부하므로 GUID 확보 전까지 apply할 수 없다.

### 안티패턴

- `scan` 없이 `suggest`를 실행한다 — manifest 없이는 배치 후보를 계산할 수 없다.
- GUID 없이 `apply`를 시도한다 — `UNKNOWN` 상태 patch는 apply에서 `ERROR PATCH_STATUS_UNRESOLVED`로 거부된다.
- `diff` 없이 바로 `apply --write`를 실행한다 — patch 내용을 먼저 확인할 것.
- `suggest` 결과를 보고 수동으로 `.unity` 파일을 편집한다 — `scene apply`를 사용할 것.

---

## 3. 에셋/프리팹 필드 수정 (Modify Field)

`.asset`, `.mat`, `.prefab` 파일의 특정 필드 값을 바꿀 때.

### Asset 수정 플로우

```bash
# 1. 현재 값 확인
unity-ctx asset get EnemyConfig.asset --field maxHealth

# 2. dry-run으로 변경 내용 미리 보기
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300

# 3. 출력이 예상과 같으면 실제 적용
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write
```

### Prefab 수정 플로우

```bash
# 1. 영향 범위 먼저 파악
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject

# 2. fileID 확보 (모를 경우)
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent

# 3. dry-run (impact 요약 자동 포함)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field speed \
  --value 4.0

# 4. impact 범위 확인 후 실제 적용
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 \
  --field speed \
  --value 4.0 \
  --write --ack-impact
```

### 안티패턴

- `prefab set --write` 시 `--ack-impact` 없이 실행한다 — `ERROR set requires --ack-impact for prefab writes`로 거부된다.
- impact depth WARN(`WARN IMPACT_DEPTH_LIMIT`)을 무시하고 진행한다 — depth cap(3)을 초과한 참조가 더 있을 수 있다. 영향 범위가 명확하지 않으면 수정을 보류할 것.
- `--id` 없이 `prefab set`을 실행한다 — prefab set은 fileID 전용이며 `--name`/`--component` 미지원.

---

## 4. 프리팹 영향 분석 (Prefab Impact)

프리팹을 수정하기 전에 어떤 씬/프리팹이 이 프리팹을 참조하는지 파악할 때.

### 플로우

```bash
# 프로젝트 전체 스캔
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject

# 특정 씬만 범위 한정
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --scenes Assets/Scenes/BossRoom.unity,Assets/Scenes/Stage01.unity
```

### 출력 해석

```
OK prefab=... guid=... scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1
SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000
PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001
```

- `SCENES` 라인: 이 프리팹을 참조하는 씬과 각 씬의 참조 fileID 목록
- `PREFABS` 라인: 이 프리팹을 참조하는 다른 프리팹 목록
- `nested_depth`: 순회한 중첩 depth (최대 3)
- `WARN IMPACT_DEPTH_LIMIT`: depth cap 도달, 더 많은 참조가 있을 수 있음

### 안티패턴

- impact 없이 프리팹을 수정한다 — 참조 씬이 많으면 예상치 못한 런타임 변화가 생긴다.
- `WARN IMPACT_DEPTH_LIMIT`을 무시한다 — 표시된 것보다 더 많은 참조가 존재할 수 있다.
