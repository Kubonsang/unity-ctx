package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/app"
	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/core"
)

func TestSceneCheckRoutesCompleteMultiContactMapping(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Scene = "Assets/Scenes/SimpleScene.unity"
	manifestPath := filepath.Join(t.TempDir(), "spatial.json")
	if err := bounds.Save(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	scenePath := stageSuggestScene(t)
	svc := app.New()
	args := app.CheckArgs{
		Manifest: manifestPath, Prefab: "Assets/Prefabs/Bookcase.prefab", HasPosition: true,
		Position: [3]float64{0, .005, 3.87}, HasRotation: true, Rotation: [4]float64{0, 1, 0, 0},
		ContactSurfaces: "wall=wall-north,floor=floor-main",
	}
	result, code := svc.Check("scene", scenePath, core.ViewCompact, false, args)
	if code != 0 || result.Status != "OK" || !strings.Contains(result.Body, "contact_surfaces=wall=wall-north,floor=floor-main") ||
		!strings.Contains(result.Body, "floor@floor-main[none]") || !strings.Contains(result.Body, "wall@wall-north[none]") {
		t.Fatalf("multi-contact check result=%#v code=%d", result, code)
	}

	args.ContactSurfaces = ""
	args.SurfaceID, args.Contact = "wall-north", "wall-backed"
	result, code = svc.Check("scene", scenePath, core.ViewCompact, false, args)
	if code != 1 || !strings.Contains(result.Body, "CONTACT_SURFACE_MAPPING_REQUIRED") {
		t.Fatalf("legacy multi-contact request was not rejected: result=%#v code=%d", result, code)
	}

	args.SurfaceID, args.Contact = "", ""
	result, code = svc.Check("scene", scenePath, core.ViewCompact, false, args)
	if code != 1 || !strings.Contains(result.Body, "CONTACT_SURFACE_MAPPING_REQUIRED") {
		t.Fatalf("contact-less multi-contact request was not rejected: result=%#v code=%d", result, code)
	}
}
