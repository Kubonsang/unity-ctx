# 소프트웨어 개발 요건 정의서

# Software Requirements Specification · Rev 5

## unity-ctx

**Unity Context Provider for AI Coding Agents**  
AI 코딩 에이전트를 위한 토큰 절약형 Unity 데이터 접근 및 조작 하니스

| 항목 | 내용 |
|---|---|
| 문서 버전 | v5.0 Codex-ready |
| 기준 문서 | Rev 4 |
| 작성 목적 | Codex가 바로 개발을 시작할 수 있도록 범위, 우선순위, 안전 정책, 테스트 기준을 확정 |
| 구현 언어 | Go 1.22+ |
| 대상 환경 | macOS / Windows / Linux, 단일 바이너리 |
| 라이선스 | MIT |

---

# 1. 프로젝트 정의

## 1.1 정의

`unity-ctx`는 AI 코딩 에이전트가 Unity 프로젝트의 `.unity`, `.prefab`, `.asset` 파일을 **전체 원문으로 읽지 않고**, 작업에 필요한 최소 정보만 조회하도록 돕는 CLI 하니스다.

에이전트는 Unity YAML 파일을 직접 읽거나 편집하지 않는다. `unity-ctx`가 Unity 직렬화 데이터를 목적에 맞게 압축해 제공하고, 수정은 안전한 명령을 통해 수행한다.

핵심 목적은 다음 세 가지다.

1. **Token-aware context**  
   Unity YAML 전체를 프롬프트에 넣지 않고, 작업 목적에 맞는 compact context만 제공한다.

2. **Query-first inspection**  
   에이전트가 `summarize`, `query`, `inspect`, `get`, `context-pack`으로 필요한 정보만 조회하게 한다.

3. **Safe mutation**  
   수정 명령은 dry-run-first, 백업, 영향 범위 확인, 재검증 절차를 따른다.

## 1.2 대상 파일 범위

| 파일 종류 | 주요 내용 | 네임스페이스 |
|---|---|---|
| `.unity` | 씬, GameObject, Transform, 컴포넌트, PrefabInstance, 배치 정보 | `unity-ctx scene ...` |
| `.prefab` | 프리팹 GameObject 계층, 컴포넌트 값, variant 관계 | `unity-ctx prefab ...` |
| `.asset` / `.mat` | ScriptableObject, Material, PhysicsMaterial, 설정 에셋 | `unity-ctx asset ...` |

세 파일은 모두 Unity YAML 직렬화 형식을 공유한다. 파서 코어는 하나로 두고, 커맨드 네임스페이스로 문맥을 구분한다.

## 1.3 핵심 설계 원칙

- **No Raw YAML Reading**  
  에이전트는 `.unity`, `.prefab`, `.asset` 전체 원문을 직접 읽지 않는다.

- **Token Budget**  
  모든 읽기 출력은 명시적 토큰 예산 또는 view level을 가진다.

- **Query-first**  
  전체 파일 조회보다 `get`, `query`, `inspect`, `context-pack`을 우선한다.

- **Read-only MVP first**  
  초기 버전은 수정 기능보다 읽기/압축/조회 기능을 먼저 완성한다.

- **Dry-run-first mutation**  
  파일을 변경하는 모든 명령은 기본 dry-run으로 동작한다. 실제 수정에는 `--write`가 필요하다.

- **fileID-first targeting**  
  수정 명령은 이름보다 `fileID`를 우선한다. 이름 기반 수정은 명시적 fallback 옵션이 필요하다.

- **Impact-aware**  
  프리팹 수정 전 영향 씬 수, 인스턴스 수, variant 목록을 출력한다.

- **UNKNOWN is not OK**  
  판단 불가는 `UNKNOWN`으로 표시하고, 사람 확인 없이 수정하지 않는다.

- **Verifiable Index**  
  index는 캐시가 아니라 `file_hash` 기반 검증 가능한 snapshot이다.

- **Single binary first**  
  기본 도구는 단일 Go 바이너리로 제공한다. Unity Editor 연동은 manifest 생성 등 선택 기능으로 제한한다.

---

# 2. 기대 효과와 측정 기준

## 2.1 토큰 소모 절감

중규모 Unity 프로젝트, 씬 오브젝트 200~500개 기준 추정치다.

| 시나리오 | 직접 읽기 | unity-ctx 경유 |
|---|---:|---:|
| 씬 전체 파악 | `.unity` raw 약 40,000 tok | `summarize` 약 200 tok |
| 프롭 배치 작업 | `.unity` raw 약 40,000 tok | `context-pack` 약 400 tok |
| 특정 오브젝트 조회 | `.unity` raw 약 40,000 tok | `query` 약 80 tok |
| 컴포넌트 값 확인 | `.prefab` raw 약 5,000 tok | `get` / `inspect` 약 30~200 tok |
| 에셋 필드 확인 | `.asset` raw 약 2,000 tok | `get` 약 30 tok |

## 2.2 효과 측정 방법

토큰 수는 외부 tokenizer에 의존하지 않고 다음 추정식을 사용한다.

```text
estimated_tokens = ceil(utf8_bytes / 4)
```

벤치마크 출력 예시:

```text
BENCH Stage01.unity raw=41200tok summarize=210tok ratio=196x
BENCH context-pack task="place chairs" output=386tok ratio=106x
BENCH prefab Enemy.prefab raw=5200tok inspect=180tok ratio=28x
```

벤치마크 명령은 v0.2 이후 제공한다.

```bash
unity-ctx bench Assets/Scenes/Stage01.unity
unity-ctx bench Assets/Prefabs/Enemy.prefab
```

## 2.3 에이전트 오류 감소

| 오류 유형 | 발생 원인 | unity-ctx 대응 |
|---|---|---|
| Unity YAML 손상 | 에이전트 직접 편집 | `set`, `patch/apply`, dry-run, backup |
| 잘못된 오브젝트 수정 | 이름 중복 | fileID 우선, name fallback 제한 |
| 잘못된 타입 입력 | 필드 타입 미확인 | `inspect → get → set` 절차, type hint |
| 프리팹 수정 전파 사고 | prefab과 instance 혼동 | `prefab impact`, impact-first |
| stale context 사용 | 캐시 불일치 | `file_hash` 기반 index 검증 |
| 프롭 겹침 | 좌표 계산 오류 | footprint check |
| rotation 오류 | 허용 범위 외 회전 | rules + `UNKNOWN/NEED_RULE` |

---

# 3. Core MVP Scope

## 3.1 MVP 목표

초기 MVP의 목표는 모든 Unity 조작을 지원하는 것이 아니다. 목표는 다음 하나다.

> 에이전트가 Unity YAML 원문을 직접 읽지 않고도 `.unity`, `.prefab`, `.asset`의 필요한 정보를 조회할 수 있게 한다.

## 3.2 MVP 포함 기능

- Unity YAML block parser
- `scene`, `prefab`, `asset` 네임스페이스
- `summarize`
- `query` by id/name/type/guid
- `inspect`
- `get`
- `index` with `file_hash`
- `context-pack`
- `--view tiny|compact|detail`
- `--json`

## 3.3 MVP 제외 기능

아래 기능은 초기 MVP에서 제외한다.

- `remove_object`
- `prefab set --write`
- `suggest --align wall`
- FBX parser fallback
- 2D OBB
- 3D OBB
- `asset impact`
- 복잡한 MonoBehaviour 타입 검증
- Unity Editor 자동 실행 검증

## 3.4 MVP 완료 기준

1. `.unity`, `.prefab`, `.asset` 원문을 직접 읽지 않고 주요 정보를 조회할 수 있다.
2. `summarize → query → inspect → get` 흐름이 샘플 프로젝트에서 동작한다.
3. `context-pack`이 `--max-tokens` 예산을 지킨다.
4. `index`가 `file_hash` 불일치 시 자동 폐기된다.
5. 에이전트가 사용할 수 있는 AGENTS.md 초안과 최소 SKILL 문서가 제공된다.

---

# 4. 커맨드 구조

## 4.1 역할별 분류

| 분류 | 커맨드 | 설명 | 우선순위 |
|---|---|---|---|
| 읽기/압축 | `context-pack` | 작업용 최소 컨텍스트 번들 | v0.2 |
| 읽기/압축 | `summarize` | 파일 전체를 100~300 tokens 수준으로 요약 | v0.1 |
| 읽기/압축 | `query` | 조건 기반 부분 조회 | v0.1 |
| 읽기/압축 | `index` | `file_hash` 기반 snapshot 생성 | v0.2 |
| Inspector | `inspect` | 컴포넌트/오브젝트 필드 compact 출력 | v0.1 |
| Inspector | `get` | 특정 필드 값 조회 | v0.1 |
| Inspector | `set` | 특정 필드 값 수정, dry-run-first | v0.3 |
| Inspector | `prefab impact` | 프리팹 수정 영향 범위 조회 | v0.5 |
| 검증 | `check` | 배치 sanity check | v0.4 |
| 조작 | `patch` | 씬 수정 patch 생성 | v0.4 |
| 조작 | `apply` | patch를 씬에 적용 | v0.4 |
| 조작 | `diff` | 변경 요약 | v0.4 |
| 조작 | `suggest` | 배치 후보 탐색 | v0.5 |
| 데이터 생성 | `scan` | Editor bounds manifest 생성 | v0.4 |
| 데이터 생성 | `infer-rules` | rotation 규칙 초안 생성 | v1.0 |
| 데이터 생성 | `validate-rules` | rotation 규칙 검증 | v1.0 |
| 측정 | `bench` | 토큰 절감 벤치마크 | v0.2 |

## 4.2 네임스페이스 규칙

```bash
unity-ctx scene <command> <file> [options]
unity-ctx prefab <command> <file> [options]
unity-ctx asset <command> <file> [options]
unity-ctx bench <file> [options]
```

예시:

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab inspect Assets/Prefabs/Enemy.prefab --component NavMeshAgent
unity-ctx asset get Assets/Configs/EnemyConfig.asset --field maxHealth
```

---

# 5. Token-aware Unity Data Parsing

## 5.1 출력 view level

모든 읽기 커맨드는 `--view`를 지원한다. 기본값은 `compact`다.

| View | 포함 정보 | 사용 목적 | 에이전트 자동 사용 |
|---|---|---|---|
| `tiny` | 이름, fileID, 위치/핵심 값 | 빠른 판단 | 허용 |
| `compact` | fileID, prefab, bounds, warning, 타입 | 기본 출력 | 허용 |
| `detail` | GUID, YAML key, component 목록, 부모 계층 | 디버깅 | 기본 금지 |
| `json` | 구조화 JSON | CI/파이프라인 | 허용 |

SKILL 규칙: 에이전트는 사용자가 요청했거나 compact 출력만으로 부족할 때만 `--view detail`을 사용한다. detail 출력을 최종 응답에 그대로 붙여넣지 않는다.

## 5.2 `summarize`

역할: 씬/프리팹/에셋 전체를 매우 짧게 요약한다.

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx asset summarize Assets/Configs/EnemyConfig.asset
```

씬 출력 예시:

```text
SCENE Stage01 ROOTS 12 PROPS 84 PREFABS 23 LIGHTS 4 CAMERAS 2 WARNINGS 3
DENSE_AREAS x=0..5 z=2..8 props=31
KEY_OBJECTS PlayerSpawn@0,0,0 MainCamera@0,6,-10 ExitDoor@18,0,4
WARNINGS GUID_MISSING Barrel_03 OVERLAP Chair_02<->Crate_01
```

프리팹 출력 예시:

```text
PREFAB Enemy COMPONENTS 6 CHILDREN 3 DEPTH 2
COMPONENTS NavMeshAgent Animator EnemyController BoxCollider Rigidbody AudioSource
CHILDREN WeaponRoot VFXRoot HitboxCollider
VARIANT_OF EnemyBase guid=xx12
```

## 5.3 `query`

역할: 조건에 맞는 일부 오브젝트만 반환한다.

지원 필터:

- `--id <fileID>`
- `--name <string>`
- `--type <GameObject|Transform|MonoBehaviour|...>`
- `--guid <guid>`
- `--near <name|fileID>`
- `--radius <m>`
- `--tag <tag>`
- `--layer <layer>`
- `--active true|false`
- `--changed-only`

예시:

```bash
unity-ctx scene query Stage01.unity --near Table_01 --radius 3
unity-ctx scene query Stage01.unity --name Chair --active true
```

출력 예시:

```text
FOUND 2 near=Table_01 r=3
Chair_01 id=12003 pos=2.1,0,3.4 rot=0,90,0
Chair_02 id=12018 pos=2.1,0,4.6 rot=0,90,0
```

## 5.4 `context-pack`

역할: 특정 작업에 필요한 최소 Unity 데이터를 압축하여 출력한다. 에이전트의 씬/프리팹/에셋 작업은 가능한 한 `context-pack`에서 시작한다.

```bash
unity-ctx scene context-pack Stage01.unity --task "place 4 chairs around Table_01"
unity-ctx scene context-pack Stage01.unity --focus Table_01 --radius 4 --max-tokens 400
```

출력 예시:

```text
TASK_CONTEXT scene=Stage01 focus=Table_01 budget=400tok
FOCUS Table_01 id=22011 pos=5,0,3 rot=0,0,0 fp=2.0x1.2
NEARBY r=4
  Lamp_01 id=22031 pos=3.1,0,2.8 fp=0.4x0.4
  Wall_A id=22080 pos=5,0,0 fp=6.0x0.2
PLACEABLE
  Chair_01 guid=ab12 fp=0.8x0.8 yaw=[0,90,180,270]
WARN
  Table_01 near wall_north distance=0.9
  Chair_01 NEED_RULE pitch/roll undefined
```

### 5.4.1 생략 정보

`--max-tokens` 때문에 정보가 생략되면 마지막에 `OMITTED` 섹션을 반드시 출력한다.

```text
OMITTED reason=token_budget nearby=12 warnings=3 components=5
NEXT_QUERY unity-ctx scene query Stage01.unity --near Table_01 --radius 6
```

에이전트는 `OMITTED`가 있는 상태에서 추측으로 `set --write` 또는 `apply --write`를 진행하지 않는다.

## 5.5 `index`

index는 캐시가 아니라 `file_hash` 기반 검증 가능한 snapshot이다.

```bash
unity-ctx scene index Stage01.unity --out .unity-ctx/index/Stage01.index.json
```

index schema 핵심 필드:

```json
{
  "schema_version": 1,
  "kind": "scene",
  "path": "Assets/Scenes/Stage01.unity",
  "file_hash": "sha256:...",
  "generated_by": "unity-ctx 0.2.0",
  "objects": []
}
```

현재 파일의 `file_hash`와 index의 `file_hash`가 다르면 index를 폐기하고 원본 파일을 다시 파싱한다.

```text
INDEX_STALE file=Assets/Scenes/Stage01.unity reason=file_hash_mismatch reparse=true
```

---

# 6. Inspector 커맨드

## 6.1 설계 철학

`get`, `inspect`, `set`은 `.unity`, `.prefab`, `.asset`에 동일한 인터페이스로 접근한다.

- 타입별 전용 커맨드를 남발하지 않는다.
- 주요 Unity built-in component는 표시 이름 매핑을 제공한다.
- 커스텀 MonoBehaviour는 `inspect`로 실제 serialized field name을 확인한 뒤 접근한다.
- 값 검증은 best-effort type hint로 제공한다.

## 6.2 `inspect`

역할: 컴포넌트 또는 에셋의 모든 직렬화 필드를 compact 형태로 출력한다.

```bash
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset inspect EnemyConfig.asset
unity-ctx scene inspect Stage01.unity --id 12003 --component BoxCollider
```

출력 예시:

```text
NavMeshAgent file=Enemy.prefab
speed 3.5 float key=m_Speed
angularSpeed 120.0 float key=m_AngularSpeed
acceleration 8.0 float key=m_Acceleration
stoppingDistance 0.5 float key=m_StoppingDistance
autoBraking true bool key=m_AutoBraking
```

## 6.3 `get`

역할: 특정 필드 값 조회.

```bash
unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
unity-ctx asset get EnemyConfig.asset --field maxHealth
unity-ctx scene get Stage01.unity --id 12003 --component Rigidbody --field mass
```

출력 예시:

```text
NavMeshAgent.speed = 3.5 type=float key=m_Speed
EnemyConfig.maxHealth = 200 type=int key=maxHealth
Rigidbody.mass = 1.0 type=float key=m_Mass
```

## 6.4 `set`

역할: 특정 필드 값을 수정한다.

### 6.4.1 set 동작 원칙

`set`은 기본적으로 dry-run이다. 실제 파일 수정에는 `--write`가 필요하다.

```bash
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300
# DRY_RUN EnemyConfig.maxHealth 200 -> 300

unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write
# WRITE backup=EnemyConfig.asset.bak changed=1
```

공통 원칙:

- 수정 전 `.bak` 백업을 생성한다.
- 수정 후 `get`으로 결과를 재확인한다.
- 타입 검증은 best-effort이며, 불확실하면 `WARN type_hint`를 출력한다.
- `UNKNOWN`이 있으면 사람 확인 없이 `--write`하지 않는다.

### 6.4.2 프리팹 set impact-first

`.prefab` 대상 `set --write`는 impact-first 정책을 따른다.

```text
IMPACT Enemy.prefab guid=ab12
SCENES 3 Stage01.unity Stage02.unity BossRoom.unity
INSTANCES 12
VARIANTS EnemyElite.prefab EnemyShadow.prefab
Proceed? [y/N]
```

에이전트는 자동으로 `y`를 입력하지 않는다. `--yes`는 사람이 명시적으로 허용한 경우에만 사용한다.

### 6.4.3 Object Targeting Policy

수정 명령은 `fileID`를 기본 식별자로 사용한다.

이름 기반 수정은 기본 거부한다.

```bash
unity-ctx scene set Stage01.unity --name Enemy --component Rigidbody --field mass --value 2.0
# ERROR NAME_TARGET_FOR_MUTATION requires --allow-name-fallback
```

명시적으로 허용한 경우만 진행한다.

```bash
unity-ctx scene set Stage01.unity --name Enemy --allow-name-fallback --component Rigidbody --field mass --value 2.0
# WARN NAME_FALLBACK matched=1
```

여러 개가 매칭되면 중단한다.

```text
ERROR AMBIGUOUS_NAME name=Enemy matches=3
```

## 6.5 표시 이름 매핑

초기 built-in mapping 대상:

- Transform
- Rigidbody
- BoxCollider
- SphereCollider
- CapsuleCollider
- MeshRenderer
- NavMeshAgent
- Animator
- Camera
- Light
- AudioSource

커스텀 MonoBehaviour는 mapping을 자동 추론하지 않는다. `inspect`로 실제 key를 확인한 뒤 serialized field name을 직접 사용한다.

---

# 7. Scene 조작 커맨드

## 7.1 `check`

배치 전 sanity check.

| 검사 | 판정 기준 | 결과 | exit |
|---|---|---|---:|
| footprint 겹침 | XZ footprint 교차 | ERROR | 1 |
| AABB 3D 겹침 | footprint OK 상태에서 3D 교차 | WARN | 0 |
| 피벗 오프셋 | `pivot_offset.y >= 0.05m` | WARN | 0 |
| rotation 위반 | `allowed_rotations` 범위 외 | ERROR | 1 |
| rotation 미정의 | `allowed_rotations == null` | UNKNOWN NEED_RULE | 0 |
| manifest 미등록 | GUID 매칭 실패 | WARN | 0 |

## 7.2 `patch / apply / diff`

### 7.2.1 지원 op 우선순위

| op | 상태 | 설명 |
|---|---|---|
| `place_prefab` | v0.4 지원 | 씬에 프리팹 인스턴스 추가 |
| `move_object` | v0.5 이후 | fileID 기반 위치/회전 변경 |
| `set_active` | v0.5 이후 | fileID 기반 활성/비활성 변경 |
| `remove_object` | v1.0 이후 제한 지원 | leaf GameObject만 허용 |

### 7.2.2 remove_object 제한 정책

`remove_object`는 v1.0 이전 기본 지원하지 않는다. 이후에도 leaf GameObject에 한해 제한적으로 허용한다.

삭제 거부 조건:

- 자식 Transform이 존재한다.
- 다른 컴포넌트가 해당 fileID를 참조한다.
- PrefabInstance 또는 stripped object 관계가 있다.
- scene root 또는 prefab root이다.
- serialized reference 검색 결과 참조가 남아 있다.

거부 출력:

```text
ERROR REMOVE_UNSAFE id=12003 reason=has_children refs=3
```

## 7.3 `apply`

`apply`는 기본 dry-run이다.

```bash
unity-ctx scene apply Stage01.unity patch.json
# DRY_RUN

unity-ctx scene apply Stage01.unity patch.json --write
# WRITE
```

동작 원칙:

- `--write` 전 자동 check 실행.
- ERROR 존재 시 중단.
- 수정 전 `.unity.bak` 생성.
- 적용 후 YAML 파싱 재검증.
- 실패 시 `.bak`에서 자동 복구.
- 성공 시 index 자동 갱신.

## 7.4 `suggest`

`suggest`는 후순위 기능이다.

초기 지원:

- `--near`
- `--count`
- `--align grid`
- `--align floor`

후순위:

- `--align wall`

`--align wall`은 벽 판정 기준이 불명확하므로 v1.0 이후로 미룬다.

---

# 8. 데이터 생성 커맨드

## 8.1 `scan`

bounds manifest 생성.

| 모드 | 설명 | 우선순위 |
|---|---|---|
| Editor export | Unity batchmode로 실제 Renderer/Collider bounds 추출 | 권장 |
| FBX parser fallback | 자체 Go FBX 파서로 vertex 기반 AABB 추출 | 후순위 |

Editor export는 Unity 실제 import scale, prefab 내부 Transform, nested prefab, SkinnedMeshRenderer 등을 반영한다.

FBX parser fallback은 정확도가 낮으므로 CI나 Editor 부재 환경에서만 사용한다.

## 8.2 `infer-rules / validate-rules`

`infer-rules`는 자동 판정이 아니라 규칙 초안 생성만 담당한다.

```bash
unity-ctx scene infer-rules --scene Assets/Scenes --out prop-rules.suggested.json
unity-ctx scene validate-rules prop-rules.json
unity-ctx scene scan --merge-rules prop-rules.json
```

`*.suggested.json`은 사람이 승인하기 전까지 apply/check의 확정 규칙으로 쓰지 않는다.

---

# 9. 기술 명세

## 9.1 Unity YAML parser core

처리 단계:

1. `--- !u!<classID> &<fileID>` 태그 파싱
2. 멀티 도큐먼트 YAML block 분리
3. classID 매핑
4. object graph 구성
5. 컴포넌트 field 접근
6. scalar / bool / int / float / Vector2 / Vector3 / Vector4 / Color 파싱
7. dot notation 접근

주요 classID:

| classID | 의미 |
|---:|---|
| 1 | GameObject |
| 4 | Transform |
| 20 | Camera |
| 23 | MeshRenderer |
| 33 | MeshFilter |
| 54 | Rigidbody |
| 65 | BoxCollider |
| 114 | MonoBehaviour |
| 1001 | PrefabInstance |

## 9.2 field access

접근 방식:

| 방식 | 예시 | 설명 |
|---|---|---|
| serialized key | `m_Speed` | 정확한 YAML key |
| display mapping | `speed` → `m_Speed` | built-in component convenience |
| dot notation | `m_LocalPosition.x` | 중첩 값 접근 |

mapping이 없으면 serialized key를 직접 사용한다.

## 9.3 GUID matching

순서:

1. PrefabInstance source GUID 직접 매칭
2. Prefab 내부 Mesh GUID 추적
3. Renderer/Collider bounds manifest GUID 매칭
4. 이름 기반 fallback

이름 기반 fallback은 항상 WARN이다.

## 9.4 prefab impact

`prefab impact`는 프로젝트 내 `.unity`, `.prefab` 파일을 스캔하여 대상 prefab GUID 참조를 찾는다.

- 기본 탐색 경로: `Assets/`
- `--scenes`로 범위 제한 가능
- nested prefab은 기본 3레벨
- 깊이 초과 시 WARN

출력 예시:

```text
WARN IMPACT_DEPTH_LIMIT prefab=Enemy depth=3 more_possible=true
```

## 9.5 향후 확장: asset impact

`asset impact`는 ScriptableObject, Material, PhysicsMaterial 등 `.asset` 수정 전 영향 범위를 조회한다.

```bash
unity-ctx asset impact EnemyConfig.asset
```

출력 예시:

```text
IMPACT EnemyConfig.asset guid=aa11
PREFABS 4 Enemy.prefab EnemyElite.prefab Boss.prefab
SCENES 2 Stage01.unity references=3 BossRoom.unity references=1
ASSETS 3 EnemySpawnTable.asset DifficultyConfig.asset
```

v1.0 이전에는 read-only impact 조회만 고려한다.

---

# 10. exit code 명세

| exit code | 의미 | 에이전트 행동 |
|---:|---|---|
| 0 | OK / WARN / UNKNOWN만 존재 | 진행 가능. 단 UNKNOWN은 사람 확인 필요 |
| 1 | ERROR 존재 | 중단. 원인 출력 후 재시도 또는 에스컬레이션 |
| 2 | 도구 실행 오류 | 재시도 무의미. 사람 확인 |

주의:

- `UNKNOWN`은 exit 0일 수 있지만, `--write` 허용을 의미하지 않는다.
- SKILL 문서에서 `UNKNOWN` 상태의 `set --write` / `apply --write`를 금지한다.

---

# 11. testplay-runner 연동

## 11.1 역할 분담

| 도구 | 역할 |
|---|---|
| unity-ctx | Unity 데이터 조회, 압축, 정적 검증, 안전한 수정 |
| testplay-runner | 수정 후 Play Mode 기준 런타임 검증 |

`unity-ctx` 성공은 런타임 성공을 의미하지 않는다. 수정 후에는 필요 수준에 따라 `testplay-runner`를 실행한다.

## 11.2 표준 연동 흐름

```text
unity-ctx context-pack / get / inspect
→ unity-ctx set 또는 patch/apply dry-run
→ unity-ctx set --write 또는 apply --write
→ unity-ctx get / query / diff 재검증
→ testplay-runner quick 또는 scene-smoke 실행
```

## 11.3 prefab impact와 테스트 추천

`prefab impact` 결과는 테스트 생략 근거가 아니라 추가 테스트 추천 근거로만 사용한다.

예시:

```text
Enemy.prefab 수정
→ impact: Stage01, Stage02, BossRoom 영향
→ testplay-runner: core smoke + Enemy 관련 테스트 + affected scene smoke 실행
```

금지:

```text
impact 결과에 없다는 이유로 필수 smoke test를 생략하지 않는다.
```

## 11.4 실패 분석 루프

`testplay-runner` 실패 시 에이전트는 전체 YAML을 읽지 않고 `unity-ctx`로 실패 주변 컨텍스트를 조회한다.

```bash
unity-ctx asset get Assets/Configs/EnemyConfig.asset --field maxHealth
unity-ctx prefab inspect Assets/Prefabs/Enemy.prefab --component EnemyController
unity-ctx scene query Assets/Scenes/Stage01.unity --name PlayerSpawn
```

---

# 12. 비기능 요구사항

## 12.1 성능 목표

| 기능 | 목표 |
|---|---:|
| `summarize` | 500ms 이내 |
| `query` | 300ms 이내 |
| `get` | index 유효 시 100ms 이내 |
| `inspect` | index 유효 시 100ms 이내 |
| `context-pack` | 500ms 이내 |
| `set` dry-run | 300ms 이내 |
| `set --write` | backup 포함 500ms 이내 |
| `prefab impact` | 100씬 기준 5초 이내, index 활용 시 1초 이내 |
| `apply` 단일 op | 1초 이내 |

## 12.2 배포

| 항목 | 내용 |
|---|---|
| 배포 형태 | 단일 정적 바이너리 |
| Go 설정 | `CGO_ENABLED=0` |
| 지원 플랫폼 | linux/amd64, linux/arm64, darwin/arm64, windows/amd64 |
| 설치 | `curl | sh`, PowerShell installer |
| Editor export | `PropManifestExporter.cs`를 단일 `.cs` 또는 Unity Package로 제공 |

---

# 13. 개발 로드맵

## v0.1 — Read-only YAML Context MVP

핵심 deliverable:

- Unity YAML block parser
- `scene / prefab / asset` namespace
- `summarize`
- `query` by id/name/type
- `inspect`
- `get`
- `--view tiny|compact|detail`
- `--json`

완료 기준:

- `.unity`, `.prefab`, `.asset` 원문을 직접 읽지 않고 필요한 필드 조회 가능
- custom MonoBehaviour는 serialized field name으로 조회 가능
- 샘플 프로젝트에서 `summarize → query → inspect → get` 동작

## v0.2 — Index & Context Pack

핵심 deliverable:

- `file_hash` 기반 index
- `context-pack`
- `OMITTED` 출력
- token budget enforcement
- `bench`

완료 기준:

- `context-pack`이 token budget을 지킴
- `INDEX_STALE` 감지 후 자동 재파싱
- README benchmark 출력 가능

## v0.3 — Safe Asset Mutation

핵심 deliverable:

- `set` dry-run 기본화
- `asset set --write`
- `.bak` 생성
- type hint
- `get` 재검증 가이드

완료 기준:

- ScriptableObject 필드 수정 E2E
- 잘못된 타입 입력 시 `WARN type_hint` 출력
- `set` 기본 실행은 파일을 수정하지 않음

## v0.4 — Scene Placement Mutation

핵심 deliverable:

- `scan --mode editor`
- bounds manifest
- `check` footprint
- `patch place_prefab`
- `apply` dry-run
- `apply --write`
- `diff`

완료 기준:

- 프롭 하나를 씬에 추가하고 `diff/query`로 확인 가능
- overlap ERROR 시 apply 중단

## v0.5 — Prefab Impact & Prefab Mutation

핵심 deliverable:

- `prefab impact`
- `prefab set` impact-first
- nested prefab depth warning
- `--yes` policy
- `suggest` basic near/grid/floor

완료 기준:

- 프리팹 수정 전 영향 씬/인스턴스/variant 출력
- 에이전트가 자동으로 `--yes`를 붙이지 않는 SKILL 문서 제공

## v1.0 — Agent Harness Release

핵심 deliverable:

- SKILL 문서 일체
- AGENTS.md integration guide
- install scripts
- sample Unity project
- CI examples
- testplay-runner 연동 예시
- FBX parser fallback, 필요 시

완료 기준:

- Codex가 AGENTS.md를 읽고 표준 루프에 따라 작업 가능
- `unity-ctx + testplay-runner` E2E 예시 동작

---

# 14. Acceptance Tests

## 14.1 Read-only tests

### Scenario A: scene summarize

```text
Given Stage01.unity exists
When unity-ctx scene summarize Stage01.unity
Then output contains SCENE, ROOTS, PREFABS
And output token estimate <= 300
```

### Scenario B: prefab inspect/get

```text
Given Enemy.prefab has NavMeshAgent
When unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
Then output contains speed and type float

When unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
Then output contains NavMeshAgent.speed
```

### Scenario C: custom MonoBehaviour field

```text
Given EnemyConfig.asset has field maxHealth
When unity-ctx asset get EnemyConfig.asset --field maxHealth
Then output contains maxHealth and type hint
```

## 14.2 Index tests

```text
Given Stage01.unity indexed
When Stage01.unity changes
And unity-ctx scene query Stage01.unity --id 12003 uses index
Then output contains INDEX_STALE
And tool reparses original file
```

## 14.3 context-pack tests

```text
Given Stage01.unity has Table_01 and nearby props
When unity-ctx scene context-pack Stage01.unity --focus Table_01 --max-tokens 200
Then output contains FOCUS
And if data is omitted, output contains OMITTED
```

## 14.4 set dry-run tests

```text
Given EnemyConfig.asset maxHealth is 200
When unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300
Then file is not modified
And output contains DRY_RUN 200 -> 300

When command is run with --write
Then .bak is created
And maxHealth becomes 300
```

## 14.5 scene apply tests

```text
Given Stage01.unity and Chair prefab manifest
When patch place_prefab is created
And apply is run without --write
Then file is not modified
And output contains DRY_RUN

When apply is run with --write
Then .unity.bak is created
And query can find the new object
```

## 14.6 unsafe removal tests

```text
Given object id=12003 has children
When remove_object patch is applied
Then command exits 1
And output contains ERROR REMOVE_UNSAFE
```

---

# 15. Codex 개발 지침

## 15.1 Codex 작업 방식

Codex는 저장소의 `AGENTS.md`를 읽고 작업한다. 따라서 이 프로젝트는 루트 `AGENTS.md`를 반드시 포함한다.

## 15.2 권장 저장소 구조

```text
unity-ctx/
├─ AGENTS.md
├─ README.md
├─ go.mod
├─ cmd/unity-ctx/main.go
├─ internal/unityyaml/
├─ internal/index/
├─ internal/commands/
├─ internal/output/
├─ internal/mutation/
├─ internal/impact/
├─ internal/bench/
├─ testdata/
│  ├─ scenes/
│  ├─ prefabs/
│  └─ assets/
├─ .agents/
│  └─ skills/
│     ├─ use-unity-ctx/SKILL.md
│     ├─ unity-scene-editing/SKILL.md
│     └─ unity-asset-editing/SKILL.md
└─ docs/
   ├─ SRS.md
   ├─ ROADMAP.md
   └─ COMMANDS.md
```

## 15.3 루트 AGENTS.md 초안

```md
# AGENTS.md

## Project

unity-ctx is a token-aware Unity context provider for AI coding agents.
Its primary goal is to prevent agents from reading or editing raw Unity YAML.

## Priorities

1. Correctness
2. Safety of Unity serialization
3. Token-efficient output
4. Deterministic behavior
5. Minimal dependencies
6. Clear error messages

## Hard Rules

- Do not edit `.unity`, `.prefab`, or `.asset` test fixtures manually unless the task explicitly requires fixture changes.
- Mutation commands must be dry-run-first.
- Do not add heuristic confidence scores.
- Unknown state must be represented as `UNKNOWN`, not guessed.
- Prefer fileID over object name for mutation targets.
- Name fallback must emit WARN or ERROR.
- Keep command output stable; tests may assert exact output.

## Development Commands

```bash
go test ./...
go run ./cmd/unity-ctx --help
```

## Test Data

Use `testdata/` fixtures for parser and command tests.
Do not require Unity Editor for unit tests.

## Roadmap Focus

Start with v0.1 read-only context MVP:

- parser
- summarize
- query
- inspect
- get
- compact output
```

## 15.4 Codex starter prompt

```text
Read AGENTS.md and docs/SRS.md first.
Implement v0.1 only.
Do not implement mutation yet.
Focus on Unity YAML block parsing, summarize, query, inspect, get, and stable compact output.
Add tests using testdata fixtures.
Run go test ./... before final response.
If the SRS is ambiguous, choose the safer behavior and document the assumption.
```

---

# 16. 에이전트 연동 SKILL 요약

## 16.1 use-unity-ctx 핵심 규칙

- Unity 파일 전체를 프롬프트에 붙여넣지 않는다.
- 작업 시작은 `context-pack`, `summarize`, `query`, `inspect`, `get` 중 하나로 한다.
- `--view detail`은 디버깅 전용이다.
- 수정 전에는 항상 dry-run을 먼저 실행한다.
- `--write` 후에는 `get`, `query`, `diff` 중 하나로 재검증한다.
- `UNKNOWN`, `AMBIGUOUS`, `REMOVE_UNSAFE` 상태에서는 임의로 진행하지 않는다.

## 16.2 표준 컴포넌트 값 수정 절차

```text
1. inspect로 현재 필드 확인
2. get으로 현재 값과 타입 확인
3. set dry-run 실행
4. 필요 시 사용자 확인
5. set --write 실행
6. get으로 결과 확인
7. 필요 시 testplay-runner 실행
```

## 16.3 표준 씬 배치 절차

```text
1. scene context-pack
2. scene query, 필요 시
3. scene check
4. scene patch place_prefab
5. scene apply dry-run
6. scene apply