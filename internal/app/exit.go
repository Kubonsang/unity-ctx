package app

// Process exit codes for the unity-ctx CLI. This is the single source of truth
// for the exit-code contract that agents and scripts rely on to tell outcomes
// apart:
//
//	0  OK / advisory (also WARN, UNKNOWN, NEED_PREFAB_GUID, DRY_RUN) — the request
//	   was answered; for a mutation the write either happened or was a no-op/dry-run.
//	1  ERROR — the command failed. Includes post-write graph corruption
//	   (final_check), where the file was already modified and may need restore.
//	2  usage / invocation error — bad flags, arguments, or output encoding.
//	3  BLOCKED — a safety check refused the mutation BEFORE any write; the file is
//	   untouched. Kept distinct from ERROR so an agent can never mistake a refusal
//	   for success (the original "BLOCKED -> exit 0" contract bug).
const (
	ExitOK      = 0
	ExitError   = 1
	ExitUsage   = 2
	ExitBlocked = 3
)

// statusBlocked is the result Status that a safety check sets when it refuses a
// mutation before any bytes are written.
const statusBlocked = "BLOCKED"

// EnforceBlockedExit guarantees that a BLOCKED result maps to ExitBlocked at the
// CLI boundary. It is a defense-in-depth backstop: even if a mutation path forgets
// to return ExitBlocked at its source, a BLOCKED status can never leak exit 0
// (which an agent would read as success). Every other status passes through
// unchanged so existing OK/WARN/ERROR/usage codes are untouched.
func EnforceBlockedExit(status string, code int) int {
	if status == statusBlocked {
		return ExitBlocked
	}
	return code
}
