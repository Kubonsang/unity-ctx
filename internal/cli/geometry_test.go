package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

func TestSceneCheckContactSurfaceMappingCLIContract(t *testing.T) {
	base := []string{"scene", "check", "missing.unity", "--manifest", "missing.json", "--prefab", "Assets/Bookcase.prefab", "--position", "0,0,0"}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	args := append(append([]string(nil), base...), "--contact-surfaces", "floor=floor-main,wall=wall-north")
	if code := Run(args, stdout, stderr); code != 1 || strings.Contains(stdout.String()+stderr.String(), "invalid contact surface mapping") {
		t.Fatalf("valid mapping was rejected by CLI parsing: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args = append(append([]string(nil), base...), "--contact-surfaces", "wall-north")
	if code := Run(args, stdout, stderr); code != 2 || !strings.Contains(stderr.String(), "expected requirement-id=surface-id") {
		t.Fatalf("malformed mapping result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args = append(append([]string(nil), base...), "--surface-id", "wall-north", "--contact", "wall-backed", "--contact-surfaces", "wall=wall-north")
	if code := Run(args, stdout, stderr); code != 2 || !strings.Contains(stderr.String(), "either --contact-surfaces or --surface-id/--contact") {
		t.Fatalf("mixed contact forms result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSceneCheckCLIRejectsOmittedReviewedContacts(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Scene = "Assets/Scenes/SimpleScene.unity"
	manifestPath := filepath.Join(t.TempDir(), "spatial.json")
	if err := bounds.Save(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	sceneData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	if err != nil {
		t.Fatal(err)
	}
	scenePath := filepath.Join(t.TempDir(), "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scenePath, sceneData, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := Run([]string{
		"scene", "check", scenePath,
		"--manifest", manifestPath,
		"--prefab", "Assets/Prefabs/Bookcase.prefab",
		"--position", "0,0.005,3.87",
		"--rotation", "0,1,0,0",
	}, stdout, stderr)
	if code != 1 || !strings.Contains(stdout.String(), "CONTACT_SURFACE_MAPPING_REQUIRED") || stderr.Len() != 0 {
		t.Fatalf("contact-less CLI check result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
