package spatialcontract

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

func TestAssetContractRoundTripApprovalAndStableSave(t *testing.T) {
	contract := validAssetContract()
	authorizeForTest(t, &contract)
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

func TestChangedPayloadInvalidatesTechnicalAndReviewEvidence(t *testing.T) {
	contract := validAssetContract()
	authorizeForTest(t, &contract)
	contract.Asset.CaptureSetHash = "capture-changed"
	Normalize(&contract)
	if contract.State != StateStale || contract.Technical != nil || contract.Review != nil {
		t.Fatalf("changed contract retained evidence: state=%s technical=%#v review=%#v", contract.State, contract.Technical, contract.Review)
	}
	if err := Validate(contract); err != nil {
		t.Fatalf("Validate() stale contract error = %v", err)
	}
}

func TestAdvancedContractCannotOmitEmbeddedPayloadHash(t *testing.T) {
	contract := validAssetContract()
	contract.Asset.CollisionProxies[0].Center[0] = .25
	contract.Asset.GeometryHash = ""
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "requires an embedded geometry_hash") {
		t.Fatalf("Decode() error = %v", err)
	}
	if err := Save(filepath.Join(t.TempDir(), "draft.json"), contract); err == nil || !strings.Contains(err.Error(), "requires an embedded geometry_hash") {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestContentHashAndSaveDoNotMutateCaller(t *testing.T) {
	contract := validAssetContract()
	contract.Asset.CollisionProxies = []OBB{
		{ID: "z", Center: Vec3{0, 0, 0}, Size: Vec3{1, 1, 1}, Rotation: Quat{0, 0, 0, 1}},
		{ID: "a", Center: Vec3{1, 0, 0}, Size: Vec3{1, 1, 1}, Rotation: Quat{0, 0, 0, 1}},
	}
	contract.Asset.GeometryHash = assetHash(*contract.Asset)
	originalFirstID := contract.Asset.CollisionProxies[0].ID
	_ = ContentHash(contract)
	if contract.Asset.CollisionProxies[0].ID != originalFirstID {
		t.Fatalf("ContentHash mutated caller proxies: %#v", contract.Asset.CollisionProxies)
	}
	// Save validates the cloned normalized value; a validation failure must also
	// leave the caller untouched.
	_ = Save(filepath.Join(t.TempDir(), "contract.json"), contract)
	if contract.Asset.CollisionProxies[0].ID != originalFirstID {
		t.Fatalf("Save mutated caller proxies: %#v", contract.Asset.CollisionProxies)
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
	authorizeForTest(t, &contract)
	dir := t.TempDir()
	draft := filepath.Join(dir, "draft.json")
	current := filepath.Join(dir, "Assets", "SpatialContracts", "Assets", contract.Asset.AssetGUID+".spatial.json")
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
	if err == nil || !strings.Contains(err.Error(), "authorized local review bridge") {
		t.Fatalf("public write Apply() result=%+v err=%v", result, err)
	}
	result, err = ApplyAuthorized(current, draft, testApprovalVerifier{})
	if err != nil || !result.Written || !result.Verified {
		t.Fatalf("ApplyAuthorized() result=%+v err=%v", result, err)
	}
	if _, err := Load(current); err != nil {
		t.Fatalf("written contract invalid: %v", err)
	}
}

func TestPublicReviewCannotApprove(t *testing.T) {
	contract := validAssetContract()
	if err := Approve(&contract, "arbitrary-reviewer"); err == nil || !strings.Contains(err.Error(), "local review bridge") {
		t.Fatalf("Approve() error = %v", err)
	}
	if err := Review(&contract, StateApproved, "arbitrary-reviewer", nil, ""); err == nil || !strings.Contains(err.Error(), "local review bridge") {
		t.Fatalf("Review(Approved) error = %v", err)
	}
	if contract.State != StateAwaitingHumanReview || contract.Review != nil {
		t.Fatalf("unauthorized review mutated contract: state=%s review=%#v", contract.State, contract.Review)
	}
}

func TestReviewAuthorizedRejectsMissingOrInvalidEvidence(t *testing.T) {
	contract := validAssetContract()
	if err := ReviewAuthorized(&contract, "local-user", ApprovalEvidence{}, testApprovalVerifier{}); err == nil || !strings.Contains(err.Error(), "authority, nonce, and proof") {
		t.Fatalf("empty evidence error = %v", err)
	}
	bad := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "forged"}
	if err := ReviewAuthorized(&contract, "local-user", bad, testApprovalVerifier{}); err == nil || !strings.Contains(err.Error(), "authorization failed") {
		t.Fatalf("forged evidence error = %v", err)
	}
	if contract.State != StateAwaitingHumanReview || contract.Review != nil {
		t.Fatalf("failed authorization mutated contract: state=%s review=%#v", contract.State, contract.Review)
	}
}

func TestGeometryMutationMarksApprovedContractStale(t *testing.T) {
	contract := validAssetContract()
	authorizeForTest(t, &contract)
	contract.Asset.CollisionProxies[0].Center[0] = 0.25
	Normalize(&contract)
	if contract.State != StateStale || contract.Technical != nil || contract.Review != nil {
		t.Fatalf("geometry mutation retained evidence: state=%s technical=%#v review=%#v", contract.State, contract.Technical, contract.Review)
	}
}

func TestApplyRejectsIdentityMismatchAndNonCanonicalDestination(t *testing.T) {
	directory := t.TempDir()
	currentContract := validAssetContract()
	authorizeForTest(t, &currentContract)
	draftContract := validAssetContract()
	draftContract.Asset.AssetGUID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	draftContract.Asset.AssetPath = "Assets/KayKit/banner-blue.prefab"
	draftContract.Asset.GeometryHash = ""
	Normalize(&draftContract)
	authorizeForTest(t, &draftContract)

	current := filepath.Join(directory, "Assets", "SpatialContracts", "Assets", draftContract.Asset.AssetGUID+".spatial.json")
	draft := filepath.Join(directory, "draft.json")
	if err := Save(current, currentContract); err != nil {
		t.Fatal(err)
	}
	if err := Save(draft, draftContract); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(current, draft, false); err == nil || !strings.Contains(err.Error(), "identities do not match") {
		t.Fatalf("identity mismatch Apply() error = %v", err)
	}

	nonCanonical := filepath.Join(directory, "output", draftContract.Asset.AssetGUID+".spatial.json")
	if _, err := Apply(nonCanonical, draft, false); err == nil || !strings.Contains(err.Error(), "must be under") {
		t.Fatalf("non-canonical Apply() error = %v", err)
	}
}

func TestApplyAuthorizedRejectsLegacyReviewerStringWithoutEvidence(t *testing.T) {
	contract := validAssetContract()
	authorizeForTest(t, &contract)
	contract.Review.Authorization = nil
	directory := t.TempDir()
	draft := filepath.Join(directory, "draft.json")
	current := filepath.Join(directory, "Assets", "SpatialContracts", "Assets", contract.Asset.AssetGUID+".spatial.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyAuthorized(current, draft, testApprovalVerifier{}); err == nil || !strings.Contains(err.Error(), "bridge-verifiable") {
		t.Fatalf("ApplyAuthorized() error = %v", err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("unauthorized apply wrote current: %v", err)
	}
}

func TestValidateRequiresTechnicalReportHash(t *testing.T) {
	contract := validAssetContract()
	contract.Technical.ReportHash = ""
	if err := Validate(contract); err == nil || !strings.Contains(err.Error(), "technical.report_hash") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestInteractionContractSupportedBy(t *testing.T) {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeInteraction,
		State:           StateAwaitingHumanReview,
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
	authorizeForTest(t, &contract)
}

func TestOverlayApprovedAssetsRequiresGUIDAndDependencyHashAndCopiesContactPolicy(t *testing.T) {
	contract := validAssetContract()
	authorizeForTest(t, &contract)
	root := t.TempDir()
	if err := Save(filepath.Join(root, "banner.spatial.json"), contract); err != nil {
		t.Fatal(err)
	}
	manifest := bounds.Manifest{Prefabs: []bounds.PrefabBounds{{
		Path: contract.Asset.AssetPath, GUID: contract.Asset.AssetGUID,
		Spatial: &bounds.SpatialProfile{DependencyHash: contract.Asset.DependencyHash},
	}}}
	applied, err := OverlayApprovedAssets(&manifest, root)
	if err != nil || applied != 1 {
		t.Fatalf("OverlayApprovedAssets() applied=%d err=%v", applied, err)
	}
	profile := manifest.Prefabs[0].Spatial
	if !profile.Reviewed || len(profile.Contacts) != 1 || profile.Contacts[0].Kind != "WallMounted" {
		t.Fatalf("approved contact policy missing: %#v", profile)
	}

	for name, mutate := range map[string]func(*bounds.PrefabBounds){
		"guid":             func(prefab *bounds.PrefabBounds) { prefab.GUID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" },
		"blank dependency": func(prefab *bounds.PrefabBounds) { prefab.Spatial.DependencyHash = "" },
		"stale dependency": func(prefab *bounds.PrefabBounds) { prefab.Spatial.DependencyHash = "stale" },
	} {
		t.Run(name, func(t *testing.T) {
			prefab := bounds.PrefabBounds{Path: contract.Asset.AssetPath, GUID: contract.Asset.AssetGUID, Spatial: &bounds.SpatialProfile{DependencyHash: contract.Asset.DependencyHash}}
			mutate(&prefab)
			candidate := bounds.Manifest{Prefabs: []bounds.PrefabBounds{prefab}}
			applied, err := OverlayApprovedAssets(&candidate, root)
			if err != nil || applied != 0 || candidate.Prefabs[0].Spatial.Reviewed {
				t.Fatalf("unproven overlay applied=%d err=%v profile=%#v", applied, err, candidate.Prefabs[0].Spatial)
			}
		})
	}
}

func validAssetContract() Contract {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeAsset,
		State:           StateAwaitingHumanReview,
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

type testApprovalVerifier struct{}

func (testApprovalVerifier) VerifyApproval(verification ApprovalVerification) error {
	if verification.ContractHash == "" || verification.CaptureSetHash == "" || verification.Reviewer == "" {
		return errors.New("approval binding is incomplete")
	}
	if verification.Evidence.Authority != "test-bridge" || verification.Evidence.Nonce != "nonce-1" || verification.Evidence.Proof != "valid-proof" {
		return errors.New("invalid approval proof")
	}
	return nil
}

func authorizeForTest(t *testing.T, contract *Contract) {
	t.Helper()
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	if err := ReviewAuthorized(contract, "local-user", evidence, testApprovalVerifier{}); err != nil {
		t.Fatalf("ReviewAuthorized() error = %v", err)
	}
}
