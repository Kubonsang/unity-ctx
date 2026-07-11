package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestSpatialCLIHumanApprovalLifecycle(t *testing.T) {
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
	if code := runSpatial([]string{"review", "--draft", draft, "--decision", "Approved", "--reviewer", "student-01", "--write", "--json"}, stdout, stderr); code != 0 {
		t.Fatalf("review code=%d stderr=%s", code, stderr.String())
	}
	reviewed, err := spatialcontract.Load(draft)
	if err != nil || reviewed.State != spatialcontract.StateApproved {
		t.Fatalf("reviewed=%+v err=%v", reviewed, err)
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
	if code := runSpatial([]string{"apply", "--current", current, "--draft", draft, "--write", "--json"}, stdout, stderr); code != 0 {
		t.Fatalf("apply code=%d stderr=%s", code, stderr.String())
	}
	if _, err := spatialcontract.Load(current); err != nil {
		t.Fatalf("written contract did not validate: %v", err)
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
	if !strings.Contains(stderr.String(), "technical validation must pass") {
		t.Fatalf("unexpected stderr %s", stderr.String())
	}
}

func validSpatialCLIContract() spatialcontract.Contract {
	return spatialcontract.Contract{
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
}
