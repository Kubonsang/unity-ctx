package reviewgrant

import (
	"crypto/ed25519"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestInteractionReceiptBindsLedgerAuthorizedDependencyGeometry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	project := t.TempDir()
	subjectGUID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	targetGUID := "cccccccccccccccccccccccccccccccc"

	subjectV1, subjectPath := approvedAssetFixture(t, project, subjectGUID, "capture-subject-v1", 1)
	target, targetPath := approvedAssetFixture(t, project, targetGUID, "capture-target-v1", 2)
	recordApprovedFixture(t, ledger, privateKey, now, subjectV1, subjectPath, "11111111111111111111111111111111", spatialcontract.ApprovalGeometryBindings{})
	recordApprovedFixture(t, ledger, privateKey, now, target, targetPath, "22222222222222222222222222222222", spatialcontract.ApprovalGeometryBindings{})

	interaction, interactionPath := approvedInteractionFixture(t, project, subjectGUID, targetGUID)
	provenanceV1, err := ledger.ResolveInteractionGeometry(project, interaction)
	if err != nil {
		t.Fatalf("ResolveInteractionGeometry() error = %v", err)
	}
	if provenanceV1.SubjectGeometryHash != subjectV1.Asset.GeometryHash || provenanceV1.TargetGeometryHash != target.Asset.GeometryHash {
		t.Fatalf("resolved geometry provenance = %+v", provenanceV1)
	}
	recordApprovedFixture(t, ledger, privateKey, now, interaction, interactionPath, "33333333333333333333333333333333", provenanceV1.ApprovalBindings())
	verification, err := spatialcontract.ApprovedVerification(interaction, interactionPath)
	if err != nil {
		t.Fatal(err)
	}
	verification.SubjectGeometryHash = provenanceV1.SubjectGeometryHash
	verification.TargetGeometryHash = provenanceV1.TargetGeometryHash
	if _, err := ledger.VerifyApprovedContractReceipt(verification); err != nil {
		t.Fatalf("fresh interaction receipt was rejected: %v", err)
	}

	subjectV2, _ := approvedAssetFixture(t, project, subjectGUID, "capture-subject-v2", 1.5)
	recordApprovedFixture(t, ledger, privateKey, now, subjectV2, subjectPath, "44444444444444444444444444444444", spatialcontract.ApprovalGeometryBindings{})
	provenanceV2, err := ledger.ResolveInteractionGeometry(project, interaction)
	if err != nil {
		t.Fatalf("updated dependency could not be resolved: %v", err)
	}
	if provenanceV2.SubjectGeometryHash == provenanceV1.SubjectGeometryHash {
		t.Fatal("fixture did not change the subject geometry hash")
	}
	verification.SubjectGeometryHash = provenanceV2.SubjectGeometryHash
	verification.TargetGeometryHash = provenanceV2.TargetGeometryHash
	if _, err := ledger.VerifyApprovedContractReceipt(verification); err == nil || !strings.Contains(err.Error(), "SUPPORT_CONTRACT_STALE") {
		t.Fatalf("stale interaction receipt error = %v", err)
	}
}

func TestResolveInteractionGeometryRejectsNonAssetTarget(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, _ := signedTestLedger(t, now)
	contract := spatialcontract.Contract{
		ContractType: spatialcontract.TypeInteraction,
		Interaction: &spatialcontract.InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "surface:table", Relation: "SupportedBy",
		},
	}
	if _, err := ledger.ResolveInteractionGeometry(t.TempDir(), contract); err == nil || !strings.Contains(err.Error(), "asset:<guid>") {
		t.Fatalf("non-asset interaction target error = %v", err)
	}
}

func approvedAssetFixture(t *testing.T, project, guid, capture string, width float64) (spatialcontract.Contract, string) {
	t.Helper()
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion,
		ContractType:    spatialcontract.TypeAsset,
		State:           spatialcontract.StateAwaitingHumanReview,
		Asset: &spatialcontract.AssetSpatialContract{
			AssetGUID: guid, AssetPath: "Assets/Fixtures/" + guid + ".prefab", DependencyHash: "dependency-" + capture,
			Units: "meter", Forward: spatialcontract.Vec3{0, 0, 1}, Up: spatialcontract.Vec3{0, 1, 0},
			CollisionProxies: []spatialcontract.OBB{{ID: "body", Size: spatialcontract.Vec3{width, 1, 1}, Rotation: spatialcontract.Quat{0, 0, 0, 1}}},
			Frames:           []spatialcontract.ContactFrame{{ID: "top", Normal: spatialcontract.Vec3{0, 1, 0}, Tangent: spatialcontract.Vec3{1, 0, 0}, Size: [2]float64{width, 1}}},
			Contacts:         []spatialcontract.ContactRequirement{{ID: "support", Kind: "FloorSupported", FrameID: "top", Target: "surface:floor", MaximumGap: .01, MinimumSupport: .6, DirectionAlignment: .95}},
			Revision:         1, CaptureSetHash: capture,
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-" + capture},
	}
	spatialcontract.Normalize(&contract)
	approveFixtureForTest(t, &contract)
	path, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(path, contract); err != nil {
		t.Fatal(err)
	}
	return contract, path
}

func approvedInteractionFixture(t *testing.T, project, subjectGUID, targetGUID string) (spatialcontract.Contract, string) {
	t.Helper()
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion,
		ContractType:    spatialcontract.TypeInteraction,
		State:           spatialcontract.StateAwaitingHumanReview,
		Interaction: &spatialcontract.InteractionContract{
			SubjectGUID: subjectGUID, TargetKey: "asset:" + targetGUID, Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: spatialcontract.Quat{0, 0, 0, 1},
			PositionTolerance: spatialcontract.Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-interaction-v1",
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-interaction-v1"},
	}
	spatialcontract.Normalize(&contract)
	approveFixtureForTest(t, &contract)
	path, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(path, contract); err != nil {
		t.Fatal(err)
	}
	return contract, path
}

func approveFixtureForTest(t *testing.T, contract *spatialcontract.Contract) {
	t.Helper()
	if err := spatialcontract.ReviewAuthorized(contract, "local-user", spatialcontract.ApprovalEvidence{
		Authority: "fixture", Nonce: "fixture-nonce", Proof: "fixture-proof",
	}, fixtureReviewVerifier{}); err != nil {
		t.Fatal(err)
	}
}

func recordApprovedFixture(t *testing.T, ledger *Ledger, privateKey ed25519.PrivateKey, now time.Time, contract spatialcontract.Contract, path, nonce string, geometry spatialcontract.ApprovalGeometryBindings) {
	t.Helper()
	verification := spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: spatialcontract.ContentHash(contract), CurrentHash: spatialcontract.CurrentHashAbsent,
		CaptureSetHash: contract.Review.CaptureSetHash, Reviewer: contract.Review.Reviewer, Destination: filepath.Clean(path),
		SubjectGeometryHash: geometry.SubjectGeometryHash, TargetGeometryHash: geometry.TargetGeometryHash,
		DependencyDestinations: append([]string(nil), geometry.DependencyDestinations...),
		Evidence:               spatialcontract.ApprovalEvidence{Authority: "local-review", Nonce: nonce, ExpiresUnix: now.Add(5 * time.Minute).Unix()},
	}
	verification.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(verification)))
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type fixtureReviewVerifier struct{}

func (fixtureReviewVerifier) VerifyApproval(spatialcontract.ApprovalVerification) error { return nil }
