# AGENTS.md

Guidance for an AI agent **contributing to** unity-ctx (writing or modifying its
Go code). If you are an agent **using** the `unity-ctx` CLI to manipulate Unity
files, read [`docs/AGENT-USAGE.md`](docs/AGENT-USAGE.md) instead.

## Project

unity-ctx is a token-aware Unity Context Provider for AI coding agents. It gives
agents a safe, compact interface to read and mutate Unity serialized files
(`.unity`, `.prefab`, `.asset`) instead of editing raw YAML.

Every write path is gated by the
[unity-fileid-graph](https://github.com/Kubonsang/unity-fileid-graph) safety
kernel (version pinned in `go.mod`), which validates Unity fileID-graph integrity before
and after each mutation.

## Architecture

```
cmd/unity-ctx/main.go        entry point → cli.Run
internal/cli/run.go          flag parsing, gating, dispatch to app.Service
internal/app/service.go      command orchestration (Summarize/Query/.../Apply/Set/Refs/MetaGUID)
internal/app/safetyout.go    safety check rendering (CHECK/BLOCKED lines, SafetyPayload)
internal/safety/             adapter over the unity-fileid-graph kernel
internal/mutation/           plan/apply for asset set, prefab set, scene apply (pure byte ops)
internal/{parser,document}/  unity-ctx's own block parser + indexed document
internal/{impact,patch,suggest,bounds,scan,check,index,contextpack,bench}/
internal/core/               Result, View
```

**Dependency boundary (enforced):** only `internal/safety` may import
`github.com/Kubonsang/unity-fileid-graph/...`. `internal/safety/seamguard_test.go`
fails the build if any other package imports the kernel. Keep this seam intact.

## Priorities

1. Correctness
2. Unity serialization safety (graph integrity before/after every write)
3. Stable CLI contracts (tests assert exact output)
4. Token-efficient output
5. Deterministic behavior
6. Minimal dependencies (the safety kernel is the only one)
7. Clear tests

## Hard Rules

- Mutation commands are **dry-run-first**; actual writes require `--write`.
- Every write path runs the safety kernel: `pre_check` → `temp_check` → write →
  `final_check`. A blocking `ERROR` before the write returns `BLOCKED` (exit 3)
  and must not touch the file; `WARN` is surfaced but does not block.
- Use `UNKNOWN` / `NEED_PREFAB_GUID` / `WARN` / `BLOCKED` instead of guessing.
  Never invent a GUID, fileID, or field value.
- Prefer fileID over object name for mutation targets; name fallback must emit
  `WARN` or `ERROR AMBIGUOUS_NAME`.
- Command output must be stable. When you change output, update the golden
  assertions in `internal/app/service_test.go` and `internal/cli/run_test.go` —
  never weaken an assertion to make it pass.
- Test fixtures under `testdata/` must be **structurally valid** Unity YAML
  (GameObjects carry `m_Component`; Transforms carry `m_Father`/`m_Children`;
  script GUIDs are 32-hex). Validate new fixtures with
  `go run github.com/Kubonsang/unity-fileid-graph/cmd/uyaml@latest <ns> check <file>`
  or the local sibling checkout. Broken fixtures will be `BLOCKED` by the checks.
- Keep agent-facing output envelopes uniform: all `WRITE_COMMITTED` lines carry
  the same summary fields; `BLOCKED`/error JSON still includes `patch_plan` and
  `safety` so a JSON consumer sees one shape regardless of verdict.
- Do not add external runtime dependencies beyond the safety kernel.
- Unit tests must not require the Unity Editor (only `scene scan` needs it, and
  it is exercised via a fake runner in tests).

## Development Commands

```bash
go build ./...
go vet ./...
go test ./...            # full suite; use -count=1 to bypass the cache
go run ./cmd/unity-ctx --help
```

## Smoke Test

빌드된 바이너리가 3개 네임스페이스에서 정상 동작하는지 확인:

```bash
go run ./cmd/unity-ctx scene summarize testdata/scenes/simple_scene.unity && \
go run ./cmd/unity-ctx prefab refs testdata/prefabs/enemy.prefab && \
go run ./cmd/unity-ctx asset get testdata/assets/enemy_config.asset --field maxHealth
```

세 줄 모두 `OK`로 시작하면 정상.

## Status

**v0.9.1 — Human-reviewed Spatial Contracts (released).** Spatial Manifest v2,
reviewed surfaces, compound OBB/contact checks, Asset/Interaction contracts,
Surface Arrangement specs, and the OS-local signed human-review ledger are in.
Manifest v1 remains readable, approval writes stay outside MCP, and all existing
mutation paths remain dry-run-first and safety-kernel gated.
See [`docs/ROADMAP.md`](docs/ROADMAP.md) for what is next.

## Reference Docs

| Need | File |
|------|------|
| Full command contract (flags, output, exit codes) | [`docs/COMMANDS.md`](docs/COMMANDS.md) |
| How an agent *uses* the CLI | [`docs/AGENT-USAGE.md`](docs/AGENT-USAGE.md) |
| Testing guide | [`docs/TESTING.md`](docs/TESTING.md) |
| Roadmap | [`docs/ROADMAP.md`](docs/ROADMAP.md) |
