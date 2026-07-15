package surfacearrangement

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenHash = "8914a0165a43fa8b1c2f21933fdd9723d45dc9b179102031d439ea2c206d8679"

func TestMarshalMatchesNormalizedGolden(t *testing.T) {
	spec := validSpec()
	spec.ArrangementID = "  archive-reading-table  "
	spec.Amount = 0.5500004
	spec.Members[0], spec.Members[2] = spec.Members[2], spec.Members[0]
	data, err := Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "arrangements", "archive_table.normalized.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("normalized output differs from golden\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestLoadGoldenAndHashAreStable(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "arrangements", "archive_table.normalized.json")
	spec, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if spec.SpecHash != goldenHash || ContentHash(spec) != goldenHash {
		t.Fatalf("hash got spec=%s content=%s want=%s", spec.SpecHash, ContentHash(spec), goldenHash)
	}

	reordered := spec
	reordered.Members = append([]Member(nil), spec.Members...)
	reordered.Members[0], reordered.Members[2] = reordered.Members[2], reordered.Members[0]
	reordered.Orderliness = 0.4500004
	Normalize(&reordered)
	if reordered.SpecHash != goldenHash {
		t.Fatalf("equivalent normalized input changed hash: %s", reordered.SpecHash)
	}
}

func TestDecodeIsStrict(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "arrangements", "archive_table.normalized.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		data    string
		message string
	}{
		{"unknown field", strings.Replace(string(golden), `"arrangement_id":`, `"surprise": true, "arrangement_id":`, 1), "unknown field"},
		{"stale hash", strings.Replace(string(golden), goldenHash, strings.Repeat("0", 64), 1), "spec_hash does not match"},
		{"trailing json", string(golden) + `{}`, "unexpected trailing JSON content"},
		{"missing version", strings.Replace(strings.Replace(string(golden), `"surface_arrangement_version": 1,`, ``, 1), goldenHash, "", 1), "surface_arrangement_version must be 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.data)); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Decode() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestValidateRejectsOutOfRangeValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		message string
	}{
		{"version", func(s *Spec) { s.SurfaceArrangementVersion = 2 }, "surface_arrangement_version"},
		{"resolver version", func(s *Spec) { s.ResolverVersion = 2 }, "resolver_version"},
		{"preset", func(s *Spec) { s.Preset = "Busy" }, "preset must be"},
		{"slider", func(s *Spec) { s.Grouping = 1.01 }, "amount, orderliness"},
		{"non-finite slider", func(s *Spec) { s.Stacking = math.Inf(1) }, "amount, orderliness"},
		{"edge margin", func(s *Spec) { s.EdgeMargin = -0.001 }, "edge_margin"},
		{"stack height", func(s *Spec) { s.MaxStackHeight = 4 }, "max_stack_height"},
		{"member count", func(s *Spec) { s.Members[0].MinimumCount = 4 }, "counts must satisfy"},
		{"total maximum", func(s *Spec) { s.Members[0].MaximumCount = 10 }, "total member counts"},
		{"member weight", func(s *Spec) { s.Members[0].SelectionWeight = -0.1 }, "selection_weight"},
		{"duplicate member", func(s *Spec) { s.Members[1].DescriptorID = s.Members[0].DescriptorID }, "duplicate member"},
		{"missing affinity", func(s *Spec) { s.Members[0].AffinityGroup = "" }, "requires descriptor_id and affinity_group"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.mutate(&spec)
			Normalize(&spec)
			if err := Validate(spec); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestAllPresetsAreAccepted(t *testing.T) {
	for _, preset := range []Preset{PresetNeat, PresetInUse, PresetScattered} {
		spec := validSpec()
		spec.Preset = preset
		Normalize(&spec)
		if err := Validate(spec); err != nil {
			t.Fatalf("preset %s rejected: %v", preset, err)
		}
	}
}

func validSpec() Spec {
	spec := Spec{
		SurfaceArrangementVersion: Version,
		ArrangementID:             "archive-reading-table",
		TargetElementID:           "support-10-reading-table",
		TargetFrameID:             "top",
		Members: []Member{
			{DescriptorID: "book-brown", MinimumCount: 2, MaximumCount: 3, SelectionWeight: 1, AffinityGroup: "books"},
			{DescriptorID: "book-grey", MinimumCount: 1, MaximumCount: 2, SelectionWeight: 0.8, AffinityGroup: "books"},
			{DescriptorID: "candle-lit", MinimumCount: 1, MaximumCount: 1, SelectionWeight: 0.65, AffinityGroup: "light"},
		},
		Preset:          PresetInUse,
		Amount:          0.55,
		Orderliness:     0.45,
		Grouping:        0.75,
		Stacking:        0.55,
		EdgeMargin:      0.08,
		MaxStackHeight:  3,
		SeedOffset:      17,
		ResolverVersion: ResolverVersion,
	}
	Normalize(&spec)
	return spec
}
