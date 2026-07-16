package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestSpatialCLIKeepsApprovalAndApplyWriteBehindBridge(t *testing.T) {
	directory := t.TempDir()
	draft := filepath.Join(directory, "draft.spatial.json")
	current := filepath.Join(directory, "Assets", "SpatialContracts", "Assets", "0123456789abcdef0123456789abcdef.spatial.json")
	contract := validSpatialCLIContract()
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runSpatial([]string{"validate", "--json", draft}, stdout, stderr); code != 0 {
		t.Fatalf("validate code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"state":"AwaitingHumanReview"`) {
		t.Fatalf("unexpected validate output %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runSpatial([]string{"review", "--draft", draft, "--decision", "Approved", "--reviewer", "student-01", "--write", "--json"}, stdout, stderr); code != 1 {
		t.Fatalf("review code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "local review bridge") {
		t.Fatalf("unexpected review error %s", stderr.String())
	}
	reviewed, err := spatialcontract.Load(draft)
	if err != nil || reviewed.State != spatialcontract.StateAwaitingHumanReview || reviewed.Review != nil {
		t.Fatalf("unauthorized review changed draft=%+v err=%v", reviewed, err)
	}
	evidence := spatialcontract.ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	if err := spatialcontract.ReviewAuthorized(&reviewed, "student-01", evidence, cliApprovalVerifier{}); err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(draft, reviewed); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runSpatial([]string{"diff", "--current", current, "--draft", draft, "--json"}, stdout, stderr); code != 0 || !strings.Contains(stdout.String(), `"status":"NEW"`) {
		t.Fatalf("diff code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runSpatial([]string{"apply", "--current", current, "--draft", draft, "--json"}, stdout, stderr); code != 0 {
		t.Fatalf("dry-run code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("dry-run unexpectedly wrote current: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runSpatial([]string{"apply", "--current", current, "--draft", draft, "--write", "--json"}, stdout, stderr); code != 1 {
		t.Fatalf("apply code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "authorized local review bridge") {
		t.Fatalf("unexpected apply error %s", stderr.String())
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("public apply unexpectedly wrote current: %v", err)
	}
}

func TestSpatialCLIRejectsApprovalWithTechnicalErrors(t *testing.T) {
	draft := filepath.Join(t.TempDir(), "failed.spatial.json")
	contract := validSpatialCLIContract()
	contract.State = spatialcontract.StateTechnicalFailed
	contract.Technical = &spatialcontract.TechnicalEvidence{Passed: false, ErrorCount: 1, ReportHash: "report-failed"}
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runSpatial([]string{"review", "--draft", draft, "--decision", "Approved", "--reviewer", "student-01", "--write"}, stdout, stderr); code != 1 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "local review bridge") {
		t.Fatalf("unexpected stderr %s", stderr.String())
	}
}

func TestSpatialValidateAcceptsJSONBeforeOrAfterFile(t *testing.T) {
	draft := filepath.Join(t.TempDir(), "draft.spatial.json")
	if err := spatialcontract.Save(draft, validSpatialCLIContract()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"validate", "--json", draft},
		{"validate", draft, "--json"},
	} {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if code := runSpatial(args, stdout, stderr); code != 0 {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), `"state":"AwaitingHumanReview"`) || !strings.Contains(stdout.String(), `"proposal_hash":"`) || stderr.Len() != 0 {
			t.Fatalf("args=%v stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestSpatialDiffInteractionRemainsOfflineAndQueryOnly(t *testing.T) {
	project := t.TempDir()
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion, ContractType: spatialcontract.TypeInteraction,
		State: spatialcontract.StateAwaitingHumanReview,
		Interaction: &spatialcontract.InteractionContract{
			SubjectGUID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetKey: "asset:cccccccccccccccccccccccccccccccc", Relation: "SupportedBy",
			SubjectFrame: "bottom", TargetFrame: "top", RelativeRotation: spatialcontract.Quat{0, 0, 0, 1},
			PositionTolerance: spatialcontract.Vec3{.1, .01, .1}, AngleTolerance: 10, CollisionPolicy: "contact-only",
			Revision: 1, CaptureSetHash: "capture-interaction",
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-interaction"},
	}
	spatialcontract.Normalize(&contract)
	draft := filepath.Join(project, "Library", "interaction.json")
	current, err := spatialcontract.CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runSpatial([]string{"diff", "--current", current, "--draft", draft, "--json"}, stdout, stderr); code != 0 {
		t.Fatalf("offline interaction diff code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"contract_type":"interaction"`) || !strings.Contains(stdout.String(), `"status":"NEW"`) {
		t.Fatalf("unexpected interaction diff output %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "subject_geometry_hash") || strings.Contains(stdout.String(), "target_geometry_hash") {
		t.Fatalf("generic diff fabricated external provenance: %s", stdout.String())
	}
}

func TestSpatialCommandSpecificHelp(t *testing.T) {
	tests := []struct {
		command string
		prefix  string
	}{
		{"validate", "unity-ctx spatial validate <file> [--json]"},
		{"verify-approved", "unity-ctx spatial verify-approved <file> [--json]"},
		{"diff", "unity-ctx spatial diff --current FILE --draft FILE [--json]"},
		{"review", "unity-ctx spatial review --draft FILE"},
		{"apply", "unity-ctx spatial apply --current FILE --draft FILE [--json]"},
	}
	for _, test := range tests {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if code := Run([]string{"spatial", test.command, "--help"}, stdout, stderr); code != 0 {
			t.Fatalf("command=%s code=%d stderr=%s", test.command, code, stderr.String())
		}
		if !strings.HasPrefix(stdout.String(), test.prefix) || stderr.Len() != 0 {
			t.Fatalf("command=%s stdout=%q stderr=%q", test.command, stdout.String(), stderr.String())
		}
	}
}

func TestSpatialVerifyApprovedRejectsSelfConsistentTrackedJSONWithoutLedger(t *testing.T) {
	contract := validSpatialCLIContract()
	if err := spatialcontract.ReviewAuthorized(&contract, "student-01", spatialcontract.ApprovalEvidence{
		Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof",
	}, cliApprovalVerifier{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), contract.Asset.AssetGUID+".spatial.json")
	if err := spatialcontract.Save(path, contract); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runSpatial([]string{"verify-approved", path, "--json"}, stdout, stderr); code != 1 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "not authorized for consumption") {
		t.Fatalf("unexpected verify-approved error %s", stderr.String())
	}
}

func TestSpatialCLIRecordsNonApprovalReviewDecision(t *testing.T) {
	draft := filepath.Join(t.TempDir(), "revision.spatial.json")
	if err := spatialcontract.Save(draft, validSpatialCLIContract()); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runSpatial([]string{"review", "--draft", draft, "--decision", "RevisionRequested", "--reviewer", "student-01", "--issues", "contact-gap", "--comment", "Move closer", "--write", "--json"}, stdout, stderr); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	reviewed, err := spatialcontract.Load(draft)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.State != spatialcontract.StateRevisionRequested || reviewed.Review == nil || reviewed.Review.Decision != spatialcontract.StateRevisionRequested {
		t.Fatalf("reviewed=%+v", reviewed)
	}
}

type cliApprovalVerifier struct{}

func (cliApprovalVerifier) VerifyApproval(verification spatialcontract.ApprovalVerification) error {
	if verification.Evidence.Proof != "valid-proof" {
		return errors.New("invalid proof")
	}
	return nil
}

func validSpatialCLIContract() spatialcontract.Contract {
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion,
		ContractType:    spatialcontract.TypeAsset,
		State:           spatialcontract.StateAwaitingHumanReview,
		Asset: &spatialcontract.AssetSpatialContract{
			AssetGUID:      "0123456789abcdef0123456789abcdef",
			AssetPath:      "Assets/Props/Banner.prefab",
			DependencyHash: "dependency-v1",
			Units:          "meter",
			Forward:        spatialcontract.Vec3{0, 0, 1},
			Up:             spatialcontract.Vec3{0, 1, 0},
			CollisionProxies: []spatialcontract.OBB{{
				ID: "body", Center: spatialcontract.Vec3{0, 0.5, 0}, Size: spatialcontract.Vec3{1, 1, 0.1}, Rotation: spatialcontract.Quat{0, 0, 0, 1},
			}},
			Frames: []spatialcontract.ContactFrame{{
				ID: "back", Point: spatialcontract.Vec3{0, 0.5, -0.05}, Normal: spatialcontract.Vec3{0, 0, -1}, Tangent: spatialcontract.Vec3{1, 0, 0}, Size: [2]float64{1, 1},
			}},
			Contacts: []spatialcontract.ContactRequirement{{
				ID: "wall", Kind: "WallMounted", FrameID: "back", Target: "surface:wall", MinimumGap: 0.005, MaximumGap: 0.01, MinimumSupport: 0.6, DirectionAlignment: 0.95,
			}},
			Revision: 1, CaptureSetHash: "capture-v1",
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-v1"},
	}
	spatialcontract.Normalize(&contract)
	return contract
}
