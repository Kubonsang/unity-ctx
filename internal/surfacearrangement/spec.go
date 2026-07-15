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
	"strings"
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

// Decode rejects unknown fields and trailing JSON, normalizes stable fields,
// and verifies a supplied spec_hash. A missing hash is populated so authoring
// tools can validate drafts before persisting their normalized form.
func Decode(data []byte) (Spec, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var spec Spec
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("invalid surface arrangement: %w", err)
	}
	providedHash := strings.TrimSpace(spec.SpecHash)
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Spec{}, errors.New("invalid surface arrangement: unexpected trailing JSON content")
		}
		return Spec{}, fmt.Errorf("invalid surface arrangement: %w", err)
	}
	Normalize(&spec)
	if providedHash != "" && providedHash != spec.SpecHash {
		return Spec{}, fmt.Errorf("invalid surface arrangement: spec_hash does not match normalized content got=%s want=%s", providedHash, spec.SpecHash)
	}
	if err := Validate(spec); err != nil {
		return Spec{}, err
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
	if spec.SurfaceArrangementVersion != Version {
		return fmt.Errorf("invalid surface arrangement: surface_arrangement_version must be %d", Version)
	}
	if spec.ResolverVersion != ResolverVersion {
		return fmt.Errorf("invalid surface arrangement: resolver_version must be %d", ResolverVersion)
	}
	if spec.ArrangementID == "" || spec.TargetElementID == "" || spec.TargetFrameID == "" {
		return errors.New("invalid surface arrangement: arrangement_id, target_element_id, and target_frame_id are required")
	}
	if !validPreset(spec.Preset) {
		return fmt.Errorf("invalid surface arrangement: preset must be %q, %q, or %q", PresetNeat, PresetInUse, PresetScattered)
	}
	if len(spec.Members) == 0 || len(spec.Members) > MaximumItemCount {
		return fmt.Errorf("invalid surface arrangement: members must contain between 1 and %d entries", MaximumItemCount)
	}
	seen := make(map[string]bool, len(spec.Members))
	minimumTotal, maximumTotal := 0, 0
	positiveWeight := false
	for index, member := range spec.Members {
		if member.DescriptorID == "" || member.AffinityGroup == "" {
			return fmt.Errorf("invalid surface arrangement: members[%d] requires descriptor_id and affinity_group", index)
		}
		if seen[member.DescriptorID] {
			return fmt.Errorf("invalid surface arrangement: duplicate member descriptor_id %q", member.DescriptorID)
		}
		seen[member.DescriptorID] = true
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
		return errors.New("invalid surface arrangement: edge_margin must be finite and >= 0")
	}
	if spec.MaxStackHeight < 1 || spec.MaxStackHeight > MaximumStackHeight {
		return fmt.Errorf("invalid surface arrangement: max_stack_height must be between 1 and %d", MaximumStackHeight)
	}
	if spec.SpecHash == "" || spec.SpecHash != ContentHash(spec) {
		return errors.New("invalid surface arrangement: spec_hash does not match normalized content")
	}
	return nil
}

// ContentHash is the SHA-256 of normalized compact JSON with spec_hash blank,
// matching the established Spatial Contract hashing convention.
func ContentHash(spec Spec) string {
	spec.SpecHash = ""
	canonicalizeWithoutHash(&spec)
	data, _ := json.Marshal(spec)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Marshal returns stable, normalized, indented JSON terminated by a newline.
func Marshal(spec Spec) ([]byte, error) {
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
		return spec.Members[i].DescriptorID < spec.Members[j].DescriptorID
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
	return math.Round(value*1_000_000) / 1_000_000
}
