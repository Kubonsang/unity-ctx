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
`scene check` is available as a read-only bounds validation foundation.
`scene patch --op place_prefab` is available as a read-only patch-plan generator.
Without `--prefab-guid` it returns UNKNOWN NEED_PREFAB_GUID instead of guessing a GUID.
With `--prefab-guid` it can return OK or WARN planning results.
`scene apply` and `scene diff` are still deferred.
Run go test ./... before final response.
```
