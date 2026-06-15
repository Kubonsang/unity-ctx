# unity-ctx — 개선점 & 추가 기능 브레인스토밍

작성일: 2026-06-15
대상: unity-ctx v0.6.0 (safety kernel: unity-fileid-graph v0.9.0)
상태: 브레인스토밍 (수렴 전 — 아이디어 카탈로그, 우선순위는 잠정)

> 이 문서는 코드베이스 전체를 세 각도(코드 건강성 / 테스트·정합성 / 미래 방향)로 분석한 결과를 한곳에 모은 것이다. "최대한 많은 정보"가 목적이므로 채택 여부와 무관하게 후보를 폭넓게 기록한다. 각 항목에 **영향(Impact)** 과 **난이도(Effort)**, 가능하면 코드 위치를 붙였다.

---

## 0. 현재 상태 스냅샷

### 기능 표면 (17개 명령, 4 네임스페이스)

| 네임스페이스 | 명령 |
|---|---|
| 공통(scene/prefab/asset) | `summarize`, `query`, `inspect`, `get`, `context-pack`, `bench`, `index`, `refs` |
| scene | `scan`, `check`, `patch`, `diff`, `apply`, `suggest` |
| prefab | `impact`, `set` |
| asset | `set` |
| meta | `guid` |

- 모든 write 경로(`scene apply`, `prefab set`, `asset set`)는 safety kernel의 pre/temp/final graph check를 통과해야 함.
- 출력 contract: 첫 줄 단일 prefix(OK/WARN/ERROR/UNKNOWN/BLOCKED/NEED_PREFAB_GUID/FOUND/DRY_RUN/WRITE/CHECK/REF/…), exit 0/1/2.
- 외부 런타임 의존성 1개(safety kernel). 단일 정적 바이너리.

### 코드 규모 (대략)

| 지표 | 값 |
|---|---|
| .go 파일 (비테스트) | ~86 |
| 비테스트 LOC | ~8,000 |
| 최대 파일 | `internal/app/service.go` (~2,700줄) |
| 최대 함수 | `internal/cli/run.go` `Run()` (~768줄) |
| 패키지 | 18 (internal/) |
| 테스트 함수 | ~350 |
| CI / lint 설정 | 0 |

---

## Part A — 코드 건강성 / 리팩터링 (구조 개선)

> 기능은 동작하지만, 코드가 god-object + 거대 함수 + 중복으로 굳어가고 있다. v1.0 전에 정리하지 않으면 새 명령 추가 비용이 계속 커진다.

### A1. `cli/run.go`의 768줄 `Run()` 플래그 게이팅 해체 — **Impact: 높음 / Effort: 중간**
- 현재: `Run()` 안에 `if command == "..."` 가지 ~59개가 플래그 허용/필수를 검사 (run.go:87–542). 새 명령 추가 시 ~10개 블록을 손봐야 하고, suggest --project 같은 버그가 여기서 샜다.
- 제안: 명령별 **flag 스펙 테이블/구조체** 도입 (각 명령이 허용/필수 플래그를 선언, 공용 validator가 검사). 또는 `cobra`/`urfave/cli` 도입 검토(단, "외부 의존성 최소" 원칙과 트레이드오프).
- 부수 효과: 에러 메시지 자동 생성 → 하드코딩 문자열 제거, 게이팅 누락 버그 구조적 차단.

### A2. `service.go` god-object 분할 — **Impact: 높음 / Effort: 중간**
- 현재: 2,700줄에 dispatch + 결과 포매팅 + YAML 의미 + impact + graph-check + patch 계획이 혼재.
- 제안: 도메인별 파일 분리 — `service_read.go`(summarize/query/inspect/get), `service_write.go`(set/apply), `service_place.go`(scan/suggest/patch/diff), `service_refs.go`, `results.go`(포매터). 패키지 분리까지는 불필요, 파일 분리만으로 가독성 크게 개선.

### A3. `setAsset` / `setPrefab` 공통 write-path 추출 — **Impact: 중간 / Effort: 중간**
- 현재: 두 함수가 ~95% 동일 구조(load→pre_check→plan→temp_check→write→final_check). 차이는 prefab의 impact scan + ack gate뿐. (service.go setAsset ~L514, setPrefab ~L681)
- 제안: `runScalarWrite(...)` 공통 골격 + 네임스페이스별 hook(impact 주입, ack gate). final_check/WRITE_COMMITTED 포매팅도 한 곳으로.

### A4. WRITE_COMMITTED / BLOCKED 포매팅 단일화 — **Impact: 중간 / Effort: 낮음**
- 현재: `ERROR WRITE_COMMITTED ...` 포맷 문자열이 5곳 이상 중복(service.go), BLOCKED도 `blockedBody()`와 인라인이 혼재.
- 제안: `safetyout.go`에 `writeCommittedLine(...)`, `blockedLine(...)` 단일 렌더러. (이번 v0.6 리뷰에서 envelope 균일화를 일부 했지만 렌더러 통합까지는 안 됨)

### A5. 출력 prefix/status를 const enum으로 — **Impact: 중간 / Effort: 낮음**
- 현재: "OK"/"WARN"/"BLOCKED"/"FOUND"/… 문자열 리터럴이 코드 전역에 흩어짐. COMMANDS.md의 목록과 코드가 수동 동기화.
- 제안: `core` 패키지에 `Status`/`Prefix` 상수 + 단일 출처. 문서 표를 코드에서 생성하거나 테스트로 일치 검증.

### A6. stringly-typed 에러 제거 — **Impact: 중간 / Effort: 낮음**
- 현재: `strings.Contains(err.Error(), "meta not found")` (service.go ~L961), `strings.HasPrefix(err.Error(), "APPLY_VERIFY_FAILED")` (~L1682) 등 에러 문자열 매칭.
- 제안: 타입화된 sentinel 에러(`errors.Is/As`). 이미 `CommittedWriteError` 인터페이스 패턴이 있으니 확장.

### A7. mutation 패키지 내부 정리 — **Impact: 낮음 / Effort: 낮음**
- `coerceScalar`/`rewriteScalarField`/`formatValue`/`cloneBytes`가 `assetset.go`(487줄)에 있는데 `prefabset.go`(95줄)도 사용. → `mutation/scalar.go`로 추출해 단일 책임 회복.

### A8. finding 렌더링 uyaml 중복 해소 — **Impact: 낮음 / Effort: 중간 (저장소 2개)**
- `internal/safety`의 `errorDetail`/`warningDetail`이 uyaml의 `writeCheckErrorFields`를 포팅한 것 → 커널이 렌더링을 바꾸면 dialect가 조용히 어긋남. (이미 이슈 #11로 기록됨)
- 제안: 렌더링을 unity-fileid-graph `pkg/`로 올려 양쪽이 단일 구현 공유.

---

## Part B — 정합성 / 테스트 / 견고성

> ※ 리서치 중 "meta guid·BLOCKED·refs issues·suggest --project 테스트 없음"이라는 보고가 있었으나, **이들은 v0.6에서 실제로 추가됨**(`TestMetaGUID*`, `TestApplyBlocksWhenPreCheckFails`, `TestSet{Prefab,Asset}BlocksWhenPreCheckFails`, `TestRefsJSONCarriesIssueDetail`, `TestSceneSuggestAcceptsProjectForGUIDAutoResolve`). 아래는 **실제로 비어있는** 항목만 추린 것.

### B1. parser `Fields map[string]any` 순회 비결정성 — **Impact: 높음 / Effort: 낮음**
- `internal/parser/unityyaml.go`의 블록 필드가 Go map → JSON 직렬화/출력 순서가 런 간 달라질 수 있음. agent가 보는 출력과 골든 테스트가 흔들릴 위험.
- 제안: 출력 시 키 정렬 또는 순서 보존 컨테이너. 결정성은 이 프로젝트의 핵심 계약이므로 우선순위 높음.

### B2. float 포매팅 일관성 — **Impact: 중간 / Effort: 낮음**
- `ParseFloat(64)` → `any` → 직렬화 경로에서 `0.5` vs `0.50` vs 지수표기 드리프트 가능. 라운드트립 테스트 부재.
- 제안: 단일 `formatScalar` 경로 강제 + 라운드트립 골든 테스트.

### B3. final_check 실패 경로 테스트 부재 — **Impact: 중간 / Effort: 중간**
- pre/temp는 통과하지만 write 후 final_check만 실패하는 시나리오(=`ERROR WRITE_COMMITTED phase=final_check`)를 재현하는 fixture/테스트가 없음. 현재 BLOCKED 테스트는 전부 pre_check.
- 제안: write가 그 자체로 깨지는 합성 케이스(예: 쓰기 후 외부에서 손상) 또는 mutation 결과가 final에서만 걸리는 fixture.

### B4. 순환/깨진 참조 처리 — **Impact: 중간 / Effort: 중간**
- prefab A→B→A 순환, fileID가 존재하지 않는 블록을 가리키는 dangling ref에 대한 테스트 없음. `impact`의 depth cap(3)이 무한루프를 막지만 순환 자체 탐지/보고는 없음.
- 제안: 순환 탐지 + 명시적 `WARN CIRCULAR_REF`, dangling ref 보고.

### B5. 실제 Unity 파일 견고성 — **Impact: 높음 / Effort: 중간**
- 모든 fixture가 수작업 최소 예제(≤31줄). 실제 Unity가 내보내는 것들이 미검증:
  - 멀티라인 스칼라(`|`, `>`), YAML anchor, null 키워드, 지수표기 float, 4-space 들여쓰기
  - PrefabInstance + m_Modifications (prefab variant)
  - MonoBehaviour 커스텀 필드 다수, 복잡한 셰이더 프로퍼티
  - 대용량 씬(1MB+) — 성능/메모리 미측정
- 제안: 실제 Unity 프로젝트에서 export한 fixture corpus 추가(샘플 프로젝트와 연계 — D3 참고).

### B6. 인코딩/경로 엣지 — **Impact: 낮음 / Effort: 낮음**
- 빈 파일, 헤더만 있는 파일, non-UTF8, Windows 경로, symlink에 대한 명시적 처리/테스트 부족.
- 제안: 빈/헤더만 파일에 대한 명확한 상태 반환, non-UTF8 거부.

### B7. CLI 통합 테스트 공백 — **Impact: 낮음 / Effort: 낮음**
- `--max-tokens`, `--task`/`--focus`(context-pack), `--ack-impact`이 단위 테스트는 있으나 CLI 레벨(runCLI) 통합 테스트는 얇음. (suggest --project 버그가 이 공백에서 샜던 전례)

---

## Part C — 신규 기능 카탈로그 (에이전트 가치 중심)

> 난이도/영향은 잠정. ⚠️는 safety kernel의 구조적 변경(structural mutation)을 요구해 next_plan.md에서 명시적으로 보류된 항목.

### 읽기/분석 계열 (낮은 위험, 높은 ROI)

| # | 기능 | 설명 | Impact | Effort |
|---|------|------|--------|--------|
| C1 | **MCP 서버 래퍼** | `unity-ctx`를 MCP 도구 서버로 노출(`unity_ctx_scene_summarize` 등). Claude Code가 shell-out 없이 네이티브 호출. 기존 CLI 파싱 재사용 → 얇은 래퍼 | 높음 | 낮~중 |
| C2 | **의존성 그래프 export** | `refs` 데이터를 모아 prefab→material→script→texture 체인을 DOT/JSON으로. `unity-ctx project deps --format dot` | 중간 | 낮음 |
| C3 | **프로젝트 전역 인덱스/검색** | `project index Assets/` → 교차 씬 질의("모든 Enemy 인스턴스", "dangling GUID 전부"). 영속 인덱스(SQLite 등) | 높음 | 높음 |
| C4 | **씬 버전 diff** | `scene diff-versions old.unity new.unity` → GameObject/component/PrefabInstance 단위 구조 비교. 롤백/머지 계획에 사용 | 중간 | 중간 |
| C5 | **validate/lint 명령** | `scene validate --rules rules.json` → 네이밍/레이어/태그/bounds 규칙 검사. write 전 사전 점검. (SRS의 infer-rules 스케치 연계) | 중간 | 낮~중 |
| C6 | **refs를 impact/context-pack에 통합** | 현재 `impact`는 prefab 인스턴스 참조만 추적. material/script 의존까지 `impact --deep`로. context-pack이 "참조된 material 없음" 경고 | 중간 | 낮~중 |
| C7 | **stripped object 노출** | 커널이 stripped 블록을 식별하나 `inspect`가 표시 안 함. "⚠️ stripped GameObject — 필드 변경 실패 가능" | 낮음 | 낮음 |

### 쓰기/세션 계열 (중간 위험)

| # | 기능 | 설명 | Impact | Effort |
|---|------|------|--------|--------|
| C8 | **배치/트랜잭션 patch** | 여러 연산(의자 5개 배치 + 기존 3개 회전)을 하나의 원자적 apply로. patch 스키마 확장 | 중간 | 중간 |
| C9 | **세션 dry-run 미리보기** | `session preview session.jsonl` → 다단계 변경을 순서대로 적용한 최종 상태를 write 없이 출력 | 중간 | 중간 |
| C10 | **.bak 넘어선 undo 스택** | 현재 write당 `.bak` 1개뿐. `scene restore --backup <path>` + 다단계 undo | 중간 | 낮음 |
| C11 | **watch/daemon 모드** | `unity-ctx watch Assets/Scenes/` → 변경 감지 시 재분석 emit. 에이전트 루프 가속 | 중간 | 낮~중 |

### 구조적 변경 계열 (⚠️ 커널 선행 필요 — next_plan.md 보류)

| # | 기능 | 설명 | Impact | Effort |
|---|------|------|--------|--------|
| C12 | ⚠️ **component add/remove** | GameObject에 컴포넌트 추가/제거. 커널에 experimental `remove-component`(Rigidbody/BoxCollider allowlist) 존재하나 unity-ctx 미노출. add는 블록 생성 필요 | 매우 높음 | 매우 높음 |
| C13 | ⚠️ **GameObject 생성/삭제** | `place_prefab`의 사촌. 블록 생성 + Transform + parent 링킹 + graph 보수 | 높음 | 매우 높음 |
| C14 | ⚠️ **Transform reparent** | `m_Father` 변경 + parent/child 양방향 정합 보수. 커널에 experimental 플랜 있으나 미머지 | 중간 | 높음 |

> C12–C14는 generic YAML mutation/serializer를 요구하며, next_plan.md §7에서 "성급히 열지 않는다"로 명시. safety kernel이 구조적 mutation을 안전하게 지원한 **뒤에야** unity-ctx가 노출하는 순서가 맞다.

---

## Part D — 생태계 / 포지셔닝 (v1.0 Agent Harness)

> ROADMAP의 다음 마일스톤은 기능이 아니라 **에이전트가 실제로 쓸 수 있는 상태**를 만드는 것. 포트폴리오 임팩트도 여기서 갈린다.

### D1. CI/CD — **Impact: 높음 / Effort: 낮음**
- 현재 `.github/workflows/` 없음 = 회귀 안전망 0. suggest --project 같은 버그가 이미 한 번 샜다.
- 제안: GitHub Actions로 `go build/vet/test` + 양 저장소 매트릭스. golangci-lint 추가. 릴리즈 자동화(태그 → 바이너리 빌드/첨부).

### D2. MCP 통합 — **Impact: 높음 / Effort: 낮~중** (C1과 동일, 생태계 관점)
- Claude Code / MCP 호스트에서 네이티브 도구로. 이 프로젝트의 "AI 에이전트용"이라는 정체성을 가장 직접적으로 실현.

### D3. 샘플 Unity 프로젝트 + E2E 데모 — **Impact: 매우 높음 / Effort: 중간**
- `samples/MiniDungeon/`(의자·테이블·적·스포너). 모든 워크플로우(place/impact/suggest/set/apply)를 하나의 시나리오로.
- README의 토큰 절감 주장을 **실측 수치**로(예: "raw 40K 토큰 → context-pack 300 토큰, 133배"). `bench` 명령이 이미 있으니 숫자만 게시.
- B5의 "실제 Unity fixture" 공백도 이걸로 해소.

### D4. 데모 영상/GIF + 케이스 스터디 — **Impact: 높음 / Effort: 낮음**
- 2–3분 에이전트 루프 워크스루(summarize→query→suggest→apply). 추상 CLI가 아니라 실제 흐름을 보여줌.
- "어떤 손상 모드(중복 fileID, GUID 불일치, graph 깨짐)를 어떻게 막는가" 안전 감사 문서 = 게임 스튜디오 대상 설득력.

### D5. 설치 경험 — **Impact: 중간 / Effort: 낮음**
- `go install`은 되지만 `curl | sh` 인스톨러, Homebrew tap, 릴리즈 바이너리 첨부가 없음.
- 빌드에 버전 주입(`-ldflags`) + `unity-ctx version` 명령.

### D6. 내러티브 강화 — **Impact: 중간 / Effort: 낮음**
- "Unity 레벨 디자인을 AI 에이전트가 자율 반복할 수 있게 됐다. 하지만 40KB raw YAML을 읽히거나 손으로 씬을 고치면 세이브가 깨진다. unity-ctx는 그 안전 계층 — dry-run-first + graph-validated write + 감사 추적. 에이전트 친화 CLI와 안전 커널, 두 제품을 하나로." (next_plan.md §5 기반)

---

## Part E — 우선순위 제안 (잠정)

브레인스토밍이므로 확정 아님. "위험 낮고 임팩트 높은 것부터, 구조적 변경은 커널 선행" 원칙으로 정렬.

### 트랙 1 — 릴리즈 신뢰성 (지금 당장, 작음)
1. **D1 CI/CD** — 회귀 안전망. 다른 모든 작업의 전제.
2. **A5 출력 const enum + 문서 일치 테스트**, **A4 포매터 단일화** — 작고 contract 안정성 직결.
3. **B1 parser 결정성**, **B2 float 포매팅** — 결정성은 핵심 계약, 버그 잠재.

### 트랙 2 — v1.0 Agent Harness (다음 마일스톤)
4. **D3 샘플 프로젝트 + 실측 bench** (+ B5 실제 fixture) — 포트폴리오/실사용 임팩트 최대.
5. **C1/D2 MCP 서버 래퍼** — 정체성 실현, 난이도 낮음.
6. **D4 데모/케이스 스터디**, **D5 설치 경험**.

### 트랙 3 — 코드 건강성 (점진, 새 명령 추가 전)
7. **A1 플래그 게이팅 해체**, **A2 service.go 분할**, **A3 write-path 추출** — 새 기능 추가 비용을 낮춤.
8. **A6 타입 에러**, **A7 mutation 정리**, **A8 finding 렌더링 단일화**(이슈 #11).

### 트랙 4 — 신규 기능 (가치 높은 읽기부터)
9. **C2 의존성 그래프 export**(낮음), **C6 refs↔impact 통합**, **C5 validate/lint**.
10. **C10 undo**, **C11 watch**, **C8 배치 patch**, **C4 씬 버전 diff**, **C3 프로젝트 인덱스**.

### 트랙 5 — 구조적 변경 (커널 선행, post-v1.0)
11. 커널: structural mutation 안전화 → 그 위에 **C12 component / C13 GameObject / C14 reparent**.

---

## Part F — 미해결 질문 (다음 단계에서 결정 필요)

1. **CLI 프레임워크**: 게이팅 리팩터링에 cobra/urfave를 들일 것인가, "외부 의존성 0(커널 제외)" 원칙을 지킬 것인가?
2. **인덱스 저장소**: C3에 SQLite를 쓰면 cgo/순수Go 선택과 "단일 정적 바이너리" 원칙 충돌. 순수 Go 임베디드(예: bbolt) vs 단순 JSON 인덱스?
3. **MCP 범위**: 읽기 전용 도구만 노출할지, write까지 노출하되 dry-run 강제할지?
4. **구조적 mutation 순서**: 커널 `remove-component`를 먼저 unity-ctx에 노출(좁게)할지, generic serializer를 기다릴지?
5. **버전 전략**: 다음은 v0.7(코드 건강성/하니스 일부)인가 바로 v1.0(하니스 완성)인가? 두 저장소 버전 동기화 정책?

---

## 참고

- 코드 위치/규모: `internal/cli/run.go`(Run 768줄), `internal/app/service.go`(~2,700줄), `internal/app/safetyout.go`, `internal/mutation/{assetset,prefabset}.go`, `internal/parser/unityyaml.go`
- 기존 계획 문서: `docs/ROADMAP.md`, `docs/SRS.md`, unity-fileid-graph `docs/unity_ctx_fileid_graph_next_plan.md`(§5 내러티브, §7 보류 목록)
- 열린 이슈: unity-ctx #11(finding 렌더링 중복), #13(배치 블록 Editor-faithful 아님), unity-fileid-graph #20(gofmt 기록)
