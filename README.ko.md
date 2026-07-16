# unity-ctx

**AI 코딩 에이전트에게 Unity 씬·프리팹·에셋을 토큰 안전하게 읽고 쓰는 인터페이스를 주는 Go CLI — raw YAML 없이, 조용한 손상 없이.**

[English](README.md) | 한국어

---

Unity의 직렬화 포맷은 자동화에 적대적입니다. 씬 파일 하나가 에이전트의 토큰 예산을 넘기고, raw YAML을 손으로 고치면 직렬화가 깨질 위험이 있으며, 안전한 dry-run 경로도 없습니다. `unity-ctx`는 AI 에이전트를 위해 설계된 query-first 명령 표면으로 이 문제를 해결합니다.

## unity-ctx는 누구를 위한 도구인가?

unity-ctx는 에디터 대체재가 아니라 **컨텍스트 레이어**입니다. 두 부류의 사용자가 있습니다.

- **AI 코딩 에이전트** — 압축된 컨텍스트, 모호하지 않은 출력 prefix, dry-run-first 변경이 필요한 자동 호출자. unity-ctx는 이들을 위해 만들어졌습니다.
- **사람 개발자** — 씬 작업은 계속 Unity 에디터로 합니다. unity-ctx는 에디터와 경쟁하지 않고, *자동화* 경로를 안전하게 만듭니다.

에이전트가 Unity 프로젝트를 반복 작업한다면, unity-ctx의 역할은 토큰 예산을 터뜨리거나 씬 파일을 손상시키지 않으면서 모든 읽기·쓰기를 읽기 쉽게 만드는 것입니다.

## 해결하는 문제

| 문제 | 해결 |
|---|---|
| 씬 파일이 토큰 예산 초과 | `summarize`, `context-pack`이 압축된 토큰 한정 출력 |
| raw YAML 직접 편집은 위험 | 모든 변경은 기본 dry-run, 커밋에는 `--write` 필요 |
| 오브젝트 이름은 모호함 | `query`가 이름을 fileID로 해석, 변경은 fileID로 타겟팅 |
| 프리팹 GUID를 모름 | `UNKNOWN`/`NEED_PREFAB_GUID` 반환 — 절대 추측 안 함 |
| 프리팹 변경 영향 범위 불명 | `prefab impact`가 참조하는 모든 씬·프리팹을 변경 전 스캔 |
| 배치 위치가 추측 | `scene suggest`가 bounds manifest로 앵커 주변 후보 순위화 |
| 씬 쓰기 미리보기 불가 | `scene diff`가 `scene apply` 커밋 전 patch 계획 요약 |
| 파일의 토큰 비용 불명 | `bench`가 raw vs summarize vs context-pack 절감률 측정 |
| 쓰기가 직렬화를 깨뜨릴 위험 | 모든 write 경로를 [unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) 안전 커널이 graph 무결성으로 검증 |

## 설치

**소스에서 (Go 1.22+ 필요):**

```bash
go install github.com/Kubonsang/unity-ctx/cmd/unity-ctx@latest
```

또는 로컬 빌드:

```bash
git clone https://github.com/Kubonsang/unity-ctx.git
cd unity-ctx
go build -o unity-ctx ./cmd/unity-ctx
```

외부 런타임 의존성 없음.

> **소스 빌드 (v0.6+):** unity-ctx는 [unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) 안전 커널(버전은 `go.mod`에 고정)에 의존하며, Go 툴체인이 GitHub에서 자동으로 받아옵니다 — 수동 설정 불필요. 산출물은 여전히 런타임 의존성 없는 단일 정적 바이너리입니다.

## 지원 파일 타입

| 확장자 | 네임스페이스 |
|---|---|
| `.unity` | `scene` |
| `.prefab` | `prefab` |
| `.asset`, `.mat` | `asset` |

## 내 Unity 프로젝트에서 사용하기

unity-ctx는 설정 파일이 필요 없습니다. 파일 경로를 명령줄에 직접 전달합니다.

**읽기 명령** — 디스크 어디에 있든 `.unity`/`.prefab`/`.asset` 파일을 가리킵니다:

```bash
unity-ctx scene summarize /Users/me/MyUnityProject/Assets/Scenes/GameLevel.unity
unity-ctx prefab inspect /Users/me/MyUnityProject/Assets/Prefabs/Player.prefab --component Rigidbody
unity-ctx asset get /Users/me/MyUnityProject/Assets/Configs/GameConfig.asset --field startingHealth
```

**`--project`가 필요한 변경 명령** (`prefab impact`, `prefab set`) — `Assets/`, `ProjectSettings/`, `Packages/`를 포함하는 Unity 프로젝트 루트를 전달합니다. 프리팹 경로는 그 루트의 `Assets/` 트리 안에 있어야 합니다:

```bash
unity-ctx prefab impact Assets/Prefabs/Player.prefab \
  --project /Users/me/MyUnityProject

unity-ctx prefab set Assets/Prefabs/Player.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 --field moveSpeed --value 5.0
```

**프리팹 GUID 얻기** — `scene patch`/`scene suggest --out`에 필요합니다. `meta guid`가 프리팹 옆 `.meta` 파일에서 읽어옵니다 (`--prefab-guid`를 생략하면 `patch`/`suggest`도 같은 방식으로 자동 resolve):

```bash
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project /Users/me/MyUnityProject
# OK guid=abc123def456... file=... meta=...
```

## 안전 통합 (v0.6)

모든 write 경로는 [unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) 안전 커널 — 무손실 Unity YAML 블록 파서 + fileID graph 무결성 검사기 — 이 세 지점에서 검증합니다:

```text
pre_check   계획 전 대상 파일        → ERROR면 차단 (BLOCKED, exit 3)
temp_check  커밋 전 후보 바이트       → ERROR면 차단 (BLOCKED, exit 3)
   --write  .bak 백업과 함께 원자적 쓰기
final_check 커밋 후 재독             → ERROR면 WRITE_COMMITTED 보고 (exit 1) + 백업 경로
```

- 이미 구조적으로 깨진 파일(중복 fileID, 누락된 컴포넌트 블록, 역참조 불일치 등)은 절대 변경되지 않습니다.
- `WARN` 발견(unknown class, 미지원 shape 등)은 요약 라인과 `CHECK` 상세 라인으로 표면화되지만 차단하지 않습니다.
- `BLOCKED`는 도구 실패가 아니라 **안전 판정**입니다 — YAML을 직접 편집해 우회하지 마세요.
- `refs`는 커널의 PPtr/GUID 참조 근거를 노출합니다 (`unity-ctx prefab refs Enemy.prefab --json`) — 영향 범위 분석용.

## 명령어

### `unity-ctx scene summarize`

씬의 압축 개요: 오브젝트 수, 컴포넌트 타입, PrefabInstance 목록.

```bash
unity-ctx scene summarize Assets/Scenes/Stage01.unity
unity-ctx prefab summarize Assets/Prefabs/Enemy.prefab
unity-ctx asset summarize Assets/Configs/EnemyConfig.asset
```

---

### `unity-ctx scene query`

이름·fileID·컴포넌트 타입으로 오브젝트를 필터링합니다. 변경을 타겟팅하기 전에 이름을 fileID로 해석할 때 사용합니다.

```bash
unity-ctx scene query Stage01.unity --name Table_01
unity-ctx scene query Stage01.unity --id 1000
unity-ctx scene query Stage01.unity --type NavMeshAgent
```

출력:
```
FOUND id=1000 name=Table_01 type=GameObject
```

---

### `unity-ctx scene inspect`

특정 오브젝트의 컴포넌트 필드.

```bash
unity-ctx scene inspect Stage01.unity --id 1000 --component Rigidbody
unity-ctx prefab inspect Enemy.prefab --component NavMeshAgent
unity-ctx asset inspect EnemyConfig.asset
```

---

### `unity-ctx scene get`

fileID로 단일 필드 값.

```bash
unity-ctx scene get Stage01.unity --id 1000 --component Rigidbody --field mass
unity-ctx prefab get Enemy.prefab --component NavMeshAgent --field speed
unity-ctx asset get EnemyConfig.asset --field maxHealth
```

---

### `unity-ctx scene context-pack`

에이전트 작업을 위한 토큰 한정 컨텍스트 번들을 조립합니다. 예산이 소진되면 `OMITTED` 라인을 냅니다.

```bash
unity-ctx scene context-pack Stage01.unity --task "place a chair near Table_01" --max-tokens 4000
```

---

### `unity-ctx bench`

토큰 절감 측정: raw 파일 vs summarize vs context-pack. `ceil(utf8_bytes / 4)`를 토큰 추정치로 사용 — 외부 토크나이저 불필요.

```bash
unity-ctx scene bench Assets/Scenes/Stage01.unity
unity-ctx scene bench Assets/Scenes/Stage01.unity --task "inspect placement safety"
```

`context-pack`은 `--task`가 있을 때만 측정됩니다.

---

### `unity-ctx scene scan`

Unity 에디터에 질의해 bounds manifest를 생성합니다. **프로젝트를 연 Unity 에디터가 실행 중이어야 합니다.**

```bash
unity-ctx scene scan Stage01.unity \
  --mode editor \
  --project /Users/me/MyUnityProject \
  --out /tmp/Stage01.bounds.json
```

필수: `--mode editor`, `--project`, `--out`

---

### `unity-ctx scene suggest`

앵커 오브젝트 주변 배치 후보를 순위화합니다. 읽기 전용 — 씬을 쓰지 않습니다. `--out`을 주면 `diff`/`apply`에 바로 쓸 수 있는 patch 산출물도 작성합니다.

```bash
unity-ctx scene suggest Stage01.unity \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 \
  --project /Users/me/MyUnityProject \
  --pick 1 \
  --out chair.patch.json
```

필수: `--manifest`, `--prefab`, `--near`
선택: `--count`(기본 4, 최대 4), `--align floor|grid`(기본 `floor`), `--out`, `--pick`(기본 1), `--prefab-guid`, `--project`

`--pick`와 `--prefab-guid`는 `--out`이 필요합니다. `--project`를 주면 `--prefab-guid` 생략 시 `.meta`에서 GUID를 자동 resolve합니다.

---

### `unity-ctx scene patch`

씬에 쓰지 않고 프리팹 배치 patch 계획을 생성합니다.

```bash
unity-ctx scene patch Stage01.unity \
  --op place_prefab \
  --manifest Stage01.bounds.json \
  --prefab Assets/Prefabs/Chair.prefab \
  --position 5,0,0 \
  --project /Users/me/MyUnityProject
```

필수: `--op place_prefab`, `--manifest`, `--prefab`, `--position`
선택: `--prefab-guid`, `--project`, `--json`

`--project`가 있으면 `--prefab-guid` 생략 시 `<prefab>.meta`에서 GUID를 자동 resolve합니다. 그래도 resolve되지 않으면 `UNKNOWN ... NEED_PREFAB_GUID`를 반환하며 절대 추측하지 않습니다. `unity-ctx meta guid`로 명시 조회도 가능합니다.

---

### `unity-ctx scene diff`

저장된 patch 계획을 요약합니다. `apply` 전에 항상 실행하세요.

```bash
unity-ctx scene diff Stage01.unity --patch chair.patch.json
```

---

### `unity-ctx scene apply`

patch 계획을 씬에 적용합니다. 기본 dry-run, `UNKNOWN` patch는 거부됩니다. write 경로는 pre/temp/final graph check를 수행합니다.

```bash
# dry-run (기본)
unity-ctx scene apply Stage01.unity --patch chair.patch.json

# 커밋
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write
```

`--write`는 파일 변경 전 `Stage01.unity.bak`을 만들고, 결과를 재파싱해 추가된 fileID를 검증한 뒤 성공을 보고합니다.

---

### `unity-ctx scene reposition`

기존 씬 오브젝트를 이동합니다: Transform의 `m_LocalPosition`을 in-place로
교체합니다. `--id`는 GameObject가 아니라 **Transform** fileID(class 4 또는
RectTransform 224)입니다. 기본 dry-run.

```bash
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --write
```

출력:
```
DRY_RUN id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 pre_check=OK temp_check=OK
WRITE backup=Stage01.unity.bak id=1001 field=m_LocalPosition old=5,0,3 new=1.5,2,-3.4 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK
```

세 축 숫자만 바뀌고 구분자·주석 등 나머지 바이트는 전부 보존됩니다. 대상이
transform 클래스가 아니거나(`UNSUPPORTED_TARGET_CLASS`) 값이 정확히
`{x, y, z}` 숫자가 아니면(`FIELD_NOT_VECTOR3`) 훼손 대신 거부합니다.

---

### `unity-ctx scene reparent` (v2 patch)

Transform을 새 부모 아래로 이동합니다 — 타깃의 `m_Father`와 양쪽 부모의
`m_Children`을 원자적으로 갱신합니다. v2 `ops[]` patch로 동일한
patch → diff → apply 파이프라인을 사용합니다. `--new-parent 0`은 씬 루트로
이동합니다.

```bash
unity-ctx scene patch Stage01.unity --op reparent --id 4001 --new-parent 4002 --json > reparent.patch.json
unity-ctx scene diff  Stage01.unity --patch reparent.patch.json
unity-ctx scene apply Stage01.unity --patch reparent.patch.json --write --ack-impact
```

사이클을 만들 이동은 write 전에 거부됩니다
(`BLOCKED phase=plan code=WOULD_CREATE_CYCLE`). `--project`를 주면 apply가
교차 파일 inbound 참조를 보고합니다(`WARN REPARENT_HAS_INBOUND_REFS`) —
이동은 fileID를 유효하게 유지하므로 정보 제공용이며 차단하지 않습니다.

---

### `unity-ctx scene delete` (v2 patch)

GameObject와 그 컴포넌트를 씬에서 제거합니다(`--cascade`면 서브트리 전체).
부모의 `m_Children` 또는 씬의 `SceneRoots`에서 unlink합니다. `--id`는
**GameObject** fileID입니다.

```bash
unity-ctx scene patch Stage01.unity --op delete --id 1001 --cascade --json > delete.patch.json
unity-ctx scene diff  Stage01.unity --patch delete.patch.json
unity-ctx scene apply Stage01.unity --patch delete.patch.json --write --ack-impact --project /path/to/project
```

삭제는 참조를 dangling으로 만들 수 있어 reparent보다 강하게 가드됩니다:
자식이 있는 오브젝트는 `--cascade` 필수(`BLOCKED WOULD_ORPHAN_CHILDREN`),
프리팹 인스턴스 내용물은 raw 삭제 금지(`STRIPPED_IN_SUBTREE`), 살아남는
같은 파일 참조는 차단(`IN_FILE_REFERENCED`), 그리고 `--write`는
`--project`가 **필수**라 커밋되는 모든 삭제는 교차 파일 검증을 거칩니다 —
inbound 또는 판정 불가 참조가 있으면 write가 차단됩니다
(`BLOCKED code=CROSS_FILE_REFERENCED`).

---

### `unity-ctx asset set`

`.asset`/`.mat` 파일의 필드 값을 설정합니다. 기본 dry-run.

```bash
# dry-run
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300

# 커밋
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 300 --write
```

`.bak`은 쓰기가 실제로 파일을 바꿀 때만 생성됩니다. `changed=0`이면 파일시스템을 건드리지 않고 성공을 반환합니다.

---

### `unity-ctx prefab impact`

어떤 씬·프리팹이 한 프리팹을 참조하는지 스캔합니다. `prefab set` 전에 실행해 영향 범위를 파악하세요. 중첩 순회는 depth 3에서 제한됩니다.

```bash
unity-ctx prefab impact Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject
```

필수: `--project`
선택: `--scenes`(쉼표 구분), `--json`

---

### `unity-ctx prefab set`

프리팹 필드 값을 설정합니다. dry-run 출력에 impact 요약이 포함됩니다. 쓰기에는 `--ack-impact`가 필요합니다.

```bash
# dry-run (impact 요약 자동 포함)
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 --field moveSpeed --value 4.0

# 커밋
unity-ctx prefab set Assets/Prefabs/Enemy.prefab \
  --project /Users/me/MyUnityProject \
  --id 11400000 --field moveSpeed --value 4.0 \
  --write --ack-impact
```

필수: `--project`, `--id`, `--field`, `--value`. 타겟팅은 fileID 전용이며 `--name`/`--component`는 미지원입니다.

---

### `unity-ctx meta guid`

프리팹/에셋 GUID를 옆의 `.meta` 파일에서 해석합니다. 절대 추측하지 않습니다.

```bash
unity-ctx meta guid Assets/Prefabs/Chair.prefab --project /Users/me/MyUnityProject
```

출력:
```
OK guid=3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b file=Assets/Prefabs/Chair.prefab meta=Assets/Prefabs/Chair.prefab.meta
NEED_PREFAB_GUID file=Assets/Prefabs/Chair.prefab reason=meta_not_found
```

---

### `unity-ctx refs`

안전 커널이 뒷받침하는, 파일의 PPtr/GUID 참조 근거(읽기 전용). raw YAML 없이 파일이 무엇을 가리키는지 추적합니다.

```bash
unity-ctx prefab refs Assets/Prefabs/Enemy.prefab
unity-ctx scene refs Assets/Scenes/Stage01.unity --json
```

`--json`은 `references[]`, `warnings` 개수, `issues[]`(경고 상세)를 담은 `refs` 페이로드를 추가합니다.

## 출력 Prefix

모든 명령은 단일 prefix로 시작하는 첫 줄을 냅니다. 자동 호출자는 나머지를 파싱하지 않고 prefix로 분기할 수 있습니다.

| Prefix | 의미 |
|---|---|
| `OK` | 성공, 경고 없음 |
| `WARN` | 성공이지만 검토 필요 (예: 배치 겹침) |
| `ERROR` | 실패 — 명령 미완료 |
| `UNKNOWN` | 진행에 정보 부족 (예: GUID 없음) |
| `DRY_RUN` | 변경 미리보기, 파일 미작성 |
| `WRITE` | 파일 작성 및 검증 완료 |
| `FOUND` | 쿼리가 최소 하나의 오브젝트와 일치 |
| `OMITTED` | 토큰 예산 소진, 내용 생략됨 |
| `CANDIDATE` | 순위화된 배치 후보 하나 |
| `PLAN` | patch 계획 상세 라인 |
| `PATCH_OUT` | `suggest --out`이 쓴 patch 산출물 |
| `SCENES` / `PREFABS` | impact 분석 결과 라인 |
| `BLOCKED` | graph 무결성 실패로 거부된 쓰기 (`code=GRAPH_CHECK_FAILED`) |
| `CHECK` | 단계별 안전 검사 상세 라인 |
| `REF` | `refs`의 참조 근거 라인 |
| `NEED_PREFAB_GUID` | `.meta`에서 GUID를 해석하지 못함 |

종료 코드: `0` = OK / WARN / UNKNOWN / NEED_PREFAB_GUID, `1` = ERROR, `2` = 도구 실행/사용법 오류, `3` = BLOCKED. `BLOCKED`는 `3`을 반환합니다 — 안전 검사가 쓰기 전에 변형을 거부한 것(파일은 그대로)이며, 에이전트가 거부를 성공으로 오인하지 않도록 성공(`0`)·실패(`1`)와 구분된 코드입니다. `NEED_PREFAB_GUID`는 거부가 아니라 선행 조건 누락이므로 `0`을 유지합니다.

## 권장 에이전트 흐름

```
# 씬 조사
unity-ctx scene summarize Stage01.unity
unity-ctx scene query Stage01.unity --name Table_01   # → fileID 확보
unity-ctx scene inspect Stage01.unity --id 1000 --component Rigidbody

# 프리팹 배치 (--project로 .meta에서 GUID 자동 resolve)
unity-ctx scene scan Stage01.unity --mode editor --project /path/to/project --out stage01.bounds.json
unity-ctx scene suggest Stage01.unity --manifest stage01.bounds.json --prefab Chair.prefab --near 1000 --project /path/to/project --pick 1 --out chair.patch.json
unity-ctx scene diff Stage01.unity --patch chair.patch.json
unity-ctx scene apply Stage01.unity --patch chair.patch.json --write   # pre/temp/final graph check 수행

# 프리팹 필드 수정
unity-ctx prefab impact Enemy.prefab --project /path/to/project
unity-ctx prefab set Enemy.prefab --project /path/to/project --id 11400000 --field moveSpeed --value 4.0
unity-ctx prefab set Enemy.prefab --project /path/to/project --id 11400000 --field moveSpeed --value 4.0 --write --ack-impact
```

## 문서

| 대상 | 문서 |
|------|------|
| 전체 명령 계약 — 플래그·출력·종료 코드 | [`docs/COMMANDS.md`](docs/COMMANDS.md) |
| CLI를 **사용하는** AI 에이전트 운영 매뉴얼 | [`docs/AGENT-USAGE.md`](docs/AGENT-USAGE.md) |
| 코드베이스에 **기여하는** AI 에이전트 | [`AGENTS.md`](AGENTS.md) |
| 테스트 가이드 | [`docs/TESTING.md`](docs/TESTING.md) |
| 로드맵 | [`docs/ROADMAP.md`](docs/ROADMAP.md) |

## 설계 원칙

- **프롬프트에 raw YAML 없음** — 모든 명령은 압축된 구조적 텍스트를 냄
- **dry-run 우선** — `set`/`apply`는 파일 변경에 명시적 `--write` 필요
- **추측보다 UNKNOWN** — 불확실한 상태(`NEED_PREFAB_GUID`, `AMBIGUOUS_NAME`)는 가정하지 않고 보고
- **fileID 타겟팅** — 변경은 fileID로 타겟, 이름 기반은 `WARN`/`ERROR AMBIGUOUS_NAME`
- **안정적 출력** — 모든 명령은 결정적 출력, 테스트가 정확한 문자열을 단언할 수 있음
- **외부 런타임 의존성 없음** — 코어 명령은 Go 1.22+만 필요

## 개발

```bash
# 전체 테스트
go test ./...

# 빌드
go run ./cmd/unity-ctx --help
```

테스트 가이드는 [`docs/TESTING.md`](docs/TESTING.md), 전체 명령 계약은 [`docs/COMMANDS.md`](docs/COMMANDS.md)를 참고하세요.

## 알려진 한계

현재 공백을 정직하게 기록합니다.

**`scene scan`은 Unity 에디터가 필요합니다.** 실행 중인 Unity 에디터 인스턴스가 필요한 유일한 명령입니다. `suggest`, `patch`, `diff`, `apply`, `impact`와 모든 읽기 명령을 포함한 나머지는 에디터 없이 동작합니다.

**`--align wall`은 미구현입니다.** `scene suggest`는 `--align floor`와 `--align grid`를 지원합니다.

**`prefab set`은 `--name`/`--component` 타겟팅을 지원하지 않습니다.** `--id`(fileID)만 받습니다. `prefab inspect`로 먼저 fileID를 찾으세요.

**`scene scan --mode`는 `editor`만 지원합니다.**

**중첩 프리팹 순회는 depth 3에서 제한됩니다.** `prefab impact`/`prefab set`은 제한 도달 시 `WARN IMPACT_DEPTH_LIMIT`를 냅니다. depth 3을 넘는 참조는 보고되지 않을 수 있습니다.

**`scene reposition`은 raw Transform을 편집하며, 프리팹 인스턴스 오버라이드는 다루지 않습니다.** 프리팹 인스턴스인 오브젝트의 실제 위치는 `PrefabInstance.m_Modifications`에 있어 raw Transform 이동은 시각적 효과가 없을 수 있습니다. 일반(비인스턴스) 씬 오브젝트에서는 기대대로 동작합니다.

**구조 변형은 씬 전용이며 patch당 1개 op입니다.** `reposition`/`reparent`/`delete`는 `.unity` 파일만 대상으로 하고, v2 patch는 정확히 하나의 op만 담습니다. reparent 엔드포인트는 순수 `Transform`(class 4)만 허용 — RectTransform과 프리팹 인스턴스(stripped) 엔드포인트는 거부됩니다.

## 상태

현재 **[v0.9.1 — Human-reviewed Spatial Contracts](https://github.com/Kubonsang/unity-ctx/releases/tag/v0.9.1)**: Spatial Manifest v2에 검수된 표면, compound OBB, 접촉 검사, 결정론적 벽 정렬 제안이 추가됐습니다. 재사용 가능한 Asset/Interaction 계약과 Surface Arrangement 사양은 MCP에 승인·변형 권한을 노출하지 않으면서 실제 사용자 승인을 암호학적으로 검증합니다. 기존 manifest v1과 dry-run-first 워크플로도 계속 지원합니다. 전체 로드맵은 [`docs/ROADMAP.md`](docs/ROADMAP.md) 참고.

다음 마일스톤: **v1.0 Agent Harness Release** — 샘플 Unity 프로젝트, CI 예제, 설치 도구.

## 라이선스

Apache 2.0 — [LICENSE](LICENSE) 참고.
