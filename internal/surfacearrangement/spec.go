package surfacearrangement

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	Version            = 1
	ResolverVersion    = 1
	MaximumItemCount   = 12
	MaximumStackHeight = 3
)

type Preset string

const (
	PresetNeat      Preset = "Neat"
	PresetInUse     Preset = "InUse"
	PresetScattered Preset = "Scattered"
)

// Member describes one eligible descriptor and its count range in an
// arrangement. SelectionWeight is a normalized, relative preference; it does
// not relax any geometry or support validation.
type Member struct {
	DescriptorID    string  `json:"descriptor_id"`
	MinimumCount    int     `json:"minimum_count"`
	MaximumCount    int     `json:"maximum_count"`
	SelectionWeight float64 `json:"selection_weight"`
	AffinityGroup   string  `json:"affinity_group"`
}

// Spec is independent from Spatial Contract v1. It describes how already
// approved support interactions may be composed on a target surface.
type Spec struct {
	SurfaceArrangementVersion int      `json:"surface_arrangement_version"`
	ArrangementID             string   `json:"arrangement_id"`
	TargetElementID           string   `json:"target_element_id"`
	TargetFrameID             string   `json:"target_frame_id"`
	Members                   []Member `json:"members"`
	Preset                    Preset   `json:"preset"`
	Amount                    float64  `json:"amount"`
	Orderliness               float64  `json:"orderliness"`
	Grouping                  float64  `json:"grouping"`
	Stacking                  float64  `json:"stacking"`
	EdgeMargin                float64  `json:"edge_margin"`
	MaxStackHeight            int      `json:"max_stack_height"`
	SeedOffset                int64    `json:"seed_offset"`
	ResolverVersion           int      `json:"resolver_version"`
	SpecHash                  string   `json:"spec_hash"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	return Decode(data)
}

// LoadForHash loads and validates a spec while intentionally ignoring an
// embedded spec_hash. It is used by the read-only hash command so callers can
// calculate the replacement for a stale hash without weakening validation.
func LoadForHash(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	return DecodeForHash(data)
}

// Decode rejects unknown fields and trailing JSON, normalizes stable fields,
// and strictly verifies the required spec_hash. Draft authoring and repair
// tools must use DecodeForHash so the validate path cannot bless an unhashed
// document by inventing its integrity value.
func Decode(data []byte) (Spec, error) {
	spec, err := decodeDocument(data)
	if err != nil {
		return Spec{}, err
	}
	providedHash := strings.TrimSpace(spec.SpecHash)
	if err := validateFields(spec); err != nil {
		return Spec{}, err
	}
	if providedHash == "" {
		return Spec{}, errors.New("invalid surface arrangement: spec_hash is required")
	}
	Normalize(&spec)
	if providedHash != spec.SpecHash {
		return Spec{}, fmt.Errorf("invalid surface arrangement: spec_hash does not match normalized content got=%s want=%s", providedHash, spec.SpecHash)
	}
	if err := Validate(spec); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// DecodeForHash is the schema-strict parsing path for recomputing a spec hash.
// It validates raw values but replaces (rather than verifies) a missing or
// stale embedded spec_hash.
func DecodeForHash(data []byte) (Spec, error) {
	spec, err := decodeDocument(data)
	if err != nil {
		return Spec{}, err
	}
	if err := validateFields(spec); err != nil {
		return Spec{}, err
	}
	Normalize(&spec)
	if err := Validate(spec); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func decodeDocument(data []byte) (Spec, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var spec Spec
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("invalid surface arrangement: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Spec{}, errors.New("invalid surface arrangement: unexpected trailing JSON content")
		}
		return Spec{}, fmt.Errorf("invalid surface arrangement: %w", err)
	}
	return spec, nil
}

// Normalize canonicalizes all hash-relevant values without inventing required
// identifiers, versions, member counts, or preset choices.
func Normalize(spec *Spec) {
	if spec == nil {
		return
	}
	canonicalizeWithoutHash(spec)
	spec.SpecHash = ContentHash(*spec)
}

func Validate(spec Spec) error {
	if err := validateFields(spec); err != nil {
		return err
	}
	if spec.SpecHash == "" || spec.SpecHash != ContentHash(spec) {
		return errors.New("invalid surface arrangement: spec_hash does not match normalized content")
	}
	return nil
}

// validateFields intentionally runs before normalization. This prevents
// slightly invalid raw values (for example 1.0000004) from being rounded into
// the accepted range by a parser before the schema sees them.
func validateFields(spec Spec) error {
	if spec.SurfaceArrangementVersion != Version {
		return fmt.Errorf("invalid surface arrangement: surface_arrangement_version must be %d", Version)
	}
	if spec.ResolverVersion != ResolverVersion {
		return fmt.Errorf("invalid surface arrangement: resolver_version must be %d", ResolverVersion)
	}
	if spec.ArrangementID == "" || spec.TargetElementID == "" || spec.TargetFrameID == "" {
		return errors.New("invalid surface arrangement: arrangement_id, target_element_id, and target_frame_id are required")
	}
	if !validPreset(Preset(strings.TrimSpace(string(spec.Preset)))) {
		return fmt.Errorf("invalid surface arrangement: preset must be %q, %q, or %q", PresetNeat, PresetInUse, PresetScattered)
	}
	if len(spec.Members) == 0 || len(spec.Members) > MaximumItemCount {
		return fmt.Errorf("invalid surface arrangement: members must contain between 1 and %d entries", MaximumItemCount)
	}
	seen := make(map[string]bool, len(spec.Members))
	minimumTotal, maximumTotal := 0, 0
	positiveWeight := false
	for index, member := range spec.Members {
		descriptorID := strings.TrimSpace(member.DescriptorID)
		affinityGroup := strings.TrimSpace(member.AffinityGroup)
		if descriptorID == "" || affinityGroup == "" {
			return fmt.Errorf("invalid surface arrangement: members[%d] requires descriptor_id and affinity_group", index)
		}
		if seen[descriptorID] {
			return fmt.Errorf("invalid surface arrangement: duplicate member descriptor_id %q", descriptorID)
		}
		seen[descriptorID] = true
		if member.MinimumCount < 0 || member.MaximumCount < 1 || member.MinimumCount > member.MaximumCount || member.MaximumCount > MaximumItemCount {
			return fmt.Errorf("invalid surface arrangement: member %q counts must satisfy 0 <= minimum_count <= maximum_count <= %d and maximum_count >= 1", member.DescriptorID, MaximumItemCount)
		}
		if !unitInterval(member.SelectionWeight) {
			return fmt.Errorf("invalid surface arrangement: member %q selection_weight must be finite and between 0 and 1", member.DescriptorID)
		}
		positiveWeight = positiveWeight || member.SelectionWeight > 0
		minimumTotal += member.MinimumCount
		maximumTotal += member.MaximumCount
	}
	if minimumTotal < 1 || maximumTotal > MaximumItemCount {
		return fmt.Errorf("invalid surface arrangement: total member counts must satisfy 1 <= minimum total <= maximum total <= %d", MaximumItemCount)
	}
	if !positiveWeight {
		return errors.New("invalid surface arrangement: at least one member selection_weight must be greater than 0")
	}
	if !unitInterval(spec.Amount) || !unitInterval(spec.Orderliness) || !unitInterval(spec.Grouping) || !unitInterval(spec.Stacking) {
		return errors.New("invalid surface arrangement: amount, orderliness, grouping, and stacking must be finite and between 0 and 1")
	}
	if !finite(spec.EdgeMargin) || spec.EdgeMargin < 0 {
		return errors.New("invalid surface arrangement: edge_margin must be finite and non-negative")
	}
	if spec.MaxStackHeight < 1 || spec.MaxStackHeight > MaximumStackHeight {
		return fmt.Errorf("invalid surface arrangement: max_stack_height must be between 1 and %d", MaximumStackHeight)
	}
	if spec.SeedOffset < 0 {
		return errors.New("invalid surface arrangement: seed_offset must be non-negative")
	}
	return nil
}

// ContentHash is the SHA-256 of normalized compact JSON with spec_hash blank,
// matching the established Spatial Contract hashing convention.
func ContentHash(spec Spec) string {
	spec = canonicalCopy(spec)
	spec.SpecHash = ""
	canonicalizeWithoutHash(&spec)
	data := canonicalJSON(spec)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Marshal returns stable, normalized, indented JSON terminated by a newline.
func Marshal(spec Spec) ([]byte, error) {
	spec = canonicalCopy(spec)
	Normalize(&spec)
	if err := Validate(spec); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func canonicalCopy(spec Spec) Spec {
	spec.Members = append([]Member(nil), spec.Members...)
	return spec
}

func canonicalizeWithoutHash(spec *Spec) {
	spec.ArrangementID = strings.TrimSpace(spec.ArrangementID)
	spec.TargetElementID = strings.TrimSpace(spec.TargetElementID)
	spec.TargetFrameID = strings.TrimSpace(spec.TargetFrameID)
	spec.Preset = Preset(strings.TrimSpace(string(spec.Preset)))
	for i := range spec.Members {
		spec.Members[i].DescriptorID = strings.TrimSpace(spec.Members[i].DescriptorID)
		spec.Members[i].AffinityGroup = strings.TrimSpace(spec.Members[i].AffinityGroup)
		spec.Members[i].SelectionWeight = round(spec.Members[i].SelectionWeight)
	}
	sort.SliceStable(spec.Members, func(i, j int) bool {
		return utf16OrdinalLess(spec.Members[i].DescriptorID, spec.Members[j].DescriptorID)
	})
	spec.Amount = round(spec.Amount)
	spec.Orderliness = round(spec.Orderliness)
	spec.Grouping = round(spec.Grouping)
	spec.Stacking = round(spec.Stacking)
	spec.EdgeMargin = round(spec.EdgeMargin)
}

func validPreset(value Preset) bool {
	switch value {
	case PresetNeat, PresetInUse, PresetScattered:
		return true
	default:
		return false
	}
}

func unitInterval(value float64) bool {
	return finite(value) && value >= 0 && value <= 1
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func round(value float64) float64 {
	// Unity stores arrangement numbers as System.Single. Reproduce that wire
	// precision before rounding so Go and C# hash boundary decimals identically.
	value = float64(float32(value))
	rounded := math.Round(value*1_000_000) / 1_000_000
	if math.Abs(rounded) < 0.0000005 {
		return 0
	}
	return rounded
}

// utf16OrdinalLess reproduces StringComparer.Ordinal. Go compares strings by
// UTF-8 bytes, while Unity compares UTF-16 code units; the distinction matters
// when a member ID contains supplementary Unicode characters.
func utf16OrdinalLess(left, right string) bool {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	limit := len(leftUnits)
	if len(rightUnits) < limit {
		limit = len(rightUnits)
	}
	for index := 0; index < limit; index++ {
		if leftUnits[index] != rightUnits[index] {
			return leftUnits[index] < rightUnits[index]
		}
	}
	return len(leftUnits) < len(rightUnits)
}

// canonicalJSON mirrors Unity's SurfaceArrangementSpecUtility.CanonicalJson:
// fixed field order, StringComparer.Ordinal member order, System.Single input
// precision, at most six fixed decimal places, and JSON's HTML-safe escaping.
func canonicalJSON(spec Spec) []byte {
	var result strings.Builder
	result.Grow(1024)
	result.WriteByte('{')
	appendIntegerField(&result, "surface_arrangement_version", int64(spec.SurfaceArrangementVersion))
	result.WriteByte(',')
	appendStringField(&result, "arrangement_id", spec.ArrangementID)
	result.WriteByte(',')
	appendStringField(&result, "target_element_id", spec.TargetElementID)
	result.WriteByte(',')
	appendStringField(&result, "target_frame_id", spec.TargetFrameID)
	result.WriteByte(',')
	appendJSONName(&result, "members")
	result.WriteByte('[')
	for index, member := range spec.Members {
		if index > 0 {
			result.WriteByte(',')
		}
		result.WriteByte('{')
		appendStringField(&result, "descriptor_id", member.DescriptorID)
		result.WriteByte(',')
		appendIntegerField(&result, "minimum_count", int64(member.MinimumCount))
		result.WriteByte(',')
		appendIntegerField(&result, "maximum_count", int64(member.MaximumCount))
		result.WriteByte(',')
		appendNumberField(&result, "selection_weight", member.SelectionWeight)
		result.WriteByte(',')
		appendStringField(&result, "affinity_group", member.AffinityGroup)
		result.WriteByte('}')
	}
	result.WriteByte(']')
	result.WriteByte(',')
	appendStringField(&result, "preset", string(spec.Preset))
	result.WriteByte(',')
	appendNumberField(&result, "amount", spec.Amount)
	result.WriteByte(',')
	appendNumberField(&result, "orderliness", spec.Orderliness)
	result.WriteByte(',')
	appendNumberField(&result, "grouping", spec.Grouping)
	result.WriteByte(',')
	appendNumberField(&result, "stacking", spec.Stacking)
	result.WriteByte(',')
	appendNumberField(&result, "edge_margin", spec.EdgeMargin)
	result.WriteByte(',')
	appendIntegerField(&result, "max_stack_height", int64(spec.MaxStackHeight))
	result.WriteByte(',')
	appendIntegerField(&result, "seed_offset", spec.SeedOffset)
	result.WriteByte(',')
	appendIntegerField(&result, "resolver_version", int64(spec.ResolverVersion))
	result.WriteByte(',')
	appendStringField(&result, "spec_hash", "")
	result.WriteByte('}')
	return []byte(result.String())
}

func appendJSONName(result *strings.Builder, name string) {
	appendJSONString(result, name)
	result.WriteByte(':')
}

func appendStringField(result *strings.Builder, name, value string) {
	appendJSONName(result, name)
	appendJSONString(result, value)
}

func appendIntegerField(result *strings.Builder, name string, value int64) {
	appendJSONName(result, name)
	result.WriteString(strconv.FormatInt(value, 10))
}

func appendNumberField(result *strings.Builder, name string, value float64) {
	appendJSONName(result, name)
	formatted := strconv.FormatFloat(round(value), 'f', 6, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if formatted == "" || formatted == "-0" {
		formatted = "0"
	}
	result.WriteString(formatted)
}

func appendJSONString(result *strings.Builder, value string) {
	// encoding/json deliberately keeps HTML escaping enabled here because
	// Unity's canonical Quote function emits the same lower-case escapes.
	encoded, _ := json.Marshal(value)
	result.Write(encoded)
}
