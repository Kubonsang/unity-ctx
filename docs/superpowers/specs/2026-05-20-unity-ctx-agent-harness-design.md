# Unity-ctx Agent Harness Design

## Purpose

바이브 코딩 에이전트(주로 Claude Code)가 `.unity` / `.prefab` / `.asset` / `.mat` 파일 작업 시
raw YAML을 직접 읽거나 편집하는 대신 unity-ctx CLI를 올바르게 활용하도록 안내하는 스킬 패키지.

v1.0 "Agent Harness Release" 마일스톤의 핵심 산출물.

## File Structure

```
.claude/
  skills/
    use-unity-ctx/
      SKILL.md                         트리거 + 하드 룰 + 라우팅 테이블
      references/
        commands.md                    커맨드 레퍼런스 (에이전트 관점 재편성)
        workflows.md                   시나리오별 커맨드 시퀀스 + 안티패턴
        subagent-prompts.md            서브에이전트 프롬프트 템플릿
```

## Routing Table

| 필요한 것           | 파일                                                          |
|--------------------|---------------------------------------------------------------|
| 커맨드 전체 레퍼런스 | `.claude/skills/use-unity-ctx/references/commands.md`         |
| 작업별 워크플로우    | `.claude/skills/use-unity-ctx/references/workflows.md`        |
| 서브에이전트 템플릿  | `.claude/skills/use-unity-ctx/references/subagent-prompts.md` |

## Hard Rules

1. `.unity` / `.prefab` / `.asset` / `.mat` 파일 직접 읽기/편집 금지
2. 변경은 dry-run 먼저, 출력 확인 후 `--write`
3. `prefab set --write` 시 반드시 `--ack-impact` 함께 전달
4. GUID 추측 금지 — 모르면 UNKNOWN 상태 patch 유지
5. 오브젝트 특정은 이름 대신 fileID 사용
6. `apply` 전에 반드시 `diff`로 patch 내용 확인
7. `scan`은 Unity Editor 실행 중 상태에서만 동작

## Scenarios Covered

1. **씬/프리팹 조사** — summarize → query → inspect → get
2. **프리팹 배치** — scan → suggest → diff → apply
3. **에셋/프리팹 필드 수정** — (impact →) set dry-run → set --write
4. **프리팹 영향 분석** — prefab impact

## Sub-agent Templates

3가지 시나리오별 서브에이전트 시작 프롬프트 (변수 자리 포함):
- Inspect & Report
- Place Prefab (전체 플로우)
- Modify Field

## Source References

- `docs/COMMANDS.md` — 커맨드 레퍼런스 원본
- `AGENTS.md` — 기존 하드 룰
- `docs/ROADMAP.md` — v1.0 마일스톤 컨텍스트
