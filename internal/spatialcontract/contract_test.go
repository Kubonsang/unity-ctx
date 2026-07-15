package spatialcontract

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetContractRoundTripApprovalAndStableSave(t *testing.T) {
	contract := validAssetContract()
	if err := Approve(&contract, "local-user"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "banner.spatial.json")
	if err := Save(path, contract); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	first, _ := os.ReadFile(path)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.State != StateApproved || loaded.Review == nil || loaded.Review.Decision != StateApproved {
		t.Fatalf("loaded review = %#v state=%s", loaded.Review, loaded.State)
	}
	if err := Save(path, loaded); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("stable save changed bytes")
	}
}

func TestApprovedContractRejectsChangedCaptureHash(t *testing.T) {
	contract := validAssetContract()
	if err := Approve(&contract, "local-user"); err != nil {
		t.Fatal(err)
	}
	contract.Asset.CaptureSetHash = "capture-changed"
	Normalize(&contract)
	if err := Validate(contract); err == nil || !strings.Contains(err.Error(), "contract_hash is stale") {
		t.Fatalf("Validate() error = %v, want stale review", err)
	}
}

func TestNegativeZeroHashMatchesApprovedTableVector(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeAsset,
		State:           StateApproved,
		Asset: &AssetSpatialContract{
			AssetGUID:      "a3fa97880303a42f48b512df88e92628",
			AssetPath:      "Assets/KayKit_DungeonRemastered_1.1_SOURCE/Assets/fbx(unity)/table_medium.fbx",
			DependencyHash: "283f72daa9e5a3be1a1ef522e9b5d527",
			Units:          "meter",
			Forward:        Vec3{0, 0, 1},
			Up:             Vec3{0, 1, 0},
			PivotOffset:    Vec3{negativeZero, 0.5, 0},
			CollisionProxies: []OBB{{
				ID:       "renderer-0",
				Center:   Vec3{negativeZero, 0.5, 0},
				Size:     Vec3{2, 1, 2},
				Rotation: Quat{0, 0, 0, 1},
			}},
			Frames: []ContactFrame{
				{ID: "back", Point: Vec3{negativeZero, 0.5, -1}, Normal: Vec3{0, 0, -1}, Tangent: Vec3{1, 0, 0}, Size: [2]float64{2, 1.000001}},
				{ID: "bottom", Point: Vec3{negativeZero, negativeZero, 0}, Normal: Vec3{0, -1, 0}, Tangent: Vec3{1, 0, 0}, Size: [2]float64{2, 2}},
				{ID: "top", Point: Vec3{negativeZero, 1, 0}, Normal: Vec3{0, 1, 0}, Tangent: Vec3{1, 0, 0}, Size: [2]float64{2, 2}},
			},
			Contacts: []ContactRequirement{{
				ID: "floor", Kind: "FloorSupported", FrameID: "bottom", Target: "surface:floor",
				MaximumGap: 0.01, MinimumSupport: 0.6, DirectionAlignment: 0.95,
			}},
			Revision:       1,
			CaptureSetHash: "f30da4a06f6652089688e95fbe19919f8b42783bb446f9874486ad91aebfe2a4",
		},
		Technical: &TechnicalEvidence{
			Passed: true, ErrorCount: 0,
			ReportHash: "685b1cfc6d78dc1e9fab3abca0c9fd947d5e75d89e0045a069be3651b8119bb7",
		},
	}
	Normalize(&contract)
	if !math.Signbit(contract.Asset.PivotOffset[0]) {
		t.Fatal("Normalize() erased IEEE-754 negative zero")
	}
	const geometryHash = "03ef438f92d4be2408a85f88cecd0f57f4d67ee826dce2b711740bfe19ade10b"
	if contract.Asset.GeometryHash != geometryHash {
		t.Fatalf("geometry hash = %s, want %s", contract.Asset.GeometryHash, geometryHash)
	}
	const contractHash = "595585303e6629aa7f9d6755e91d5e2c2aae59f5ac51e6798caaf4b9a4fa447e"
	if got := ContentHash(contract); got != contractHash {
		t.Fatalf("content hash = %s, want %s", got, contractHash)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	contract := validAssetContract()
	Normalize(&contract)
	path := filepath.Join(t.TempDir(), "draft.json")
	if err := Save(path, contract); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), `"contract_version": 1,`, `"contract_version": 1, "surprise": true,`, 1))
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Decode() error = %v", err)
	}
}

func TestApplyIsDryRunUntilWrite(t *testing.T) {
	contract := validAssetContract()
	if err := Approve(&contract, "local-user"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	draft := filepath.Join(dir, "draft.json")
	current := filepath.Join(dir, "Assets", "SpatialContracts", "banner.spatial.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(current, draft, false)
	if err != nil || result.Written {
		t.Fatalf("dry Apply() result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote current: %v", err)
	}
	result, err = Apply(current, draft, true)
	if err != nil || !result.Written || !result.Verified {
		t.Fatalf("write Apply() result=%+v err=%v", result, err)
	}
	if _, err := Load(current); err != nil {
		t.Fatalf("written contract invalid: %v", err)
	}
}

func TestInteractionContractSupportedBy(t *testing.T) {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeInteraction,
		State:           StateTechnicalPassed,
		Interaction: &InteractionContract{
			SubjectGUID:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			TargetKey:         "asset:cccccccccccccccccccccccccccccccc",
			Relation:          "SupportedBy",
			SubjectFrame:      "bottom",
			TargetFrame:       "top",
			RelativeRotation:  Quat{0, 0, 0, 1},
			PositionTolerance: Vec3{0.2, 0.01, 0.2},
			AngleTolerance:    180,
			CollisionPolicy:   "contact-only",
			Revision:          1,
			CaptureSetHash:    "capture-table-prop",
		},
		Technical: &TechnicalEvidence{Passed: true, ReportHash: "report"},
	}
	Normalize(&contract)
	if err := Approve(&contract, "local-user"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
}

func validAssetContract() Contract {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeAsset,
		State:           StateTechnicalPassed,
		Asset: &AssetSpatialContract{
			AssetGUID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AssetPath:        "Assets/KayKit/banner.prefab",
			DependencyHash:   "dependency-v1",
			Units:            "meter",
			Forward:          Vec3{0, 0, 1},
			Up:               Vec3{0, 1, 0},
			CollisionProxies: []OBB{{ID: "banner", Center: Vec3{0, 1, 0}, Size: Vec3{1, 2, 0.05}, Rotation: Quat{0, 0, 0, 1}}},
			Frames:           []ContactFrame{{ID: "back", Point: Vec3{0, 1, -0.025}, Normal: Vec3{0, 0, -1}, Tangent: Vec3{1, 0, 0}, Size: [2]float64{1, 2}}},
			Contacts:         []ContactRequirement{{ID: "wall", Kind: "WallMounted", FrameID: "back", Target: "surface:wall", MinimumGap: 0.005, MaximumGap: 0.01, MinimumSupport: 0.6, DirectionAlignment: 0.95}},
			Revision:         1,
			CaptureSetHash:   "capture-banner",
		},
		Technical: &TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-banner"},
	}
	Normalize(&contract)
	return contract
}
