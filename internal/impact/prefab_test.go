package impact

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanPrefabImpactFindsSceneAndPrefabHits(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	got, err := ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("ScanPrefabImpact() error = %v", err)
	}

	if got.PrefabPath != "Assets/Prefabs/Enemy.prefab" {
		t.Fatalf("PrefabPath mismatch: got %q want %q", got.PrefabPath, "Assets/Prefabs/Enemy.prefab")
	}
	if got.PrefabGUID != "fake_enemy_guid" {
		t.Fatalf("PrefabGUID mismatch: got %q want %q", got.PrefabGUID, "fake_enemy_guid")
	}

	if len(got.SceneHits) != 2 {
		t.Fatalf("SceneHits length mismatch: got %d want %d", len(got.SceneHits), 2)
	}
	if got.SceneHits[0].Path != "Assets/Scenes/BossRoom.unity" {
		t.Fatalf("first scene mismatch: got %q want %q", got.SceneHits[0].Path, "Assets/Scenes/BossRoom.unity")
	}
	if got.SceneHits[1].Path != "Assets/Scenes/Stage01.unity" {
		t.Fatalf("second scene mismatch: got %q want %q", got.SceneHits[1].Path, "Assets/Scenes/Stage01.unity")
	}
	if !reflect.DeepEqual(got.SceneHits[1].FileIDs, []int64{1000, 2000}) {
		t.Fatalf("Stage01 fileIDs mismatch: got %#v want %#v", got.SceneHits[1].FileIDs, []int64{1000, 2000})
	}

	if len(got.PrefabHits) != 1 {
		t.Fatalf("PrefabHits length mismatch: got %d want %d", len(got.PrefabHits), 1)
	}
	if got.PrefabHits[0].Path != "Assets/Prefabs/EnemyElite.prefab" {
		t.Fatalf("first prefab mismatch: got %q want %q", got.PrefabHits[0].Path, "Assets/Prefabs/EnemyElite.prefab")
	}
	if !reflect.DeepEqual(got.PrefabHits[0].FileIDs, []int64{3000, 3001}) {
		t.Fatalf("EnemyElite fileIDs mismatch: got %#v want %#v", got.PrefabHits[0].FileIDs, []int64{3000, 3001})
	}
}

func TestScanPrefabImpactSkipsTargetPrefabHit(t *testing.T) {
	project := copyImpactProject(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	selfRef := "" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1001 &1101\n" +
		"PrefabInstance:\n" +
		"  m_SourcePrefab: {fileID: 100100000, guid: fake_enemy_guid, type: 3}\n"
	if err := os.WriteFile(target, append([]byte(selfRef), data...), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("ScanPrefabImpact() error = %v", err)
	}

	for _, hit := range got.PrefabHits {
		if hit.Path == "Assets/Prefabs/Enemy.prefab" {
			t.Fatalf("target prefab should be skipped from PrefabHits: got %#v", got.PrefabHits)
		}
	}
}

func TestLoadPrefabGUIDReadsGUIDFromMeta(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	got, err := LoadPrefabGUID(target)
	if err != nil {
		t.Fatalf("LoadPrefabGUID() error = %v", err)
	}

	if got != "fake_enemy_guid" {
		t.Fatalf("LoadPrefabGUID() = %q want %q", got, "fake_enemy_guid")
	}
}

func TestLoadPrefabGUIDRejectsMissingMeta(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadPrefabGUID(target)
	if err == nil {
		t.Fatal("LoadPrefabGUID() error = nil, want missing meta error")
	}

	want := "prefab meta not found file=" + filepath.Clean(target)
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestScanPrefabImpactRejectsMissingTargetPrefabFile(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Missing.prefab")

	_, err := ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err == nil {
		t.Fatal("ScanPrefabImpact() error = nil, want missing target prefab error")
	}

	absoluteTarget, absErr := filepath.Abs(target)
	if absErr != nil {
		t.Fatalf("Abs() error = %v", absErr)
	}
	want := "prefab file not found: " + filepath.Clean(absoluteTarget)
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestScanPrefabImpactPrefersAssetsValidationForOutOfProjectTarget(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(t.TempDir(), "Missing.prefab")

	_, err := ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err == nil {
		t.Fatal("ScanPrefabImpact() error = nil, want out-of-project target error")
	}

	absoluteProject, absErr := filepath.Abs(project)
	if absErr != nil {
		t.Fatalf("Abs() error = %v", absErr)
	}
	want := "prefab must be under project Assets/ file=" + filepath.Clean(target) + " project=" + filepath.Clean(absoluteProject)
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestScanPrefabImpactRespectsSceneScope(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	absoluteScene := filepath.Join(project, "Assets", "Scenes", "BossRoom.unity")

	for _, tc := range []struct {
		name  string
		scope []string
	}{
		{name: "absolute", scope: []string{absoluteScene}},
		{name: "assets", scope: []string{" Assets/Scenes/BossRoom.unity "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ScanPrefabImpact(Request{
				ProjectPath: project,
				TargetPath:  target,
				SceneScope:  tc.scope,
				MaxDepth:    3,
			})
			if err != nil {
				t.Fatalf("ScanPrefabImpact() error = %v", err)
			}

			if len(got.SceneHits) != 1 || got.SceneHits[0].Path != "Assets/Scenes/BossRoom.unity" {
				t.Fatalf("SceneHits mismatch: got %#v", got.SceneHits)
			}
			if len(got.PrefabHits) != 1 || got.PrefabHits[0].Path != "Assets/Prefabs/EnemyElite.prefab" {
				t.Fatalf("PrefabHits mismatch: got %#v", got.PrefabHits)
			}
		})
	}
}

func TestScanPrefabImpactWarnsOnDepthLimit(t *testing.T) {
	project := copyImpactProject(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writeTestPrefab(t, filepath.Join(project, "Assets", "Prefabs", "EnemyElite.prefab"), "fake_enemy_elite_guid", "fake_enemy_guid")
	writeTestPrefab(t, filepath.Join(project, "Assets", "Prefabs", "EnemyBoss.prefab"), "fake_enemy_boss_guid", "fake_enemy_elite_guid")
	writeTestPrefab(t, filepath.Join(project, "Assets", "Prefabs", "EnemyUltra.prefab"), "fake_enemy_ultra_guid", "fake_enemy_boss_guid")

	got, err := ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("ScanPrefabImpact() error = %v", err)
	}

	if got.Status != "OK" {
		t.Fatalf("Status mismatch before overflow: got %q want %q", got.Status, "OK")
	}
	if got.DepthLimitHit {
		t.Fatal("DepthLimitHit mismatch before overflow: got true want false")
	}
	if got.MaxNestedDepth != 3 {
		t.Fatalf("MaxNestedDepth mismatch before overflow: got %d want %d", got.MaxNestedDepth, 3)
	}

	writeTestPrefab(t, filepath.Join(project, "Assets", "Prefabs", "EnemyLegend.prefab"), "fake_enemy_legend_guid", "fake_enemy_ultra_guid")

	got, err = ScanPrefabImpact(Request{
		ProjectPath: project,
		TargetPath:  target,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("ScanPrefabImpact() error after overflow = %v", err)
	}

	if got.Status != "WARN" {
		t.Fatalf("Status mismatch after overflow: got %q want %q", got.Status, "WARN")
	}
	if !got.DepthLimitHit {
		t.Fatal("DepthLimitHit mismatch after overflow: got false want true")
	}
	if got.MaxNestedDepth != 3 {
		t.Fatalf("MaxNestedDepth mismatch after overflow: got %d want %d", got.MaxNestedDepth, 3)
	}
}

func copyImpactProject(t *testing.T) string {
	t.Helper()

	source := filepath.Join("..", "..", "testdata", "impact", "project")
	dest := filepath.Join(t.TempDir(), "project")
	if err := copyTree(source, dest); err != nil {
		t.Fatalf("copyTree() error = %v", err)
	}
	return dest
}

func writeTestPrefab(t *testing.T, prefabPath, guid, referencedGUID string) {
	t.Helper()

	prefab := "" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1001 &3000\n" +
		"PrefabInstance:\n" +
		"  m_SourcePrefab: {fileID: 100100000, guid: " + referencedGUID + ", type: 3}\n"
	if err := os.WriteFile(prefabPath, []byte(prefab), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", prefabPath, err)
	}

	meta := "" +
		"fileFormatVersion: 2\n" +
		"guid: " + guid + "\n" +
		"PrefabImporter:\n" +
		"  externalObjects: {}\n"
	if err := os.WriteFile(prefabPath+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(%s.meta) error = %v", prefabPath, err)
	}
}

func copyTree(source, dest string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, relative)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
