# Roadmap

## v0.1 — Read-only YAML Context MVP

- Unity YAML block parser
- scene / prefab / asset namespace
- summarize
- query by id/name/type
- inspect
- get
- --view tiny|compact|detail
- --json if feasible

Done when:

- `.unity`, `.prefab`, `.asset` fixtures can be parsed
- summarize/query/inspect/get work on testdata
- command output is deterministic
- `go test ./...` passes

## v0.2 — Index & Context Pack

- file_hash index
- context-pack
- OMITTED output
- token budget enforcement
- bench command

## v0.3 — Safe Asset Mutation

- set defaults to dry-run
- asset set --write
- .bak backup
- type hints
- get-based verification

## v0.4 — Scene Placement Mutation (complete)

- scan --mode editor
- bounds manifest
- check footprint
- patch place_prefab
- apply dry-run / --write
- diff

## v0.5 — Prefab Impact & Basic Suggest

- prefab impact foundation
- prefab set impact-first
- nested prefab warning
- suggest near/grid/floor

## v1.0 — Agent Harness Release

- SKILL docs
- AGENTS.md integration guide
- installers
- sample Unity project
- CI examples
- testplay-runner integration
