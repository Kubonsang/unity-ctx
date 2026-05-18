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
`asset set` is the only mutation slice currently implemented.
`scene scan --mode editor` can generate a deterministic bounds manifest through a Unity Editor edge.
`scene check` is available as a read-only bounds validation foundation.
`scene patch --op place_prefab` is available as a read-only patch-plan generator.
Without `--prefab-guid` it returns UNKNOWN NEED_PREFAB_GUID instead of guessing a GUID.
With `--prefab-guid` it can return OK or WARN planning results.
`scene diff` can summarize persisted patch plans.
`scene apply` can dry-run or `--write` the current append-only place_prefab patch contract.
Run go test ./... before final response.
```

Current `v0.4` surface:

- `scene scan --mode editor`
- `scene check`
- `scene patch --op place_prefab`
- `scene diff`
- `scene apply`
