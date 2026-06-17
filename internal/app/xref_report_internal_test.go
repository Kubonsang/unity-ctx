package app

import (
	"errors"
	"testing"
)

func TestCrossFileSkipReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "assets root error whose path contains 'meta' is not no_meta",
			err:  errors.New("project Assets root not found: /Users/x/gamemeta/Assets"),
			want: "no_assets_root",
		},
		{
			name: "missing meta",
			err:  errors.New("prefab meta not found file=/Users/x/proj/Assets/A.unity"),
			want: "no_meta",
		},
		{
			name: "meta present but no guid line",
			err:  errors.New("prefab guid not found file=/Users/x/proj/Assets/A.unity.meta"),
			want: "no_meta",
		},
		{
			name: "anything else",
			err:  errors.New("open /x: permission denied"),
			want: "scan_error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := crossFileSkipReason(tc.err); got != tc.want {
				t.Fatalf("crossFileSkipReason(%q) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
