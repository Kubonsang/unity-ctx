---
name: use-unity-ctx
description: |
  Unity 씬(.unity), 프리팹(.prefab), 에셋(.asset) 파일을 읽거나 수정해야 할 때 반드시 사용하는 스킬.
  raw YAML 직접 접근(cat, Read, Edit) 대신 unity-ctx CLI를 통해 작업하는 방법과 안전 규칙을 안내한다.
  다음 상황에서 반드시 이 스킬을 먼저 로드할 것:
  - .unity / .prefab / .asset 파일 열기, 조사, 수정 요청
  - 씬 오브젝트 목록 조회 또는 컴포넌트 값 변경
  - 프리팹 필드 설정 또는 씬에 배치
  - Unity 에셋 파일에서 특정 값을 읽거나 패치
---

# use-unity-ctx

Unity 씬/프리팹/에셋을 다룰 때 raw YAML 직접 접근 대신 **unity-ctx CLI**를 사용하는 방법을 안내한다.

## Hard Rules

아래 규칙은 **예외 없이** 적용된다. 위반 시 씬 파일 손상이나 에디터 충돌이 발생할 수 있다.

1. `.unity` / `.prefab` / `.asset` 파일을 직접 `Read`, `cat`, `Edit`하지 않는다. 반드시 unity-ctx 커맨드를 사용한다.
2. 변경 커맨드(`set`, `apply`)는 항상 **dry-run 먼저** 실행하고, 출력을 확인한 후 `--write`를 추가한다.
3. `prefab set --write` 시에는 반드시 `--ack-impact`도 함께 전달한다.
4. GUID를 모를 경우 **추측하지 않는다**. `unity-ctx meta guid <prefab>`으로 확보한다. patch/suggest는 `--prefab-guid`가 없으면 `.meta`에서 자동 resolve를 시도하고, 실패하면 `UNKNOWN NEED_PREFAB_GUID`로 남는다 — GUID 확보 전까지 `apply`하지 않는다.
5. 오브젝트 특정은 이름 대신 **fileID**를 사용한다. 이름 사용 시 `ERROR AMBIGUOUS_NAME`이 발생할 수 있다.
6. `scene apply` 전에 반드시 `scene diff`로 patch 내용을 확인한다.
7. **`BLOCKED`는 우회하지 않는다.** 모든 write 커맨드는 fileid-graph 안전 검증(`pre_check`/`temp_check`/`final_check`)을 통과해야 한다. `BLOCKED code=GRAPH_CHECK_FAILED`가 나오면 파일이 구조적으로 깨진 것이므로, raw YAML 편집으로 우회하지 말고 원인을 보고한다.
8. Editor 의존성은 커맨드별로 다르다:
   - **Editor 필요**: `scene scan`
   - **Editor 불필요**: 나머지 전부 (`summarize`, `query`, `inspect`, `get`, `set`, `reposition`, `patch`, `diff`, `apply`, `impact`, `refs`, `meta guid`, `suggest`)

## 빠른 작업 패턴

### 씬 오브젝트 조사

```bash
# 1. 전체 구조 파악
unity-ctx scene summarize Assets/Scenes/Stage01.unity

# 2. 이름/타입으로 fileID 확보
unity-ctx scene query Stage01.unity --name Enemy
# FOUND id=1234567890 name=Enemy type=GameObject

# 3. fileID로 특정해서 inspect / get
unity-ctx scene inspect Stage01.unity --id 1234567890 --component Rigidbody
unity-ctx scene get Stage01.unity --id 1234567890 --component Rigidbody --field mass
```

### 에셋 값 읽기 / 수정

```bash
# 값 읽기
unity-ctx asset get EnemyConfig.asset --field maxHealth

# dry-run 먼저 — pre_check/temp_check 결과까지 확인
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 200
# DRY_RUN field=maxHealth old=100 new=200 type_hint=int changed=1 pre_check=OK temp_check=OK

# 확인 후 실제 반영
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 200 --write
# WRITE backup=EnemyConfig.asset.bak ... pre_check=OK temp_check=OK final_check=OK
```

### 프리팹 수정

```bash
# 1. 영향 범위 파악
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab --project <프로젝트 루트>

# 2. dry-run (impact 요약 + safety check 포함)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab --project <프로젝트 루트> \
  --id 11400000 --field moveSpeed --value 4.0

# 3. 반영 (--ack-impact 필수)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab --project <프로젝트 루트> \
  --id 11400000 --field moveSpeed --value 4.0 --write --ack-impact
```

### 프리팹 배치 (씬 패치)

```bash
# 1. GUID 확보 (patch/suggest가 자동 resolve하지만 명시 확인 가능)
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project <프로젝트 루트>

# 2. patch 생성 (scan→suggest 경유 또는 직접)
unity-ctx scene patch Stage01.unity --op place_prefab \
  --manifest /tmp/Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab --position 5,0,0 --json

# 3. 반드시 diff로 내용 확인
unity-ctx scene diff Stage01.unity --patch /tmp/chair.patch.json

# 4. 확인 후 apply (pre/temp/final check 자동 수행)
unity-ctx scene apply Stage01.unity --patch /tmp/chair.patch.json --write
```

### 씬 오브젝트 이동 / 재부모화 / 삭제 (구조 변형, v0.8)

```bash
# 이동 — --id는 Transform fileID (inspect --component Transform으로 확보)
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4          # dry-run
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --write

# 재부모화 — patch → diff → apply, --new-parent 0 = 씬 루트
unity-ctx scene patch Stage01.unity --op reparent --id 4001 --new-parent 4002 --json > /tmp/rp.patch.json
unity-ctx scene diff  Stage01.unity --patch /tmp/rp.patch.json
unity-ctx scene apply Stage01.unity --patch /tmp/rp.patch.json --write --ack-impact

# 삭제 — --id는 GameObject fileID, --write는 --project 필수(교차 파일 검증)
unity-ctx scene patch Stage01.unity --op delete --id 1001 --cascade --json > /tmp/del.patch.json
unity-ctx scene diff  Stage01.unity --patch /tmp/del.patch.json
unity-ctx scene apply Stage01.unity --patch /tmp/del.patch.json --write --ack-impact --project <프로젝트 루트>
```

- 사이클/고아/프리팹-인스턴스/교차파일 참조는 write 전에 `BLOCKED`로 거부된다 — 우회 금지.
- 자세한 가드 목록: `references/commands.md`의 "Structural Scene Mutation Commands".

### 참조 추적

```bash
# 파일이 어떤 fileID/GUID를 참조하는지 raw YAML 없이 확인
unity-ctx prefab refs Assets/Prefabs/Enemy.prefab
unity-ctx scene refs Assets/Scenes/Stage01.unity --json
```

## 오류 대응

| 코드 | 의미 | 대응 |
|-----------|------|------|
| `ERROR AMBIGUOUS_NAME` | 이름으로 오브젝트를 특정할 수 없음 | `query`로 fileID 확보 후 재시도 |
| `NEED_PREFAB_GUID` | GUID를 `.meta`에서 찾지 못함 | `unity-ctx meta guid` 실행, `.meta` 파일 존재 확인. 추측 금지 |
| `BLOCKED code=GRAPH_CHECK_FAILED` | 파일 구조 손상으로 write 거부 | raw YAML 우회 금지. `CHECK`/`ERROR` 라인을 사용자에게 보고 |
| `BLOCKED code=WOULD_CREATE_CYCLE` | reparent가 계층 사이클을 만듦 | 타깃 서브트리 밖의 부모를 선택 |
| `BLOCKED code=WOULD_ORPHAN_CHILDREN` | 자식 있는 오브젝트 삭제 시도 | `--cascade` 추가 또는 자식 먼저 reparent |
| `BLOCKED code=CROSS_FILE_REFERENCED` | 다른 파일이 삭제 대상을 참조 | 우회 금지. 보고된 `files=` 목록의 참조를 먼저 제거/재지정 |
| `ERROR PATCH_STALE` | patch 생성 후 씬이 변경됨 | 현재 씬 기준으로 patch 재생성 |
| `ERROR WRITE_COMMITTED ... phase=final_check` | write 후 검증 실패 | 출력된 `backup=` 경로의 `.bak`으로 복원 |
| `WARN` (check) | 비차단 경고 (unknown class 등) | 진행 가능. write 전 관련 블록 inspect 권장 |
| `ERROR SCAN_EDITOR_FAILED` | Editor 미실행/통신 실패 | Unity Editor를 실행하고 재시도 |

## 패턴에 없는 작업 요청 시

위 패턴에 해당하지 않는 작업(예: 씬에 프리팹 인스턴스 배치, 컴포넌트 추가 등)은 커맨드를 추측해서 실행하지 않는다. 반드시 아래 참조 문서를 먼저 읽고 올바른 커맨드를 확인한 뒤 실행한다.

## 참조 문서 라우팅

더 자세한 내용이 필요하면 아래 파일을 읽는다. (프로젝트 로컬 경로)

| 필요한 것 | 파일 경로 |
|-----------|-----------|
| 커맨드 전체 레퍼런스 | `.claude/skills/use-unity-ctx/references/commands.md` |
| 작업별 워크플로우 | `.claude/skills/use-unity-ctx/references/workflows.md` |
| 서브에이전트 템플릿 | `.claude/skills/use-unity-ctx/references/subagent-prompts.md` |

> **Note**: 위 참조 파일들은 프로젝트 로컬에 위치한다. 해당 파일이 없으면 사용자에게 알린다.
