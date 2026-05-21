# AGENTS.md

## Project

unity-ctx is a token-aware Unity Context Provider for AI coding agents.

The goal is to prevent agents from reading or editing raw Unity YAML by providing:

- compact context
- query-first inspection
- dry-run-first mutation
- deterministic command output

## Priorities

1. Correctness
2. Unity serialization safety
3. Stable CLI contracts
4. Token-efficient output
5. Deterministic behavior
6. Minimal dependencies
7. Clear tests

## Hard Rules

- Do not edit `.unity`, `.prefab`, or `.asset` fixtures manually unless the task explicitly asks for fixture changes.
- Do not implement mutation before the read-only MVP is stable.
- Mutation commands must be dry-run-first.
- Do not add heuristic confidence scores.
- Use `UNKNOWN`, `NEED_RULE`, or `WARN` instead of guessing.
- Prefer fileID over object name for mutation targets.
- Name fallback must emit WARN or ERROR.
- Command output must be stable; tests may assert exact output.
- Do not introduce external runtime dependencies for core commands.
- Unit tests must not require Unity Editor.

## Development Commands

```bash
go test ./...
go run ./cmd/unity-ctx --help
```

## Smoke Test

빌드된 바이너리가 3개 네임스페이스 모두 정상 동작하는지 확인:

```bash
./unity-ctx scene summarize testdata/scenes/simple_scene.unity && \
./unity-ctx prefab summarize testdata/prefabs/enemy.prefab && \
./unity-ctx asset get testdata/assets/enemy_config.asset --field maxHealth
```

3줄 모두 `OK`로 시작하면 정상.

## Current Focus

Start with v0.1 read-only context MVP:

- Unity YAML block parser
- scene / prefab / asset namespace
- summarize
- query
- inspect
- get
- tiny / compact / detail output
