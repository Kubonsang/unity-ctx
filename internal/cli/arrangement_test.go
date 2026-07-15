package cli

import (
	"bytes"
	"strings"
	"testing"
)

const arrangementFixture = "../../testdata/arrangements/archive_table.normalized.json"
const arrangementGoldenHash = "8914a0165a43fa8b1c2f21933fdd9723d45dc9b179102031d439ea2c206d8679"

func TestArrangementValidateStableOutput(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runArrangement([]string{"validate", arrangementFixture}, stdout, stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	want := "OK file=" + arrangementFixture + " version=1 preset=InUse members=3 spec_hash=" + arrangementGoldenHash + "\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("stdout=%q want=%q stderr=%q", stdout.String(), want, stderr.String())
	}
}

func TestArrangementValidateStableJSON(t *testing.T) {
	want := `{"file":"` + arrangementFixture + `","members":3,"preset":"InUse","spec_hash":"` + arrangementGoldenHash + `","status":"OK","surface_arrangement_version":1}` + "\n"
	for _, args := range [][]string{
		{"validate", "--json", arrangementFixture},
		{"validate", arrangementFixture, "--json"},
	} {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if code := runArrangement(args, stdout, stderr); code != 0 {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("args=%v stdout=%q want=%q stderr=%q", args, stdout.String(), want, stderr.String())
		}
	}
}

func TestArrangementHashAndTopLevelDispatch(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run([]string{"arrangement", "hash", arrangementFixture}, stdout, stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	want := "OK file=" + arrangementFixture + " spec_hash=" + arrangementGoldenHash + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout=%q want=%q", stdout.String(), want)
	}
}

func TestArrangementRejectsInvalidInvocationAndSpec(t *testing.T) {
	tests := []struct {
		args    []string
		code    int
		message string
	}{
		{[]string{}, 2, "requires validate or hash"},
		{[]string{"save"}, 2, "is not supported"},
		{[]string{"validate"}, 2, "requires exactly one spec file"},
		{[]string{"validate", "--write", arrangementFixture}, 2, "flag provided but not defined"},
		{[]string{"validate", "../../testdata/arrangements/missing.json"}, 1, "ERROR"},
	}
	for _, test := range tests {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if code := runArrangement(test.args, stdout, stderr); code != test.code || !strings.Contains(stderr.String(), test.message) {
			t.Fatalf("args=%v code=%d want=%d stdout=%q stderr=%q", test.args, code, test.code, stdout.String(), stderr.String())
		}
	}
}

func TestArrangementHelp(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run([]string{"arrangement", "hash", "--help"}, stdout, stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != "unity-ctx arrangement hash <file> [--json]\n  print the stable normalized Surface Arrangement v1 spec hash\n" {
		t.Fatalf("unexpected help %q", stdout.String())
	}
}
