package app_test

import (
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/app"
)

// The exit-code contract is public surface: agents branch on these numbers.
// Lock the values so a refactor can't silently renumber them.
func TestExitCodeContractValues(t *testing.T) {
	if app.ExitOK != 0 || app.ExitError != 1 || app.ExitUsage != 2 || app.ExitBlocked != 3 {
		t.Fatalf("exit-code contract drifted: OK=%d ERROR=%d USAGE=%d BLOCKED=%d",
			app.ExitOK, app.ExitError, app.ExitUsage, app.ExitBlocked)
	}
}

// EnforceBlockedExit is the defense-in-depth backstop: a BLOCKED status must map
// to ExitBlocked even if a mutation path returns the wrong code at its source,
// while every other status is passed through untouched.
func TestEnforceBlockedExit(t *testing.T) {
	cases := []struct {
		name   string
		status string
		code   int
		want   int
	}{
		{"blocked source forgot to set the code", "BLOCKED", 0, app.ExitBlocked},
		{"blocked already correct", "BLOCKED", app.ExitBlocked, app.ExitBlocked},
		{"ok stays zero", "OK", 0, 0},
		{"warn stays zero", "WARN", 0, 0},
		{"unknown advisory stays zero", "UNKNOWN", 0, 0},
		{"error passes through", "ERROR", app.ExitError, app.ExitError},
		{"usage passes through", "ERROR", app.ExitUsage, app.ExitUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.EnforceBlockedExit(tc.status, tc.code); got != tc.want {
				t.Fatalf("EnforceBlockedExit(%q, %d) = %d, want %d", tc.status, tc.code, got, tc.want)
			}
		})
	}
}
