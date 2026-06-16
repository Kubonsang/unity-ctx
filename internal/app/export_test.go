package app

// Test-only seam for the external app_test package.

// SetReadFinalState overrides the post-write final_check reader and returns the
// previous one, so tests can reach the otherwise-unreachable final_check
// failure branch.
func SetReadFinalState(f func(string) ([]byte, error)) func(string) ([]byte, error) {
	old := readFinalState
	readFinalState = f
	return old
}
