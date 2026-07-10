# unity-ctx 프로젝트 분석 — 기능 · 철학 · 발전방향

작성일: 2026-07-10
기준: main `2baab3e` (v0.7.2 + 미태깅 29커밋 = 구조 변형 슬라이스 S1~S5) + 미커밋 exit-code 작업
분석 방법: 전 패키지 코드 분석(2개 병렬 탐색) + 문서 전수(README/SRS/ROADMAP/BRAINSTORM/AGENT-USAGE/COMMANDS) + git 이력/이슈 대조

> 이 문서는 `docs/BRAINSTORM-improvements-and-features.md`(2026-06-15)의 후속이다.
> 그 문서 이후 완료된 것(구조 변형 3종, xref 스캐너, exit-code 계약)을 반영해
> 현재 시점의 기능 전수 · 철학 분석 · 남은 방향을 다시 수렴한다.

---

## 1. 요약 (TL;DR)

- unity-ctx는 **"AI 에이전트용 Unity 직렬화 파일 안전 계층"** 이다. 에디터 대체가 아니라,
  에이전트의 읽기(토큰 압축)와 쓰기(부패 방지)를 모두 중개하는 컨텍스트 프로바이더.
- 기능 표면은 **4 네임스페이스 22개 명령**으로, 읽기(9) · 계획(5) · 변형(4) · 복구/검증(3) · MCP(1)를 덮는다.
  변형은 스칼라 set → 프리팹 배치 → **구조 변형(reposition/reparent/delete)** 까지 도달했다.
- 철학의 핵심은 **"불확실하면 멈춘다"** — dry-run-first, UNKNOWN-over-guessing, BLOCKED(exit 3),
  provably-over-detect 스캐너, 그리고 이를 **테스트로 강제하는 아키텍처**(seamguard, 골든 출력, exit 백스톱).
- 다음 단계는 세 갈래: ① **v0.8 태깅 + 계약/문서 정리**(즉시), ② **구조 변형 라인 완성**
  (rotation, prefab 네임스페이스, GameObject 생성, component add/remove, 배치 트랜잭션),
  ③ **v1.0 에이전트 하니스**(MCP 확장, 프로젝트 인덱스, testplay-runner 연동, 배포 경험).

---

## 2. 현재 상태 스냅샷

| 항목 | 값 |
|---|---|
| 최신 태그 | `v0.7.2` (main은 그 뒤 29커밋 — 구조 변형 슬라이스 전체가 **미태깅**) |
| 미커밋 작업 | BLOCKED exit 0→3 계약 수정 (`internal/app/exit.go` 신규 + service 13개소 + 문서 6종) |
| 안전 커널 | `unity-fileid-graph v0.9.2` (유일한 외부 의존성, `internal/safety`만 import 가능 — seamguard 테스트로 강제) |
| 명령 수 | 22 (scene/prefab/asset/meta + mcp) |
| 패키지 | internal/ 21개, 비테스트 TODO/FIXME **0건** |
| 인프라 | CI(빌드/vet/테스트) + Release(linux-amd64, darwin-amd64/arm64 — **Windows 없음**) |
| 샘플/픽스처 | `samples/MiniDungeon` 데모 프로젝트, `testdata/` 24픽스처(정상/broken/manifest/patch/impact 미니프로젝트) |
| 에이전트 배포물 | `.claude/skills/use-unity-ctx` 스킬, `docs/AGENT-USAGE.md`(에이전트 운영 매뉴얼), MCP 서버 |
| 열린 이슈 | #13(배치 블록 Editor-faithful 아님), #11(finding 렌더링 커널 중복), #2(v0.5d 테스트 공백) |

---

## 3. 기능 전수 분석

### 3.1 명령 표면 (22개)

**읽기 / 컨텍스트 (토큰 절감 축)**

| 명령 | 역할 | 비고 |
|---|---|---|
| `summarize` | 파일 개요(오브젝트/컴포넌트/PrefabInstance 수) | raw 대비 ~88% 토큰 절감 |
| `query` | `--name/--id/--type` → fileID 해소 | AMBIGUOUS_NAME 명시 보고 |
| `inspect` | 오브젝트의 컴포넌트 필드 목록 | stripped 여부는 아직 미표시(→ §6 F-07) |
| `get` | 단일 필드 값 | dot 표기 지원 |
| `context-pack` | 토큰 예산 내 태스크 컨텍스트, 초과 시 `OMITTED` | SRS의 `NEXT_QUERY` 힌트는 **미구현** |
| `bench` | raw vs summarize vs context-pack 실측 | `ceil(bytes/4)` 추정, tokenizer 무의존 |
| `index` | file_hash(sha256) 검증 가능한 블록 스냅샷 | `generated_by`가 `"unity-ctx 0.2.0"` **하드코딩**(version 미연동) |
| `refs` | 커널 기반 PPtr/GUID 참조 증거 | forward 방향 |
| `deps` | GUID → 에셋 경로 해소, DOT 그래프 출력 | forward 의존성 |
| `prefab impact` | 역참조(어느 씬/프리팹이 이 프리팹을 쓰나) | 중첩 깊이 3 하드캡 |

**변형 (안전 축) — 전부 dry-run-first, `--write` 필수, `.bak` 백업**

| 명령 | 역할 | 전용 가드 |
|---|---|---|
| `asset set` / `prefab set` | 스칼라 필드 1개 수정 | type-preserving 강제(TYPE_MISMATCH), prefab은 `--ack-impact` + impact 스캔 필수 |
| `scene reposition` | Transform `m_LocalPosition` in-place 교체 | 대상 클래스 4/224만, 정확히 `{x,y,z}` 숫자만, 구분자/주석/CRLF 보존 |
| `scene patch/diff/apply` (v1 `place_prefab`) | 프리팹 배치 계획→미리보기→적용 | GameObject+Transform 고정 2블록 append, GUID 미해소 시 UNKNOWN 거부 |
| `scene patch/apply` (v2 `reparent`) | 3블록 원자 갱신(m_Father + 양쪽 m_Children) | Transform(4)만, WOULD_CREATE_CYCLE 사전 차단, cross-file은 **가시화만**(WARN) |
| `scene patch/apply` (v2 `delete` `--cascade`) | GameObject+컴포넌트(+서브트리) 제거 + 부모/SceneRoots unlink | WOULD_ORPHAN_CHILDREN, STRIPPED_IN_SUBTREE, IN_FILE_REFERENCED, cross-file은 **하드 BLOCK** |

**계획 / 공간 (배치 파이프라인)**: `scene scan`(에디터 필요, unity-cli 경유 bounds manifest 생성) → `suggest`(앵커 주변 4방향 후보 랭킹, floor/grid 정렬) → `check`(AABB 겹침 검사) → `patch` → `diff` → `apply`.

**복구 / 검증**: `validate`(쓰기와 동일한 graph check를 단독 실행), `changes`(.bak 대비 구조 diff), `restore`(.bak 원자 복구).

**연동**: `meta guid`(.meta에서 GUID 해소, 절대 추측 안 함), `mcp`(stdio MCP 서버 — read-only 7도구: summarize/validate/refs/query/get/deps/impact).

### 3.2 안전 아키텍처 — 6개 층위

1. **Seamguard** — `internal/safety`만 커널을 import 가능. 테스트가 전 저장소를 걸어 위반 시 빌드 실패. 안전 로직의 단일 관문을 **아키텍처로** 강제.
2. **3-phase graph check** — 모든 write: `pre_check`(원본) → plan → `temp_check`(쓸 바이트) → write → `final_check`(재독). pre/temp ERROR = `BLOCKED`(exit 3, 파일 무손상), final ERROR = `ERROR WRITE_COMMITTED`(exit 1, .bak 안내). **no-auto-revert 정책**: temp가 이미 그 바이트를 검증했으므로 final 실패는 사실상 외부 동시 수정 — 자동 복원은 그 수정을 파괴하므로 명시적 `restore`에 맡긴다(문서화된 설계 결정).
3. **파일 종류 게이트** — reposition/reparent/delete는 `.unity`만, prefab set은 `.prefab`만, asset set은 `.asset/.mat`만.
4. **연산별 plan-phase 정책 가드** — 엔드포인트 클래스 제한, 사이클/고아/stripped/in-file dangling 사전 차단(§3.1 표).
5. **교차 파일 역참조 스캐너**(`internal/xref`) — per-mutation 무캐시 스캔(stale 인덱스가 dangling을 가리는 것 자체를 배제). `%YAML` 헤더 스니핑으로 전 텍스트 에셋 커버(Assets+Packages), brace-depth 인식 raw 스캔(`parser.ScanInlinePPtrs`)으로 exotic inline PPtr 전 형태 검출, **GUID 멘션 카운트 백스톱**으로 "파서가 놓친 참조 형태"를 indeterminate로 강등 — 설계상 **증명 가능한 over-detect만**(silent miss 불가). delete는 inbound/indeterminate 모두 차단, reparent는 보고만.
6. **Exit-code 계약 + 백스톱**(진행 중) — `0`=OK/WARN/UNKNOWN, `1`=ERROR, `2`=usage, `3`=BLOCKED. 소스가 잘못 반환해도 CLI 경계의 `EnforceBlockedExit`가 BLOCKED→3을 재강제(이중 방어).

**쓰기 원자성**: temp 파일 + fsync + rename + 디렉터리 fsync. rename 후 dir-sync 실패는 `CommittedWriteError`(바이트는 디스크에 있음)로 구분 — "썼는데 안 썼다고 보고"하는 부류의 거짓말을 타입으로 봉쇄.

### 3.3 코드 품질 관찰

- 비테스트 코드에 TODO/FIXME 0건 — 한계는 주석이 아니라 **명시적 거부 코드**(`UNSUPPORTED_*`, `NEED_RULE`, `PATCH_STALE` 등)로 인코딩됨.
- CLI 통합 테스트(`run_test.go` ~130KB)가 명령/플래그/exit 코드를 전수 골든 단언. 출력 안정성이 계약의 일부.
- 남은 구조 부채(§5.1): `service.go` 2,977줄(분할 후에도 재비대), `Run()` 플래그 게이팅 수백 줄, write 경로 6개가 같은 골격 반복.

---

## 4. 목표와 철학

### 4.1 명시적 목표 (SRS Rev 5)

세 가지 핵심 목적: **① Token-aware context**(raw YAML을 프롬프트에 넣지 않음 — 40K tok 씬을 200~400 tok으로), **② Query-first inspection**(summarize→query→inspect→get 깔때기), **③ Safe mutation**(dry-run→백업→영향 확인→재검증). 측정 기준까지 SRS에 수치로 정의(§2.1: 씬 파악 40,000→200 tok 등)하고 `bench`로 실측 가능하게 했다.

### 4.2 코드에 새겨진 암묵적 철학

문서에 다 적혀 있지 않지만 구현 전반에서 일관되게 관찰되는 원칙들:

1. **불확실성은 1급 시민이다.** `UNKNOWN`/`NEED_PREFAB_GUID`/`indeterminate`/`AMBIGUOUS_NAME`은 오류가 아니라 정상 출력 상태다. 도구는 추측으로 매끄러워지는 대신 모른다고 말하고 멈춘다.
2. **Fail-safe는 증명 가능해야 한다.** xref 스캐너의 "over-detect만 가능" 설계, GUID 멘션 백스톱, exit 백스톱, seamguard — 안전 주장을 사람의 주의력이 아니라 **구조와 테스트**가 보증한다.
3. **바이트 보존 수술.** 전체 재직렬화 대신 대상 토큰만 교체(구분자/들여쓰기/주석/CRLF 보존). Unity 에디터가 다시 저장했을 때 diff가 폭발하지 않고, 지원 못 하는 직렬화 형태는 mangle 대신 거부.
4. **거짓 성공이 최악의 실패다.** BLOCKED를 exit 3으로 분리한 이유, WRITE_COMMITTED에 backup 경로를 싣는 이유, verify가 부재/존재를 재독으로 단언하는 이유 — 모두 "에이전트가 실패를 성공으로 읽는 것"을 막기 위한 설계.
5. **실데이터가 최종 심판.** 수작업 픽스처만 믿다 두 번 데였고(S1: 실제 m_Children은 F3 형태, S5: SceneRoots 누락으로 루트 삭제 전면 오차단), 이후 실측 Unity 프로젝트 전수 검증이 슬라이스 완료 조건으로 자리 잡았다. adversarial 다중 렌즈 리뷰도 개발 사이클의 일부.
6. **에이전트-우선 인터페이스.** 첫 줄 단일 prefix + `key=value` 본문 + 결정적 출력 + exit 코드 = 사람이 아니라 **기계 분기**에 최적화된 UX. 문서도 사람용(README)과 에이전트용(AGENT-USAGE.md 의사결정 표)을 분리.
7. **작은 슬라이스, 사람 게이트.** 기능은 독립 슬라이스(S1~S5)로 쪼개 매번 리뷰·머지 게이트를 거친다. 커널이 안전을 보장 못 하는 구조 변형은 커널 선행 없이 열지 않는다(순서 규율).

### 4.3 포지셔닝 — 3-제품 생태계, 2 사용자

```
unity-fileid-graph (안전 커널)   — YAML 파서 + fileID 그래프 무결성 판정
        ▲ (유일 의존, seamguard로 격리)
unity-ctx (이 프로젝트)          — 에이전트용 컨텍스트/변형 하니스 (정적 검증)
        ▼ (권장 루프)
testplay-runner                  — Play Mode 런타임 검증 (동적 검증)
```

- **AI 에이전트**가 1차 고객: 컴팩트 컨텍스트, 명확한 분기 신호, dry-run-first.
- **인간 개발자**는 Unity Editor를 계속 쓴다 — unity-ctx는 에디터와 경쟁하지 않고 *자동화 경로*만 안전하게 만든다.
- 내러티브: "에이전트가 Unity 레벨을 자율 반복하게 됐지만, raw YAML을 읽히면 토큰이 터지고 손으로 고치면 씬이 깨진다. unity-ctx가 그 사이의 안전 계층이다."

---

## 5. 발전방향

### 5.1 트랙 0 — 즉시: 계약 마무리 + 드리프트 정리 (수일)

1. **exit-code 작업 머지 + `v0.8.0` 태깅.** BLOCKED exit 0→3은 breaking change이므로 구조 변형 슬라이스(29커밋)와 함께 v0.8.0으로 묶어 릴리즈하는 것이 자연스럽다. 릴리즈 노트에 마이그레이션(“exit 0 분기하던 에이전트는 3 추가”) 명시.
2. **문서 드리프트 해소.** `docs/ROADMAP.md`에 v0.8(구조 변형) 항목 자체가 없다 — v1.0 목록에 있던 reparent/delete가 완료됐음을 반영해야 한다. BRAINSTORM의 진행 현황도 마찬가지. README 예시 출력에 reposition/reparent/delete 미반영 여부도 점검.
3. **소소한 정리:** `internal/index`의 `generatedBy="unity-ctx 0.2.0"` 하드코딩을 `version.Version` 연동으로 수정. MEMORY/스킬 문서의 exit 코드 표 최신화(진행 중인 diff에 이미 일부 포함).

### 5.2 트랙 1 — 단기: 구조 부채 상환 + 저비용 완결 (새 기능 추가 전)

- **write-path 공통 골격 추출(구 A3, 지금이 적기).** load→pre→plan→temp→ack→write→verify→final 골격이 이제 **6경로**(asset/prefab set, reposition, apply, reparent, delete)에 복제돼 있다. 다음 구조 변형(op 추가)마다 7번째 복제가 생기기 전에 공통 파이프라인 + op별 hook으로 추출.
- **플래그 게이팅 테이블화(구 A1).** 명령별 허용/필수 플래그를 선언형 스펙으로 — 새 명령 추가 비용을 상수화. (cobra 도입보다는 자체 테이블이 "의존성 최소" 원칙에 부합)
- **stripped object 노출(구 C7).** `parser.Block.IsStripped`가 이미 있으므로 `inspect`/`query` 출력에 표시만 하면 됨 — 에이전트가 "왜 이 오브젝트는 수정이 거부되나"를 사전에 알 수 있다.
- **파서 결정성(구 B1)·float 라운드트립(구 B2)** — `Fields map[string]any` 순회 순서와 float 포매팅 드리프트는 "결정적 출력" 계약의 잠재 균열. 골든 라운드트립 테스트로 봉인.

### 5.3 트랙 2 — 중기: 구조 변형 라인 완성 (v0.9 후보)

S1~S5로 검증된 인프라(v2 ops[], plan-phase 가드, xref 스캐너, 커널 사이클/대칭 검증) 위에서:

- **rotation/scale 변형.** `reposition`의 형제로 `m_LocalRotation`(쿼터니언 — 정규화 검증 필요)과 `m_LocalScale`. 배치 에이전트가 회전 없이 할 수 있는 일은 제한적이다. SRS도 rotation 오류를 대응 대상으로 명시.
- **prefab 네임스페이스로 구조 변형 확장.** reposition/reparent/delete는 현재 `.unity` 전용. `.prefab` 내부 계층에 같은 연산을 여는 것 — 파일 구조가 동일하므로 가드 재검토(프리팹 루트 보호 등)만 하면 인프라 재사용 가능.
- **PrefabInstance `m_Modifications` 편집.** 문서화된 한계 중 실사용 타격이 가장 큰 항목 — 씬에 배치된 프리팹 인스턴스의 위치/필드 오버라이드는 raw Transform이 아니라 m_Modifications에 있다. `scene set-override --instance N --path m_LocalPosition.x --value ...` 형태의 신규 연산.
- **GameObject 생성 + component add/remove(구 C12/C13, 커널 협조).** place_prefab v1의 고정 2블록 한계(이슈 #13 "Editor-faithful 아님")를 v2 ops[]로 흡수하면서: 부모 지정 배치, 빈 GameObject 생성, allowlist 기반 컴포넌트 추가/제거. 커널의 structural-mutation 지원(블록 생성 검증)이 선행 조건 — 커널 로드맵과 동기화 필요.
- **배치/트랜잭션 patch(구 C8).** v2 스키마의 "정확히 1 op" 제약을 다중 op 원자 적용으로 확장(전부 성공 아니면 전부 미적용). "의자 5개 배치 + 3개 회전" 시나리오. plan-phase 가드를 op 시퀀스의 **가상 누적 상태**에 대해 돌려야 하므로 설계 난도 있음.

### 5.4 트랙 3 — v1.0 Agent Harness: "실제로 쓰이는 상태" 만들기

- **MCP 확장.** 현재 read-only 7도구는 표면의 1/3이다. ① 누락된 읽기 도구 추가(context-pack, inspect, bench, changes, suggest, meta guid), ② 변형을 **dry-run 전용**으로 노출(`unity_set_dryrun` 등 — `--write`는 CLI에만 남겨 안전 계약 유지)하는 방안 결정(BRAINSTORM Part F-3 미해결 질문).
- **프로젝트 전역 인덱스(구 C3).** "모든 씬에서 Enemy 인스턴스 찾기", xref/impact 가속. 단, per-mutation 무캐시 원칙과 충돌하지 않도록 **file_hash 검증 가능한 스냅샷**(기존 `index` 명령의 프로젝트 버전)으로 — 읽기/탐색 가속에만 쓰고 안전 판정은 계속 실시간 스캔.
- **testplay-runner 통합 예제.** SRS §12의 권장 루프(정적 unity-ctx → 동적 testplay)를 실제 CI 예제/스킬 문서로. 두 도구를 잇는 것이 "에이전트가 Unity를 다룬다" 스토리의 완성.
- **배포 경험.** release.yml에 **windows-amd64 추가**(Unity 개발자 다수가 Windows — 현재 빌드 매트릭스에 없음), Homebrew tap, `.claude/skills/use-unity-ctx` 스킬의 플러그인 배포, MiniDungeon 기반 실측 bench 수치 README 게재.
- **대용량 성능 검증.** 1MB+ 실씬에서 파서/xref 전수 스캔의 시간·메모리 실측(구 B5 잔여). xref는 프로젝트 규모에 선형이므로 대형 프로젝트 수치가 나와야 인덱스(위) 우선순위를 판단할 수 있다.

### 5.5 트랙 4 — 장기/탐색 (post-v1.0)

- **씬 버전 diff(구 C4)** — `scene diff-versions old.unity new.unity`, git 워크플로/리뷰 통합.
- **다단계 undo(구 C10)** — 현재 .bak 1단계. 타임스탬프 백업 스택 + `restore --backup <path>`.
- **rules/lint(구 C5)** — 네이밍/레이어/배치 규칙 검사(`NEED_RULE` 인프라 이미 존재).
- **scan --mode standalone** — 에디터 없이 bounds 근사(mesh 파싱 없이 Transform+기본 크기 휴리스틱). 정확도 한계를 WARN으로 명시하는 조건부.
- **watch 모드(구 C11)** — 이전 판단(행동 주도 에이전트에 부적합)은 유효하나, MCP 상주 서버가 생기면 file-hash 무효화 알림 형태로 재평가 여지.

---

## 6. 필요 기능 카탈로그 (우선순위 정렬)

| # | 기능 | 트랙 | Impact | Effort | 선행 조건 |
|---|---|---|---|---|---|
| F-01 | exit-code 머지 + v0.8.0 태깅/릴리즈 노트 | 0 | 높음 | 낮음 | — |
| F-02 | ROADMAP/BRAINSTORM/README 드리프트 해소 | 0 | 중간 | 낮음 | F-01 |
| F-03 | `index` generated_by 버전 연동 | 0 | 낮음 | 낮음 | — |
| F-04 | write-path 공통 골격 추출 (6경로) | 1 | 높음 | 중간 | — |
| F-05 | 플래그 게이팅 선언형 테이블 | 1 | 높음 | 중간 | — |
| F-06 | 파서 결정성 + float 라운드트립 봉인 | 1 | 중간 | 낮음 | — |
| F-07 | stripped object를 inspect/query에 표시 | 1 | 중간 | 낮음 | — |
| F-08 | rotation/scale 변형 (`m_LocalRotation`/`m_LocalScale`) | 2 | 높음 | 중간 | F-04 권장 |
| F-09 | PrefabInstance m_Modifications 오버라이드 편집 | 2 | 매우 높음 | 높음 | — |
| F-10 | prefab 네임스페이스 구조 변형(reparent/delete/reposition) | 2 | 높음 | 중간 | 가드 재설계 |
| F-11 | GameObject 생성 / component add·remove (v2 ops[]) | 2 | 매우 높음 | 매우 높음 | 커널 structural mutation |
| F-12 | 배치/트랜잭션 다중 op 원자 적용 | 2 | 중간 | 높음 | F-04 |
| F-13 | place_prefab Editor-faithful 블록 (이슈 #13) | 2 | 중간 | 중간 | F-11과 연계 |
| F-14 | MCP 읽기 도구 확장 + dry-run 변형 노출 결정 | 3 | 높음 | 낮~중 | 범위 결정(§7-1) |
| F-15 | Windows 릴리즈 빌드 + CI 매트릭스 | 3 | 높음 | 낮음 | — |
| F-16 | 프로젝트 전역 인덱스(file_hash 검증형) | 3 | 높음 | 높음 | 성능 실측(F-18) |
| F-17 | testplay-runner 연동 예제/CI 레시피 | 3 | 높음 | 중간 | — |
| F-18 | 1MB+ 실씬 성능/메모리 벤치 | 3 | 중간 | 낮음 | — |
| F-19 | 실측 bench 수치 README 게재 + 데모 GIF | 3 | 중간 | 낮음 | — |
| F-20 | context-pack `NEXT_QUERY` 힌트 (SRS 스케치 구현) | 3 | 중간 | 낮음 | — |
| F-21 | 씬 버전 diff | 4 | 중간 | 중간 | — |
| F-22 | 다단계 undo 스택 | 4 | 중간 | 낮음 | — |
| F-23 | rules/lint 검사 | 4 | 중간 | 중간 | — |
| F-24 | scan standalone 모드(근사 bounds) | 4 | 낮~중 | 높음 | — |
| F-25 | finding 렌더링 커널 단일화 (이슈 #11) | 4 | 낮음 | 중간 | 커널 협조 |

---

## 7. 미해결 질문 (결정 필요)

1. **MCP 변형 노출 범위** — read-only 유지 vs dry-run 전용 변형 노출 vs `--write`까지(비권장). 안전 계약의 경계를 어디에 둘 것인가.
2. **커널 structural mutation 로드맵** — F-11(생성/컴포넌트)은 커널이 "블록 생성"을 검증할 수 있어야 한다. unity-fileid-graph 쪽 다음 슬라이스 정의가 선행.
3. **인덱스 저장 형식** — 프로젝트 인덱스(F-16)에 순수 Go 임베디드(bbolt) vs 단순 JSON. "단일 정적 바이너리" 원칙상 cgo SQLite는 배제.
4. **버전 전략** — v0.8.0(구조 변형+exit) → v0.9(구조 변형 라인 완성) → v1.0(하니스)로 갈지, v0.8 이후 바로 v1.0 하니스로 갈지. 커널과의 버전 동기화 정책 포함.
5. **prefab 구조 변형의 가드 수위** — 프리팹 루트/variant 관계 보호를 어디까지 차단으로 강제할 것인가.

---

## 참고

- 명령 계약 전문: `docs/COMMANDS.md` / 에이전트 운영: `docs/AGENT-USAGE.md` / 기여 규칙: `AGENTS.md`
- 이전 브레인스토밍(아이디어 카탈로그): `docs/BRAINSTORM-improvements-and-features.md`
- 구조 변형 슬라이스 이력(S1~S5): PR #34~#39, 커널 v0.9.1/v0.9.2
- 열린 이슈: #13, #11, #2
