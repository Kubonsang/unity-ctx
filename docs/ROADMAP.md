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

## v0.5 — Prefab Impact, Prefab Set, & Basic Suggest

Status:

- `v0.5a` complete: prefab impact foundation
- `v0.5b` complete: prefab set impact-first mutation
- `v0.5c` complete: scene suggest read-only planner (`--manifest`, `--prefab`, `--near`, optional `--count`, `--align floor|grid`, `--json`)
- `v0.5d` complete: suggest-to-patch handoff (`--out`, `--pick`, `--prefab-guid` for `scene suggest`)
- `wall` alignment remains out of scope for this slice
- actual scene placement still flows through `scene patch` and `scene apply`

- prefab impact foundation
- prefab set impact-first
- nested prefab warning reuse across `prefab impact` and `prefab set`
- suggest near/grid/floor

- `v0.2x` complete: token reduction `bench` backfill

## v0.6 — YAML Safety Integration

Status: complete (tag `v0.6.0`)

- unity-fileid-graph safety kernel (`v0.9.0`) integrated as a Go library
- pre/temp/final graph-integrity check on every write path (`scene apply`, `prefab set`, `asset set`)
- `meta guid`, prefab GUID auto-resolve for `patch`/`suggest`
- `refs` (scene/prefab/asset, text + JSON)

## v0.7 — Agent Tooling Expansion

Status: complete (tag `v0.7.0`)

- `validate` — read-only graph integrity check
- `changes` — structural diff vs `<file>.bak`
- `restore` — recover from `<file>.bak`
- `deps` — forward asset dependency export (text / DOT / JSON)
- `mcp` — MCP server over stdio (read-only tools for Claude Code etc.)
- CI/CD (GitHub Actions), Makefile, golangci config
- `samples/MiniDungeon` sample project + benchmarks
- `internal/app` god-object split (types.go / format.go)

Deferred this cycle (low ROI / out of scope): full CLI flag-gating rewrite,
status/prefix const enum, watch mode. See
`docs/BRAINSTORM-improvements-and-features.md`.

## v0.8 — Structural Scene Mutation

Status: complete (tag `v0.8.0`)

- safety kernel bumped to `v0.9.2` (transform parent/child symmetry gap closed
  in `v0.9.1`, transform cycle detection added in `v0.9.2`)
- `scene reposition` — in-place `m_LocalPosition` edit (Transform/RectTransform,
  byte-preserving, topology-invariant)
- v2 `ops[]` patch schema — `scene patch --op reparent|delete` → `diff` →
  `apply`, coexisting with v1 `place_prefab`
- `scene reparent` — atomic 3-block `m_Father`/`m_Children` update; plan-phase
  cycle pre-check (`WOULD_CREATE_CYCLE`); Transform (class 4) endpoints only
- `scene delete` (`--cascade`) — GameObject + component (+ subtree) removal with
  parent `m_Children` / `SceneRoots.m_Roots` unlink; orphan/stripped/in-file
  dangling guards
- `internal/xref` — per-mutation project-wide reverse-reference scanner
  (`Assets/` + `Packages/`, brace-aware inline-PPtr scan, indeterminate
  reporting): reparent surfaces inbound refs as WARN, delete hard-BLOCKS on them
- exit-code contract fix: `BLOCKED` now exits `3` (was `0`) with an
  `EnforceBlockedExit` CLI backstop, so a safety refusal is never mistaken for
  success
- `index` snapshots stamp the real build version in `generated_by`

## v0.9 — Reviewed Spatial Geometry

Status: implemented on the Concept Room Decorator integration branch.

- Spatial Manifest v2 with v1 read compatibility and strict stable JSON output
- compound local OBB proxies, semantic axes, pivot offsets, arbitrary named contact frames, and legacy bottom/back/top aliases
- reviewed planar `SurfacePatch` floor, wall, and ceiling records
- `scene scan --mode editor --geometry detailed` using Unity-imported Collider data first and Renderer bounds as fallback
- rotated compound OBB SAT and holistic simultaneous-contact checks in `scene check`
- `scene suggest --align wall --surface-id ...` read-only deterministic candidates
- `UNKNOWN NEED_GEOMETRY_V2` instead of estimating contact from manifest v1
- read-only MCP tools `unity_spatial_check` and `unity_suggest_wall`
- shared Go/C# geometry verdict fixtures

Unity remains the final authority for preview and Apply. unity-ctx does not parse raw FBX geometry and its MCP server does not expose mutation tools.

## v0.9.1 — Human-reviewed Spatial Contracts

Status: implemented on the Agent Spatial Contract Studio branch.

- strict `AssetSpatialContract` and `InteractionContract` load/save with stable normalized hashes
- Draft, technical, human-review, revision-request, unable-to-judge, approved, and stale states
- `spatial validate`, `spatial review`, `spatial diff`, and dry-run-first `spatial apply`
- approval invalidation through contract, dependency, geometry, interaction, and capture hashes
- approved contract overlay for `scene scan --geometry detailed --contracts ...`
- CLI-only human-authority writes; MCP remains read-only/proposal-only
- lifecycle tests covering technical-error approval refusal and verified apply

## v1.0 — Agent Harness Release

- batch / transaction patch (multi-op atomic apply)
- project-wide index & cross-scene search
- remaining structural mutation (component add/remove, GameObject create) —
  requires block-creation support in the safety kernel first
- installers
- testplay-runner integration

See `docs/PROJECT-ANALYSIS-2026-07.md` for the full feature/direction analysis
behind this roadmap.
