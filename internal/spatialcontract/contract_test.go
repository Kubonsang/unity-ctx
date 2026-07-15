package spatialcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestFloat32OverflowFailsClosedBeforeApprovalHashes(t *testing.T) {
	assetCases := map[string]func(*AssetSpatialContract){
		"pivot offset": func(asset *AssetSpatialContract) { asset.PivotOffset[0] = 1e39 },
		"obb center":   func(asset *AssetSpatialContract) { asset.CollisionProxies[0].Center[0] = 1e39 },
		"frame point":  func(asset *AssetSpatialContract) { asset.Frames[0].Point[0] = 1e39 },
		"frame size":   func(asset *AssetSpatialContract) { asset.Frames[0].Size[0] = 1e39 },
		"contact gap":  func(asset *AssetSpatialContract) { asset.Contacts[0].MaximumGap = 1e39 },
	}
	for name, mutate := range assetCases {
		t.Run(name, func(t *testing.T) {
			contract := validAssetContract()
			mutate(contract.Asset)
			Normalize(&contract)
			if err := Validate(contract); err == nil || !strings.Contains(err.Error(), "invalid spatial contract") {
				t.Fatalf("Validate() overflow error = %v", err)
			}
			if hash, err := ContentHashChecked(contract); err == nil || hash != "" {
				t.Fatalf("ContentHashChecked() hash=%q err=%v", hash, err)
			}
			if hash, err := ProposalHashChecked(contract); err == nil || hash != "" {
				t.Fatalf("ProposalHashChecked() hash=%q err=%v", hash, err)
			}
			if ContentHash(contract) != "" || ProposalHash(contract) != "" {
				t.Fatal("invalid overflow payload produced an authorizable compatibility hash")
			}
		})
	}

	interactionCases := map[string]func(*InteractionContract){
		"relative position": func(value *InteractionContract) { value.RelativePosition[0] = 1e39 },
		"angle tolerance":   func(value *InteractionContract) { value.AngleTolerance = 1e39 },
	}
	for name, mutate := range interactionCases {
		t.Run("interaction "+name, func(t *testing.T) {
			contract := validInteractionContractForOverflowTest()
			mutate(contract.Interaction)
			Normalize(&contract)
			if err := Validate(contract); err == nil {
				t.Fatal("Validate() accepted interaction float32 overflow")
			}
			if hash, err := ContentHashChecked(contract); err == nil || hash != "" {
				t.Fatalf("ContentHashChecked() hash=%q err=%v", hash, err)
			}
		})
	}
}

func TestDecodeRejectsFiniteFloat64ThatOverflowsUnityFloat32(t *testing.T) {
	contract := validAssetContract()
	contract.State = StateDraft
	contract.Technical = nil
	contract.Review = nil
	contract.Asset.GeometryHash = ""
	contract.Asset.CollisionProxies[0].Center[0] = 1e39
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "center must be finite") {
		t.Fatalf("Decode() overflow error = %v", err)
	}
}

func validInteractionContractForOverflowTest() Contract {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeInteraction,
		State:           StateAwaitingHumanReview,
		Interaction: &InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "asset:cccccccccccccccccccccccccccccccc", Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: Quat{0, 0, 0, 1},
			PositionTolerance: Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-interaction",
		},
		Technical: &TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-interaction"},
	}
	Normalize(&contract)
	return contract
}

func TestProposalHashExcludesCaptureAndEmbeddedPayloadHashes(t *testing.T) {
	contract := validAssetContract()
	base := ProposalHash(contract)
	recaptured := cloneContract(contract)
	recaptured.Asset.CaptureSetHash = "capture-replacement"
	recaptured.Asset.GeometryHash = strings.Repeat("f", 64)
	recaptured.Technical.ReportHash = "different-report"
	if got := ProposalHash(recaptured); got != base {
		t.Fatalf("recapture changed stable proposal hash: got=%s want=%s", got, base)
	}
	changed := cloneContract(contract)
	changed.Asset.CollisionProxies[0].Size[0] += .25
	changed.Asset.GeometryHash = ""
	if got := ProposalHash(changed); got == base {
		t.Fatal("geometry change did not change proposal hash")
	}

	interaction := Contract{
		ContractVersion: ContractVersion, ContractType: TypeInteraction, State: StateAwaitingHumanReview,
		Interaction: &InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "asset:cccccccccccccccccccccccccccccccc", Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: Quat{0, 0, 0, 1},
			PositionTolerance: Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-one",
		},
		Technical: &TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report"},
	}
	Normalize(&interaction)
	interactionHash := ProposalHash(interaction)
	interaction.Interaction.CaptureSetHash = "capture-two"
	interaction.Interaction.InteractionHash = strings.Repeat("e", 64)
	if got := ProposalHash(interaction); got != interactionHash {
		t.Fatalf("interaction recapture changed proposal hash: got=%s want=%s", got, interactionHash)
	}
	if interactionHash == base {
		t.Fatal("asset and interaction proposal hash domains collided")
	}
}

func TestProposalHashGoldenCases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "spatial", "proposal_hash_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Version int `json:"version"`
		Cases   []struct {
			Name         string   `json:"name"`
			Contract     Contract `json:"contract"`
			PayloadHash  string   `json:"payload_hash"`
			ProposalHash string   `json:"proposal_hash"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Version != 1 || len(vectors.Cases) != 2 {
		t.Fatalf("proposal hash fixture shape = version %d cases %d", vectors.Version, len(vectors.Cases))
	}
	for _, vector := range vectors.Cases {
		t.Run(vector.Name, func(t *testing.T) {
			if vector.PayloadHash == "" || vector.ProposalHash == "" {
				normalized := cloneContract(vector.Contract)
				Normalize(&normalized)
				payloadHash := ""
				if normalized.Asset != nil {
					payloadHash = normalized.Asset.GeometryHash
				} else if normalized.Interaction != nil {
					payloadHash = normalized.Interaction.InteractionHash
				}
				t.Fatalf("fill golden values: payload_hash=%s proposal_hash=%s", payloadHash, ProposalHash(normalized))
			}
			if err := Validate(vector.Contract); err != nil {
				t.Fatalf("golden contract is invalid: %v", err)
			}
			payloadHash := ""
			if vector.Contract.Asset != nil {
				payloadHash = vector.Contract.Asset.GeometryHash
			} else if vector.Contract.Interaction != nil {
				payloadHash = vector.Contract.Interaction.InteractionHash
			}
			if payloadHash != vector.PayloadHash {
				t.Fatalf("payload hash = %s, want %s", payloadHash, vector.PayloadHash)
			}
			if got := ProposalHash(vector.Contract); got != vector.ProposalHash {
				t.Fatalf("proposal hash = %s, want %s", got, vector.ProposalHash)
			}
		})
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

func TestHashMatchesUnityFloat32AndLiteralAmpersandVector(t *testing.T) {
	contract := validAssetContract()
	contract.Asset.AssetPath = "Assets/Props/A&B.prefab"
	contract.Asset.PivotOffset[0] = 0.5500005
	contract.Asset.GeometryHash = ""
	Normalize(&contract)
	if contract.Asset.PivotOffset[0] != 0.55 {
		t.Fatalf("float32-first normalized value = %.9f, want 0.55", contract.Asset.PivotOffset[0])
	}
	asset := *contract.Asset
	asset.GeometryHash = ""
	canonical, err := marshalCanonical(asset)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(canonical), `\u0026`) || !strings.Contains(string(canonical), `A&B.prefab`) {
		t.Fatalf("canonical JSON escaped ampersand differently from Unity: %s", canonical)
	}
	const geometryHash = "e936d7f62d75993c19bd44ba4bba22dfc9f862c99c85301ccf5e84b51863ad60"
	if contract.Asset.GeometryHash != geometryHash {
		t.Fatalf("geometry hash = %s, want Unity vector %s", contract.Asset.GeometryHash, geometryHash)
	}
	const contentHash = "8118e7ac51b8c8167e3fb1c9102a35f1c8bdf2fb0d2fab534ac15b439caaeeca"
	if got := ContentHash(contract); got != contentHash {
		t.Fatalf("content hash = %s, want Unity vector %s", got, contentHash)
	}
}

func TestHashMatchesUnityUTF16OrderingAndLineSeparatorVector(t *testing.T) {
	contract := validAssetContract()
	contract.Asset.DependencyHash = "quoted:\"\u2028:literal:\\u2028:\u2029"
	base := contract.Asset.CollisionProxies[0]
	base.ID = "\ue000"
	supplementary := base
	supplementary.ID = "\U00010000"
	supplementary.Center[0] = 0.25
	// Deliberately provide code-point order. C# StringComparer.Ordinal sorts the
	// supplementary-plane ID first because its leading UTF-16 surrogate (D800)
	// precedes the BMP private-use character (E000).
	contract.Asset.CollisionProxies = []OBB{base, supplementary}
	contract.Asset.GeometryHash = ""
	Normalize(&contract)
	if got := contract.Asset.CollisionProxies[0].ID; got != supplementary.ID {
		t.Fatalf("first UTF-16 ordinal ID = %q, want %q", got, supplementary.ID)
	}

	asset := *contract.Asset
	asset.GeometryHash = ""
	canonical, err := marshalCanonical(asset)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(canonical, []byte("\u2028")) || !bytes.Contains(canonical, []byte("\u2029")) {
		t.Fatalf("canonical JSON did not preserve Unity's literal line separators: %s", canonical)
	}
	if !bytes.Contains(canonical, []byte(`literal:\\u2028`)) {
		t.Fatalf("canonical JSON corrupted a literal \\u2028 sequence: %s", canonical)
	}
	// Generated by Unity's SpatialContractHashUtility from the same boundary
	// vector. Pinning it catches both UTF-16 ordering and JSON separator drift.
	const geometryHash = "1dd4d0e938b2a5bcb8e41ad182ade23b0d2eb8b2dbed3b6118beb4a64df3fae4"
	if contract.Asset.GeometryHash != geometryHash {
		t.Fatalf("geometry hash = %s, want Unity boundary vector %s", contract.Asset.GeometryHash, geometryHash)
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
	if _, err = ApplyAuthorized(current, draft, testApprovalVerifier{}); err == nil || !strings.Contains(err.Error(), "atomic ApproveAndApplyAuthorized") {
		t.Fatalf("ApplyAuthorized() error=%v", err)
	}
}

func TestDiffPinsMissingAndRawCurrentFileBaseline(t *testing.T) {
	contract := validAssetContract()
	directory := t.TempDir()
	draft := filepath.Join(directory, "draft.json")
	current := filepath.Join(directory, "current.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	missing, err := Diff(current, draft)
	if err != nil || missing.CurrentHash != CurrentHashAbsent || missing.Status != "NEW" {
		t.Fatalf("missing Diff() = %+v err=%v", missing, err)
	}
	if err := Save(current, contract); err != nil {
		t.Fatal(err)
	}
	first, err := Diff(current, draft)
	if err != nil || len(first.CurrentHash) != sha256.Size*2 || first.Changed {
		t.Fatalf("existing Diff() = %+v err=%v", first, err)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, append(data, ' ', '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	formatted, err := Diff(current, draft)
	if err != nil || formatted.Changed || formatted.CurrentHash == first.CurrentHash {
		t.Fatalf("format-only Diff() = %+v first=%+v err=%v", formatted, first, err)
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

func TestInteractionCanonicalDestinationBindsEntireIdentity(t *testing.T) {
	project := t.TempDir()
	base := Contract{
		ContractVersion: ContractVersion, ContractType: TypeInteraction, State: StateAwaitingHumanReview,
		Interaction: &InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "asset:Case&A", Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: Quat{0, 0, 0, 1},
			PositionTolerance: Vec3{.2, .01, .2}, AngleTolerance: 180, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-table-prop",
		},
		Technical: &TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report"},
	}
	Normalize(&base)
	path, err := CanonicalContractPath(project, base)
	if err != nil {
		t.Fatal(err)
	}
	wantName := base.Interaction.SubjectGUID + "__" + hex.EncodeToString([]byte(base.Interaction.TargetKey)) + "__" + hex.EncodeToString([]byte(base.Interaction.Relation)) + ".interaction.json"
	if filepath.Base(path) != wantName {
		t.Fatalf("canonical interaction filename = %s, want %s", filepath.Base(path), wantName)
	}
	caseVariant := cloneContract(base)
	caseVariant.Interaction.TargetKey = "asset:case&A"
	caseVariant.Interaction.InteractionHash = ""
	Normalize(&caseVariant)
	variantPath, err := CanonicalContractPath(project, caseVariant)
	if err != nil {
		t.Fatal(err)
	}
	if sameFilesystemPath(path, variantPath) {
		t.Fatalf("case-distinct target identities collided: %s", path)
	}
	fake := filepath.Join(project, "PrefixAssets", "SpatialContracts", "Interactions", filepath.Base(path))
	if err := validateCanonicalDestination(project, fake, base); err == nil {
		t.Fatal("substring-based non-project destination was accepted")
	}
}

func TestCanonicalDestinationRejectsSymlinkedContractDirectory(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	contracts := filepath.Join(project, "Assets", "SpatialContracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(contracts, "Assets")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	contract := validAssetContract()
	path, err := CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCanonicalDestination(project, path, contract); err == nil || !strings.Contains(err.Error(), "symlink or junction") {
		t.Fatalf("symlinked destination error = %v", err)
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
	if _, err := ApplyAuthorized(current, draft, testApprovalVerifier{}); err == nil || !strings.Contains(err.Error(), "atomic ApproveAndApplyAuthorized") {
		t.Fatalf("ApplyAuthorized() error = %v", err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("unauthorized apply wrote current: %v", err)
	}
}

func TestApproveAndApplyAuthorizedConsumesGrantAndDoesNotPersistProof(t *testing.T) {
	contract := validAssetContract()
	directory := t.TempDir()
	draft := filepath.Join(directory, "Library", "SpatialDrafts", "banner.json")
	current := filepath.Join(directory, "Assets", "SpatialContracts", "Assets", contract.Asset.AssetGUID+".spatial.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	consumer := &testOneShotConsumer{}
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	result, err := ApproveAndApplyAuthorized(directory, current, draft, CurrentHashAbsent, "local-user", evidence, testApprovalVerifier{}, consumer)
	if err != nil || !result.Written || !result.Verified {
		t.Fatalf("ApproveAndApplyAuthorized() result=%+v err=%v", result, err)
	}
	loaded, err := Load(current)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateApproved || loaded.Review == nil || loaded.Review.Authorization != nil {
		t.Fatalf("approved contract persisted reusable grant evidence: %#v", loaded.Review)
	}
	if _, err := ApproveAndApplyAuthorized(directory, current, draft, CurrentHashAbsent, "local-user", evidence, testApprovalVerifier{}, consumer); err == nil || !strings.Contains(err.Error(), "APPLY_SOURCE_CHANGED") {
		t.Fatalf("reused grant error = %v", err)
	}
}

func TestApproveAndApplyAuthorizedConsumesGrantBeforeCallbackBaselineRecheck(t *testing.T) {
	directory := t.TempDir()
	draft := filepath.Join(directory, "Library", "SpatialDrafts", "banner.json")
	contract := validAssetContract()
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	current, err := CanonicalContractPath(directory, contract)
	if err != nil {
		t.Fatal(err)
	}
	consumer := &testOneShotConsumer{beforeApply: func() {
		if err := Save(current, contract); err != nil {
			t.Fatalf("inject concurrent current contract: %v", err)
		}
	}}
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	if _, err := ApproveAndApplyAuthorized(directory, current, draft, CurrentHashAbsent, "local-user", evidence, testApprovalVerifier{}, consumer); err == nil || !strings.Contains(err.Error(), "APPLY_SOURCE_CHANGED") {
		t.Fatalf("callback-time baseline error = %v", err)
	}
	if !consumer.used["test-bridge:nonce-1"] {
		t.Fatal("baseline race did not consume the one-shot grant")
	}
	loaded, err := Load(current)
	if err != nil || loaded.State != StateAwaitingHumanReview {
		t.Fatalf("baseline race overwrote concurrent current contract: state=%s err=%v", loaded.State, err)
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

func TestApproveAndApplyAuthorizedInteractionRequiresGeometryBindings(t *testing.T) {
	contract := Contract{
		ContractVersion: ContractVersion,
		ContractType:    TypeInteraction,
		State:           StateAwaitingHumanReview,
		Interaction: &InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "asset:cccccccccccccccccccccccccccccccc", Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: Quat{0, 0, 0, 1},
			PositionTolerance: Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-interaction",
		},
		Technical: &TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-interaction"},
	}
	Normalize(&contract)
	project := t.TempDir()
	draft := filepath.Join(project, "Library", "interaction.json")
	current, err := CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	if _, err := ApproveAndApplyAuthorized(project, current, draft, CurrentHashAbsent, "local-user", evidence, testApprovalVerifier{}, &testOneShotConsumer{}); err == nil || !strings.Contains(err.Error(), "SUPPORT_CONTRACT_STALE") {
		t.Fatalf("unbound interaction approval error = %v", err)
	}
	geometry := ApprovalGeometryBindings{
		SubjectGeometryHash: strings.Repeat("a", 64), TargetGeometryHash: strings.Repeat("b", 64),
		DependencyDestinations: []string{filepath.Join(project, "subject.spatial.json"), filepath.Join(project, "target.spatial.json")},
		RevalidateCurrent:      func() error { return nil },
	}
	consumer := &testOneShotConsumer{}
	if _, err := ApproveAndApplyAuthorizedWithGeometry(project, current, draft, CurrentHashAbsent, "local-user", evidence, geometry, testApprovalVerifier{}, consumer); err != nil {
		t.Fatalf("geometry-bound interaction approval failed: %v", err)
	}
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
	if applied, err := OverlayApprovedAssets(&manifest, root); err == nil || applied != 0 || !strings.Contains(err.Error(), "approval-ledger") {
		t.Fatalf("unverified OverlayApprovedAssets() applied=%d err=%v", applied, err)
	}
	applied, err := OverlayApprovedAssetsWithPolicy(&manifest, root, OverlayPolicy{Verifier: testApprovedContractVerifier{}})
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
			applied, err := OverlayApprovedAssetsWithPolicy(&candidate, root, OverlayPolicy{Verifier: testApprovedContractVerifier{}})
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

type testOneShotConsumer struct {
	used        map[string]bool
	beforeApply func()
}

func (consumer *testOneShotConsumer) ConsumeApprovalGrant(verification ApprovalVerification, apply func() error) error {
	if consumer.used == nil {
		consumer.used = map[string]bool{}
	}
	key := verification.Evidence.Authority + ":" + verification.Evidence.Nonce
	if consumer.used[key] {
		return errors.New("approval grant already consumed")
	}
	consumer.used[key] = true
	if consumer.beforeApply != nil {
		consumer.beforeApply()
	}
	return apply()
}

type testApprovedContractVerifier struct{}

func (testApprovedContractVerifier) VerifyApprovedContract(verification ApprovedContractVerification) error {
	if verification.ContractHash == "" || verification.CaptureSetHash == "" || verification.Reviewer == "" || verification.ContractPath == "" {
		return errors.New("approval record is incomplete")
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
