# unity-ctx Codex Development Kit

`unity-ctx` is a token-aware Unity Context Provider for AI coding agents.

This kit contains:

- `docs/SRS.md`: Codex-ready SRS Rev 5
- `AGENTS.md`: root project rules for Codex
- `.agents/skills/*/SKILL.md`: focused development skills
- `.agents/prompts/*.md`: copy-paste task prompts
- `.codex/agents/*.toml`: optional Codex subagent specs
- `docs/COMMANDS.md`, `docs/ROADMAP.md`, `docs/TESTING.md`: implementation references

Recommended first prompt:

```text
Read AGENTS.md and docs/SRS.md first.
Start from the read-only commands first.
Implemented mutation slices are `asset set` and `prefab set`.

`scene scan --mode editor` generates a deterministic bounds manifest through a Unity Editor edge.
`scene check` is a read-only bounds validation tool.
`scene patch --op place_prefab` is a read-only patch-plan generator.
`scene diff` summarizes persisted patch plans.
`scene apply` dry-runs or `--write`s the append-only place_prefab patch contract.

`scene suggest` is a read-only placement planner for ranked near/grid/floor candidates.
  - `--manifest`, `--prefab`, `--near`, optional `--count`, `--align floor|grid`, `--json`
  - `--align wall` is out of scope
  - `--out <file>` writes a diff/apply-compatible patch artifact for the selected rank
  - `--pick <n>` (default 1) selects the candidate rank; requires `--out`
  - `--prefab-guid <guid>` embeds the GUID; requires `--out`; omit → `status=UNKNOWN` (cannot apply until GUID is known)
  - `PATCH_OUT status` may differ from `candidate_status`: patch checks anchor overlap, suggest does not
  - Placement always flows through `scene apply --write`

`prefab impact --project <project>` is a read-only impact scan for scene and nested prefab references.
  - compact output, optional `--scenes`, `--json` with nested `impact` payload
  - nested traversal beyond depth cap returns `WARN IMPACT_DEPTH_LIMIT`

`prefab set <prefab> --project <project> --id <fileID> --field <field> --value <value>` is an impact-first mutation slice.
  - fileID-only, defaults to dry-run, `ack_required` in dry-run output
  - writes require `--write --ack-impact`
  - `--json` uses the same nested `impact` payload shape as `prefab impact`; `asset set` JSON is unchanged

`bench` measures token reduction: raw-vs-summarize always, raw-vs-context-pack with `--task`.

Run go test ./... before final response.
```

Current `v0.5d` surface:

- `scene scan --mode editor`
- `scene check`
- `scene suggest` (`--out`, `--pick`, `--prefab-guid`)
- `scene patch --op place_prefab`
- `scene diff`
- `scene apply`
- `prefab impact --project <project>`
- `prefab set <prefab> --project <project> --id <fileID> --field <field> --value <value>`
- `asset set`
- `bench`
