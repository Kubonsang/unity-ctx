package surfacearrangement

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
	// The checked-out fixture may use CRLF on Windows; Marshal deliberately
	// emits stable LF JSON on every platform.
	normalizedWant := strings.ReplaceAll(string(want), "\r\n", "\n")
	if string(data) != normalizedWant {
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

func TestContentHashAndMarshalDoNotMutateMembers(t *testing.T) {
	spec := validSpec()
	spec.ArrangementID = "  archive-reading-table  "
	spec.Members = append([]Member(nil), spec.Members...)
	spec.Members[0], spec.Members[2] = spec.Members[2], spec.Members[0]
	spec.Members[0].DescriptorID = "  candle-lit  "
	want := spec
	want.Members = append([]Member(nil), spec.Members...)

	_ = ContentHash(spec)
	if !reflect.DeepEqual(spec, want) {
		t.Fatalf("ContentHash mutated caller\ngot:  %#v\nwant: %#v", spec, want)
	}
	if _, err := Marshal(spec); err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !reflect.DeepEqual(spec, want) {
		t.Fatalf("Marshal mutated caller\ngot:  %#v\nwant: %#v", spec, want)
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
		{"missing hash", strings.Replace(string(golden), goldenHash, "", 1), "spec_hash is required"},
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

func TestDecodeForHashAllowsMissingHashWhileDecodeRejectsIt(t *testing.T) {
	spec := validSpec()
	spec.SpecHash = ""
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "spec_hash is required") {
		t.Fatalf("Decode() error = %v, want required hash", err)
	}
	got, err := DecodeForHash(data)
	if err != nil {
		t.Fatalf("DecodeForHash() error = %v", err)
	}
	if got.SpecHash == "" || got.SpecHash != ContentHash(got) {
		t.Fatalf("DecodeForHash() did not calculate a valid hash: %#v", got)
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
		{"negative seed offset", func(s *Spec) { s.SeedOffset = -1 }, "seed_offset must be non-negative"},
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

func TestDecodeAndValidateRejectRawValuesBeforeRounding(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		message string
	}{
		{"slider just above one", func(s *Spec) { s.Amount = 1.0000004 }, "amount, orderliness"},
		{"negative edge margin near zero", func(s *Spec) { s.EdgeMargin = -0.0000004 }, "edge_margin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.mutate(&spec)
			spec.SpecHash = ""
			if err := Validate(spec); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate(raw) error = %v, want containing %q", err, test.message)
			}
			data, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Decode(raw) error = %v, want containing %q", err, test.message)
			}
			if _, err := DecodeForHash(data); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("DecodeForHash(raw) error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestCanonicalJSONMatchesUnityUnicodeAndNumberSemantics(t *testing.T) {
	longID := "arrangement-" + strings.Repeat("x", 140) + "-한글<&>"
	spec := Spec{
		SurfaceArrangementVersion: Version,
		ArrangementID:             longID,
		TargetElementID:           "table<&>",
		TargetFrameID:             "top-면",
		Members: []Member{
			// Go rune ordering puts U+E000 before U+10000. Unity's UTF-16
			// ordinal ordering must put the surrogate pair first instead.
			{DescriptorID: "book-\uE000", MinimumCount: 1, MaximumCount: 1, SelectionWeight: 1, AffinityGroup: "책<&>"},
			{DescriptorID: "book-\U00010000", MinimumCount: 1, MaximumCount: 1, SelectionWeight: 0.5500005, AffinityGroup: "책<&>"},
		},
		Preset:          PresetInUse,
		Amount:          0.5500005,
		Orderliness:     0.45,
		Grouping:        0.75,
		Stacking:        0.55,
		EdgeMargin:      101.25,
		MaxStackHeight:  3,
		SeedOffset:      17,
		ResolverVersion: ResolverVersion,
	}
	Normalize(&spec)
	if err := Validate(spec); err != nil {
		t.Fatalf("Unicode/long-ID/>100m spec should match Unity validation: %v", err)
	}
	if got := spec.Members[0].DescriptorID; got != "book-\U00010000" {
		t.Fatalf("UTF-16 ordinal first member = %q, want supplementary code point", got)
	}
	want := fmt.Sprintf(
		`{"surface_arrangement_version":1,"arrangement_id":"%s","target_element_id":"table\u003c\u0026\u003e","target_frame_id":"top-면","members":[{"descriptor_id":"book-𐀀","minimum_count":1,"maximum_count":1,"selection_weight":0.55,"affinity_group":"책\u003c\u0026\u003e"},{"descriptor_id":"book-","minimum_count":1,"maximum_count":1,"selection_weight":1,"affinity_group":"책\u003c\u0026\u003e"}],"preset":"InUse","amount":0.55,"orderliness":0.45,"grouping":0.75,"stacking":0.55,"edge_margin":101.25,"max_stack_height":3,"seed_offset":17,"resolver_version":1,"spec_hash":""}`,
		"arrangement-"+strings.Repeat("x", 140)+`-한글\u003c\u0026\u003e`,
	)
	got := string(canonicalJSON(spec))
	if got != want {
		t.Fatalf("canonical JSON differs from Unity\ngot:  %s\nwant: %s", got, want)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(want)))
	if spec.SpecHash != wantHash || ContentHash(spec) != wantHash {
		t.Fatalf("canonical hash got spec=%s content=%s want=%s", spec.SpecHash, ContentHash(spec), wantHash)
	}
}

func TestSharedCanonicalParityFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "arrangements", "canonical_parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion     int      `json:"schema_version"`
		UTF16OrdinalOrder []string `json:"utf16_ordinal_order"`
		HTMLEscape        struct {
			Input     string `json:"input"`
			Canonical string `json:"canonical"`
		} `json:"html_escape"`
		MinimumLongIDLength       int     `json:"minimum_long_id_length"`
		EdgeMarginOverLegacyLimit float64 `json:"edge_margin_over_legacy_limit"`
		CanonicalJSON             string  `json:"canonical_json"`
		SpecHash                  string  `json:"spec_hash"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != Version || len(fixture.UTF16OrdinalOrder) != 2 ||
		!utf16OrdinalLess(fixture.UTF16OrdinalOrder[0], fixture.UTF16OrdinalOrder[1]) {
		t.Fatalf("invalid UTF-16 parity fixture: %#v", fixture)
	}
	var quoted strings.Builder
	appendJSONString(&quoted, fixture.HTMLEscape.Input)
	if got, want := quoted.String(), `"`+fixture.HTMLEscape.Canonical+`"`; got != want {
		t.Fatalf("HTML-safe quote = %q, want %q", got, want)
	}
	spec := validSpec()
	spec.ArrangementID = strings.Repeat("x", fixture.MinimumLongIDLength)
	spec.EdgeMargin = fixture.EdgeMarginOverLegacyLimit
	Normalize(&spec)
	if err := Validate(spec); err != nil {
		t.Fatalf("shared parity fixture rejected: %v", err)
	}
	canonicalSpec, err := DecodeForHash([]byte(fixture.CanonicalJSON))
	if err != nil {
		t.Fatalf("canonical fixture is not a valid arrangement: %v", err)
	}
	if got := string(canonicalJSON(canonicalSpec)); got != fixture.CanonicalJSON {
		t.Fatalf("canonical fixture changed\ngot:  %s\nwant: %s", got, fixture.CanonicalJSON)
	}
	if canonicalSpec.SpecHash != fixture.SpecHash || ContentHash(canonicalSpec) != fixture.SpecHash {
		t.Fatalf("fixed fixture hash got spec=%s content=%s want=%s", canonicalSpec.SpecHash, ContentHash(canonicalSpec), fixture.SpecHash)
	}
}

func TestNormalizeMatchesUnityFloat32BoundarySemantics(t *testing.T) {
	spec := validSpec()
	spec.Members[0].SelectionWeight = 0.9999995
	spec.Amount = 0.5500005
	spec.Orderliness = math.Copysign(0, -1)
	spec.Grouping = 0.0000005
	spec.Stacking = 0.9999995
	spec.EdgeMargin = 0.0000005

	Normalize(&spec)

	if spec.Members[0].SelectionWeight != 1 || spec.Amount != 0.55 || spec.Orderliness != 0 ||
		spec.Grouping != 0 || spec.Stacking != 1 || spec.EdgeMargin != 0 {
		t.Fatalf("unexpected float32 normalization: weight=%v amount=%v orderliness=%v grouping=%v stacking=%v edge=%v",
			spec.Members[0].SelectionWeight, spec.Amount, spec.Orderliness, spec.Grouping, spec.Stacking, spec.EdgeMargin)
	}
	if math.Signbit(spec.Orderliness) || math.Signbit(spec.Grouping) || math.Signbit(spec.EdgeMargin) {
		t.Fatal("normalized zero values must not retain a negative sign")
	}
}

func TestSharedFloat32BoundaryFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "arrangements", "float32_boundaries.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []struct {
			Name       string  `json:"name"`
			Input      float64 `json:"input"`
			Normalized float64 `json:"normalized"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Cases) == 0 {
		t.Fatalf("invalid shared float32 fixture: version=%d cases=%d", fixture.SchemaVersion, len(fixture.Cases))
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			got := round(test.Input)
			if got != test.Normalized || (got == 0 && math.Signbit(got)) {
				t.Fatalf("round(%v)=%v signbit=%v want=%v", test.Input, got, math.Signbit(got), test.Normalized)
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
