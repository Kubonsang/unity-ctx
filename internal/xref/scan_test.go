package xref

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	guidA = "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	guidB = "b2c3d4e5f60718293a4b5c6d7e8f90a1"
	guidC = "c3d4e5f60718293a4b5c6d7e8f90a1b2"
	guidD = "d4e5f60718293a4b5c6d7e8f90a1b2c3"
)

// writeAsset writes Assets/<name> + its .meta guid under projectRoot.
func writeAsset(t *testing.T, projectRoot, name, guid, body string) {
	t.Helper()
	path := filepath.Join(projectRoot, "Assets", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.WriteFile(path+".meta", []byte("guid: "+guid+"\n"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

func targetScene() string {
	return "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: Root\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children:\n  - {fileID: 4001}\n  - {fileID: 4002}\n  m_Father: {fileID: 0}\n" +
		"--- !u!1 &1001\nGameObject:\n  m_Component:\n  - component: {fileID: 4001}\n  m_Name: X\n" +
		"--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_Children: []\n  m_Father: {fileID: 4000}\n" +
		"--- !u!1 &1002\nGameObject:\n  m_Component:\n  - component: {fileID: 4002}\n  m_Name: Y\n" +
		"--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_Children: []\n  m_Father: {fileID: 4000}\n"
}

func refFile(targetFileID int64, targetGUID, ownGUID string) string {
	return "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Script: {fileID: 11500000, guid: " + ownGUID + ", type: 3}\n" +
		"  m_Ref: {fileID: " + itoa(targetFileID) + ", guid: " + targetGUID + ", type: 2}\n"
}

func itoa(v int64) string {
	if v == 4001 {
		return "4001"
	}
	if v == 4002 {
		return "4002"
	}
	return "0"
}

func TestScanInboundFindsCrossFileRefAndExcludesSelf(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	writeAsset(t, root, "B.unity", guidB, refFile(4001, guidA, guidB)) // references A's 4001
	writeAsset(t, root, "C.prefab", guidC, refFile(4001, guidD, guidC)) // references a DIFFERENT guid

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("ScanInbound error = %v", err)
	}
	if res.TargetGUID != guidA {
		t.Fatalf("target guid = %q, want %q", res.TargetGUID, guidA)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" || len(res.Inbound[0].FileIDs) != 1 || res.Inbound[0].FileIDs[0] != 4001 {
		t.Fatalf("unexpected inbound: %+v", res.Inbound)
	}
	if len(res.Indeterminate) != 0 {
		t.Fatalf("unexpected indeterminate: %v", res.Indeterminate)
	}
}

func TestScanInboundSetInput(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	writeAsset(t, root, "B.unity", guidB, refFile(4001, guidA, guidB))
	writeAsset(t, root, "C.prefab", guidC, refFile(4002, guidA, guidC))

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001, 4002}, // set
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 2 {
		t.Fatalf("expected 2 inbound hits (set), got %+v", res.Inbound)
	}
}

func TestScanInboundClassifiesIndeterminate(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	// D has an unparseable PPtr (oversized fileID) -> ExtractRefs warning.
	bad := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Script: {fileID: 999999999999999999999999999999, guid: " + guidD + ", type: 3}\n"
	writeAsset(t, root, "D.unity", guidD, bad)

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Indeterminate) != 1 || res.Indeterminate[0] != "Assets/D.unity" {
		t.Fatalf("expected D.unity indeterminate, got %v", res.Indeterminate)
	}
}

func TestScanInboundNoMatch(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	writeAsset(t, root, "B.unity", guidB, refFile(4001, guidC, guidB)) // refs guidC, not A

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 0 || len(res.Indeterminate) != 0 {
		t.Fatalf("expected no hits, got inbound=%v indeterminate=%v", res.Inbound, res.Indeterminate)
	}
}
