package reviewbridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/reviewgrant"
	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestRunAtomicallyApprovesAndAppliesWithoutPersistingGrant(t *testing.T) {
	project := t.TempDir()
	draft := filepath.Join(project, "Library", "SpatialDrafts", "banner.json")
	contract := awaitingContract()
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	current, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		ProtocolVersion: ProtocolVersion, Action: spatialcontract.ApprovalActionApproveApply,
		ProjectRoot: project, CurrentPath: current, CurrentHash: spatialcontract.CurrentHashAbsent, DraftPath: draft, Reviewer: "local-user",
		Grant: spatialcontract.ApprovalEvidence{Authority: "test", Nonce: "0123456789abcdef0123456789abcdef", Proof: "valid"},
	}
	input, _ := json.Marshal(request)
	consumer := &oneShotConsumer{}
	var output, errorOutput bytes.Buffer
	code := Run(bytes.NewReader(input), &output, &errorOutput, Config{Verifier: bridgeVerifier{}, Consumer: consumer})
	if code != 0 {
		t.Fatalf("Run() code=%d output=%s stderr=%s", code, output.String(), errorOutput.String())
	}
	loaded, err := spatialcontract.Load(current)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != spatialcontract.StateApproved || loaded.Review == nil || loaded.Review.Authorization != nil {
		t.Fatalf("persisted approval = %#v", loaded.Review)
	}
	diff, err := spatialcontract.Diff(current, draft)
	if err != nil {
		t.Fatal(err)
	}
	request.CurrentHash = diff.CurrentHash
	input, _ = json.Marshal(request)
	output.Reset()
	errorOutput.Reset()
	if code := Run(bytes.NewReader(input), &output, &errorOutput, Config{Verifier: bridgeVerifier{}, Consumer: consumer}); code != 1 || !strings.Contains(output.String(), "already consumed") {
		t.Fatalf("replay code=%d output=%s stderr=%s", code, output.String(), errorOutput.String())
	}
}

func TestRunReportsCommittedContractAndBackupWhenReceiptFails(t *testing.T) {
	project := t.TempDir()
	contract := awaitingContract()
	draft := filepath.Join(project, "Library", "SpatialDrafts", "banner.json")
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	current, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(current, contract); err != nil {
		t.Fatal(err)
	}
	diff, err := spatialcontract.Diff(current, draft)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		ProtocolVersion: ProtocolVersion, Action: spatialcontract.ApprovalActionApproveApply,
		ProjectRoot: project, CurrentPath: current, CurrentHash: diff.CurrentHash, DraftPath: draft, Reviewer: "local-user",
		Grant: spatialcontract.ApprovalEvidence{Authority: "test", Nonce: "0123456789abcdef0123456789abcdef", Proof: "valid"},
	}
	input, _ := json.Marshal(request)
	var output, errorOutput bytes.Buffer
	code := Run(bytes.NewReader(input), &output, &errorOutput, Config{Verifier: bridgeVerifier{}, Consumer: failingAfterApplyConsumer{}})
	var response Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if code != 1 || response.OK || !response.Written || response.Status != "WRITE_COMMITTED_UNRECEIPTED" || response.CurrentPath != current || response.Backup == "" || !strings.Contains(response.Error, "receipt sync failed") {
		t.Fatalf("committed failure response=%+v code=%d stderr=%s", response, code, errorOutput.String())
	}
	if _, err := os.Stat(response.Backup); err != nil {
		t.Fatalf("reported backup is missing: %v", err)
	}
	loaded, err := spatialcontract.Load(current)
	if err != nil || loaded.State != spatialcontract.StateApproved {
		t.Fatalf("committed contract state=%q err=%v", loaded.State, err)
	}
}

func TestRunRejectsWrongActionAndNonCanonicalDestination(t *testing.T) {
	project := t.TempDir()
	draft := filepath.Join(project, "Library", "draft.json")
	contract := awaitingContract()
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	canonical, _ := spatialcontract.CanonicalContractPath(project, contract)
	base := Request{
		ProtocolVersion: ProtocolVersion, Action: spatialcontract.ApprovalActionApproveApply,
		ProjectRoot: project, CurrentPath: canonical, CurrentHash: spatialcontract.CurrentHashAbsent, DraftPath: draft, Reviewer: "local-user",
		Grant: spatialcontract.ApprovalEvidence{Authority: "test", Nonce: "0123456789abcdef0123456789abcdef", Proof: "valid"},
	}
	for name, mutate := range map[string]func(*Request){
		"action": func(value *Request) { value.Action = "apply" },
		"current hash": func(value *Request) {
			value.CurrentHash = ""
		},
		"destination": func(value *Request) {
			value.CurrentPath = filepath.Join(project, "Assets", "SpatialContracts", "Assets", "other.spatial.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			mutate(&request)
			data, _ := json.Marshal(request)
			var output, errorOutput bytes.Buffer
			code := Run(bytes.NewReader(data), &output, &errorOutput, Config{Verifier: bridgeVerifier{}, Consumer: &oneShotConsumer{}})
			if code == 0 {
				t.Fatalf("mutated request succeeded: %s", output.String())
			}
		})
	}
}

func TestRunDerivesInteractionGeometryFromLedgerAuthorizedAssets(t *testing.T) {
	project := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(authorityRoot, "local-review.pub"), []byte(base64.RawURLEncoding.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := &reviewgrant.Ledger{Root: t.TempDir(), AuthorityRoot: authorityRoot, Now: func() time.Time { return now }}
	subjectGUID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	targetGUID := "cccccccccccccccccccccccccccccccc"
	recordBridgeAssetApproval(t, project, ledger, privateKey, now, subjectGUID, "subject", "11111111111111111111111111111111")
	recordBridgeAssetApproval(t, project, ledger, privateKey, now, targetGUID, "target", "22222222222222222222222222222222")

	interaction := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion, ContractType: spatialcontract.TypeInteraction,
		State: spatialcontract.StateAwaitingHumanReview,
		Interaction: &spatialcontract.InteractionContract{
			SubjectGUID: subjectGUID, TargetKey: "asset:" + targetGUID, Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: spatialcontract.Quat{0, 0, 0, 1},
			PositionTolerance: spatialcontract.Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-interaction",
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-interaction"},
	}
	spatialcontract.Normalize(&interaction)
	draft := filepath.Join(project, "Library", "SpatialDrafts", "interaction.json")
	if err := spatialcontract.Save(draft, interaction); err != nil {
		t.Fatal(err)
	}
	current, err := spatialcontract.CanonicalContractPath(project, interaction)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := ledger.ResolveInteractionGeometry(project, interaction)
	if err != nil {
		t.Fatal(err)
	}
	grant := spatialcontract.ApprovalEvidence{Authority: "local-review", Nonce: "33333333333333333333333333333333", ExpiresUnix: now.Add(5 * time.Minute).Unix()}
	binding := spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: spatialcontract.ContentHash(interaction), CurrentHash: spatialcontract.CurrentHashAbsent,
		CaptureSetHash: interaction.Interaction.CaptureSetHash, Reviewer: "local-user", Destination: current,
		SubjectGeometryHash: provenance.SubjectGeometryHash, TargetGeometryHash: provenance.TargetGeometryHash, Evidence: grant,
	}
	grant.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, reviewgrant.SigningPayload(binding)))
	request := Request{
		ProtocolVersion: ProtocolVersion, Action: spatialcontract.ApprovalActionApproveApply, ProjectRoot: project,
		CurrentPath: current, CurrentHash: spatialcontract.CurrentHashAbsent, DraftPath: draft, Reviewer: "local-user", Grant: grant,
	}
	input, _ := json.Marshal(request)
	var output, errorOutput bytes.Buffer
	config := Config{Verifier: reviewgrant.Verifier{AuthorityRoot: authorityRoot, Now: func() time.Time { return now }}, Consumer: ledger, Ledger: ledger}
	if code := Run(bytes.NewReader(input), &output, &errorOutput, config); code != 0 {
		t.Fatalf("interaction bridge code=%d output=%s stderr=%s", code, output.String(), errorOutput.String())
	}
	approved, err := spatialcontract.Load(current)
	if err != nil || approved.State != spatialcontract.StateApproved {
		t.Fatalf("approved interaction state=%s err=%v", approved.State, err)
	}
}

func awaitingContract() spatialcontract.Contract {
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion,
		ContractType:    spatialcontract.TypeAsset,
		State:           spatialcontract.StateAwaitingHumanReview,
		Asset: &spatialcontract.AssetSpatialContract{
			AssetGUID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AssetPath: "Assets/Props/banner.prefab",
			DependencyHash: "dependency", Units: "meter", Forward: spatialcontract.Vec3{0, 0, 1}, Up: spatialcontract.Vec3{0, 1, 0},
			CollisionProxies: []spatialcontract.OBB{{ID: "body", Center: spatialcontract.Vec3{0, 1, 0}, Size: spatialcontract.Vec3{1, 2, .05}, Rotation: spatialcontract.Quat{0, 0, 0, 1}}},
			Frames:           []spatialcontract.ContactFrame{{ID: "back", Point: spatialcontract.Vec3{0, 1, -.025}, Normal: spatialcontract.Vec3{0, 0, -1}, Tangent: spatialcontract.Vec3{1, 0, 0}, Size: [2]float64{1, 2}}},
			Contacts:         []spatialcontract.ContactRequirement{{ID: "wall", Kind: "WallMounted", FrameID: "back", Target: "surface:wall", MinimumGap: .005, MaximumGap: .01, MinimumSupport: .6, DirectionAlignment: .95}},
			Revision:         1, CaptureSetHash: "capture",
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report"},
	}
	spatialcontract.Normalize(&contract)
	return contract
}

type bridgeVerifier struct{}

func (bridgeVerifier) VerifyApproval(value spatialcontract.ApprovalVerification) error {
	if value.Action != spatialcontract.ApprovalActionApproveApply || value.ContractHash == "" || value.CurrentHash == "" || value.CaptureSetHash == "" || value.Reviewer == "" || !filepath.IsAbs(value.Destination) {
		return errors.New("invalid binding")
	}
	return nil
}

type oneShotConsumer struct{ used bool }

func (consumer *oneShotConsumer) ConsumeApprovalGrant(value spatialcontract.ApprovalVerification, apply func() error) error {
	if consumer.used {
		return errors.New("approval grant already consumed")
	}
	consumer.used = true
	return apply()
}

type failingAfterApplyConsumer struct{}

func (failingAfterApplyConsumer) ConsumeApprovalGrant(_ spatialcontract.ApprovalVerification, apply func() error) error {
	if err := apply(); err != nil {
		return err
	}
	return errors.New("receipt sync failed")
}

func recordBridgeAssetApproval(t *testing.T, project string, ledger *reviewgrant.Ledger, privateKey ed25519.PrivateKey, now time.Time, guid, label, nonce string) {
	t.Helper()
	contract := awaitingContract()
	contract.Asset.AssetGUID = guid
	contract.Asset.AssetPath = "Assets/Fixtures/" + label + ".prefab"
	contract.Asset.DependencyHash = "dependency-" + label
	contract.Asset.CaptureSetHash = "capture-" + label
	contract.Asset.GeometryHash = ""
	contract.Technical.ReportHash = "report-" + label
	spatialcontract.Normalize(&contract)
	if err := spatialcontract.ReviewAuthorized(&contract, "local-user", spatialcontract.ApprovalEvidence{
		Authority: "fixture", Nonce: "fixture-nonce", Proof: "fixture-proof",
	}, permissiveReviewVerifier{}); err != nil {
		t.Fatal(err)
	}
	path, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(path, contract); err != nil {
		t.Fatal(err)
	}
	verification := spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: spatialcontract.ContentHash(contract), CurrentHash: spatialcontract.CurrentHashAbsent,
		CaptureSetHash: contract.Asset.CaptureSetHash, Reviewer: contract.Review.Reviewer, Destination: path,
		Evidence: spatialcontract.ApprovalEvidence{Authority: "local-review", Nonce: nonce, ExpiresUnix: now.Add(5 * time.Minute).Unix()},
	}
	verification.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, reviewgrant.SigningPayload(verification)))
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type permissiveReviewVerifier struct{}

func (permissiveReviewVerifier) VerifyApproval(spatialcontract.ApprovalVerification) error {
	return nil
}

func TestRunRejectsDraftSymlink(t *testing.T) {
	project := t.TempDir()
	outside := filepath.Join(t.TempDir(), "draft.json")
	if err := spatialcontract.Save(outside, awaitingContract()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "Library"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(project, "Library", "draft.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	contract := awaitingContract()
	current, _ := spatialcontract.CanonicalContractPath(project, contract)
	request := Request{ProtocolVersion: ProtocolVersion, Action: spatialcontract.ApprovalActionApproveApply, ProjectRoot: project, CurrentPath: current, CurrentHash: spatialcontract.CurrentHashAbsent, DraftPath: link, Reviewer: "local-user", Grant: spatialcontract.ApprovalEvidence{Authority: "test", Nonce: "0123456789abcdef0123456789abcdef", Proof: "valid"}}
	data, _ := json.Marshal(request)
	var output, errorOutput bytes.Buffer
	if code := Run(bytes.NewReader(data), &output, &errorOutput, Config{Verifier: bridgeVerifier{}, Consumer: &oneShotConsumer{}}); code != 2 || !strings.Contains(output.String(), "symlink") {
		t.Fatalf("symlink draft code=%d output=%s", code, output.String())
	}
}
