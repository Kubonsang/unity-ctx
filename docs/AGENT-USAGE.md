# Using unity-ctx as an AI Agent

Operating manual for an AI agent that uses the `unity-ctx` CLI to read and modify
Unity files. Optimized for machine branching: every command emits a single
status-prefix first line, and every failure mode has one prescribed recovery.

For exhaustive flag/output detail see [`COMMANDS.md`](COMMANDS.md). This document
is the decision layer on top of it.

## The one rule

**Never read or edit `.unity` / `.prefab` / `.asset` files as raw YAML.** They
blow the token budget and hand-edits silently corrupt Unity serialization. Use
`unity-ctx` for every read and every write. If a command refuses
(`BLOCKED` / `NEED_PREFAB_GUID` / `UNKNOWN`), that is a safety verdict — fix the
cause, do not bypass it by touching the YAML.

## Command map

| Goal | Command | Editor? |
|------|---------|---------|
| Overview of a file | `<ns> summarize <file>` | no |
| Resolve a name → fileID | `scene query <file> --name X` | no |
| Read a component | `<ns> inspect <file> --id N --component C` | no |
| Read one field | `<ns> get <file> --id N --component C --field F` | no |
| Token-budgeted context | `<ns> context-pack <file> --task "..." --max-tokens N` | no |
| What a file references | `<ns> refs <file> [--json]` | no |
| Resolve a prefab GUID | `meta guid <prefab> --project .` | no |
| Blast radius of a prefab | `prefab impact <prefab> --project .` | no |
| Change an asset/prefab field | `<asset\|prefab> set ...` | no |
| Move a scene object | `scene reposition <file> --id N --position x,y,z` | no |
| Place a prefab in a scene | `scene scan` → `suggest` → `diff` → `apply` | scan only |

`<ns>` is `scene`, `prefab`, or `asset`. Only `scene scan` needs a running Unity
Editor; everything else is offline.

## Output prefixes → what to do

Branch on the first token of the first line.

| Prefix | Meaning | Action |
|--------|---------|--------|
| `OK` | success | proceed |
| `FOUND` | query matched | use the `id=` |
| `WARN` | success, review advised | proceed; for writes, inspect the flagged block first |
| `DRY_RUN` | mutation previewed, nothing written | verify `old=`/`new=`, then re-run with `--write` |
| `WRITE` | file written and verified | done; `.bak` backup was created |
| `UNKNOWN` | not enough info (e.g. patch needs a GUID) | supply what is missing; never guess |
| `NEED_PREFAB_GUID` | `.meta` lookup failed | run `meta guid`; if no `.meta`, the asset needs a Unity import |
| `BLOCKED` | write refused by a graph-integrity failure | **do not edit raw YAML**; read the `CHECK`/`ERROR` lines and report |
| `ERROR` | command failed | read the message; fix inputs |
| `OMITTED` | token budget exhausted | raise `--max-tokens` or narrow the query |
| `CHECK` | per-phase safety report detail line | informational, pairs with `BLOCKED`/`WARN` |
| `REF` | one reference evidence line from `refs` | parse as evidence |
| `CANDIDATE` / `PLAN` / `PATCH_OUT` / `SCENES` / `PREFABS` | detail lines | parse per command |

Exit codes: `0` = OK / WARN / UNKNOWN / BLOCKED / NEED_PREFAB_GUID, `1` = ERROR,
`2` = tool execution error. `BLOCKED` and `NEED_PREFAB_GUID` exit 0 because the
tool worked correctly — the result is a refusal, not a crash.

## Write safety contract

`scene apply`, `prefab set`, and `asset set` each run the safety kernel three
times and report the phase statuses on the summary line:

```
pre_check    target file before planning    → ERROR ⇒ BLOCKED (exit 0), file untouched
temp_check   candidate bytes before commit  → ERROR ⇒ BLOCKED (exit 0), file untouched
   --write   atomic write + .bak backup
final_check  re-read file after commit      → ERROR ⇒ ERROR WRITE_COMMITTED (exit 1) + backup= path
```

`WARN` in any phase does not block; it is surfaced via `CHECK` + `WARN` lines.

`final_check` is defense-in-depth and does **not** auto-revert: since `temp_check`
already validated the exact bytes written, a `final_check` failure essentially
only happens under concurrent external modification, where restoring `.bak` would
destroy that edit. The tool surfaces `WRITE_COMMITTED` and leaves recovery to an
explicit `restore`.

## Standard workflows

### Inspect
```bash
unity-ctx scene summarize Stage01.unity
unity-ctx scene query Stage01.unity --name Enemy        # → FOUND id=1234
unity-ctx scene inspect Stage01.unity --id 1234 --component Rigidbody
unity-ctx scene get Stage01.unity --id 1234 --component Rigidbody --field mass
```

### Modify a field (asset / prefab)
```bash
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 200          # DRY_RUN
unity-ctx asset set EnemyConfig.asset --field maxHealth --value 200 --write  # WRITE

unity-ctx prefab impact Enemy.prefab --project .
unity-ctx prefab set Enemy.prefab --project . --id 11400000 --field moveSpeed --value 4.0
unity-ctx prefab set Enemy.prefab --project . --id 11400000 --field moveSpeed --value 4.0 --write --ack-impact
```

### Move a scene object
```bash
unity-ctx scene query Stage01.unity --name Table_01        # → FOUND id=1000 (GameObject)
unity-ctx scene inspect Stage01.unity --id 1000 --component Transform   # → Transform fileID
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4         # DRY_RUN
unity-ctx scene reposition Stage01.unity --id 1001 --position 1.5,2,-3.4 --write # WRITE
```
`--id` is the **Transform** fileID (the block that owns `m_LocalPosition`), not the
GameObject. Resolve it via `inspect ... --component Transform`. Topology is
unchanged, so the same dry-run → `--write` → `.bak` safety contract applies.

### Place a prefab
```bash
unity-ctx scene scan Stage01.unity --mode editor --project . --out /tmp/b.json   # Editor required
unity-ctx scene suggest Stage01.unity --manifest /tmp/b.json --prefab Assets/Prefabs/Chair.prefab \
  --near Table_01 --project . --pick 1 --out /tmp/chair.patch.json               # GUID auto-resolved from .meta
unity-ctx scene diff Stage01.unity --patch /tmp/chair.patch.json
unity-ctx scene apply Stage01.unity --patch /tmp/chair.patch.json --write
```

## Failure recovery

| Situation | Recovery |
|-----------|----------|
| `BLOCKED ... phase=pre_check` | Target file is already broken. Stop. Report the `CHECK`/`ERROR` lines. Never patch raw YAML. |
| `BLOCKED ... phase=temp_check` | This change would corrupt the file. Discard the plan; go back to `inspect`/`query`. |
| `ERROR WRITE_COMMITTED ... phase=final_check backup=<p>` | Write committed then failed verification. Recover with `unity-ctx <ns> restore <file>` (restores `<file>.bak`), then report. |
| `NEED_PREFAB_GUID` | Run `unity-ctx meta guid <prefab> --project .`. If `.meta` is absent, the asset must be imported in Unity. |
| `UNKNOWN` (patch) | Do not `apply` until the GUID is resolved. |
| `OMITTED` | Raise `--max-tokens` or narrow the query. |
| check `WARN` | Read-only work continues. Before writing, `inspect` the flagged fileID. |

## Anti-patterns

- Editing raw YAML to work around `BLOCKED` — spreads corruption, violates the safety policy.
- Filling `NEED_PREFAB_GUID` with a made-up GUID — Unity cannot resolve the reference.
- Skipping `scene diff` before `scene apply --write` — always preview the patch.
- Targeting prefab writes by name — `prefab set` is fileID-only; resolve via `inspect` first.
