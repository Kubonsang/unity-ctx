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

// TestScanInboundDetectsBlockFormRef is the fix for the silent block-style miss:
// a multi-line block PPtr to the target is detected as a real inbound ref (the
// unity-ctx parser yields a nested map for it), NOT silently dropped.
func TestScanInboundDetectsBlockFormRef(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	blockRef := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Link:\n    fileID: 4001\n    guid: " + guidA + "\n    type: 2\n"
	writeAsset(t, root, "B.unity", guidB, blockRef)

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" {
		t.Fatalf("block-form ref not detected as inbound: %+v (indeterminate=%v)", res.Inbound, res.Indeterminate)
	}
}

// TestScanInboundCoversNonStandardExtensions is the fix for the extension
// allow-list miss: a .mat (Unity text-YAML, not .unity/.prefab/.asset) that
// references the target is detected via the %YAML header sniff.
func TestScanInboundCoversNonStandardExtensions(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	mat := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!21 &2100000\nMaterial:\n  m_Name: Mat\n  m_Link: {fileID: 4001, guid: " + guidA + ", type: 2}\n"
	writeAsset(t, root, "M.mat", guidC, mat)

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/M.mat" {
		t.Fatalf(".mat ref not detected as inbound: %+v", res.Inbound)
	}
}

func TestScanInboundClassifiesIndeterminate(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	// A multiline-flow PPtr to the target. The line parser leaves it opaque, but
	// the raw brace-aware scanner (parser.ScanInlinePPtrs) reads across the newline
	// and recovers it precisely -> a real INBOUND hit (better than indeterminate;
	// both BLOCK a delete).
	multiline := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Ref: {fileID: 4001,\n    guid: " + guidA + ", type: 2}\n"
	writeAsset(t, root, "D.unity", guidD, multiline)
	// A genuinely unparseable file (malformed block header) -> parse-error indeterminate.
	broken := "%YAML 1.1\n--- !u!1 1000\nGameObject:\n  m_Name: X\n"
	writeAsset(t, root, "E.unity", "e5f60718293a4b5c6d7e8f90a1b2c3d4", broken)

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// D.unity (multiline-flow) is now precisely detected as inbound.
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/D.unity" || res.Inbound[0].FileIDs[0] != 4001 {
		t.Fatalf("multiline-flow ref not detected as inbound: %+v", res.Inbound)
	}
	// E.unity (parse failure) stays conservatively indeterminate (never silent).
	hasE := false
	for _, p := range res.Indeterminate {
		if p == "Assets/E.unity" {
			hasE = true
		}
	}
	if !hasE {
		t.Fatalf("unparseable file not flagged indeterminate: %v", res.Indeterminate)
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

// TestScanInboundDoesNotAbortOnUnreadableDir is the fix for the WalkDir abort
// (return werr): a single unreadable directory must NOT drop all detection — the
// rest of the scan continues and the unreadable path is flagged indeterminate.
func TestScanInboundDoesNotAbortOnUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs as root; permission bits do not block root")
	}
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	writeAsset(t, root, "B.unity", guidB, refFile(4001, guidA, guidB)) // genuine inbound ref
	locked := filepath.Join(root, "Assets", "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(locked, "hidden.unity"), []byte(refFile(4001, guidA, guidC)), 0o644); err != nil {
		t.Fatalf("write hidden: %v", err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // restore so TempDir cleanup can remove it

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("scan must not abort on an unreadable dir: %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" {
		t.Fatalf("readable inbound ref was dropped: inbound=%+v", res.Inbound)
	}
	found := false
	for _, p := range res.Indeterminate {
		if p == "Assets/locked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unreadable dir not flagged indeterminate: %v", res.Indeterminate)
	}
}

// TestScanInboundExcludesTargetViaSymlinkedPath is the fix for lexical-only
// self-exclusion: when the project is reached through a symlink (so the walked
// path and the target arg are spelled differently), the target must still be
// recognized as itself and not scanned as its own referrer.
func TestScanInboundExcludesTargetViaSymlinkedPath(t *testing.T) {
	root := t.TempDir()
	// A.unity references its OWN guid (e.g. a prefab-variant self-PPtr); were the
	// target not excluded it would be reported as inbound to itself.
	selfRef := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Self: {fileID: 4001, guid: " + guidA + ", type: 2}\n"
	writeAsset(t, root, "A.unity", guidA, selfRef)

	link := filepath.Join(t.TempDir(), "proj")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	res, err := ScanInbound(Request{
		ProjectPath: link,                                     // symlinked spelling
		TargetPath:  filepath.Join(root, "Assets", "A.unity"), // real spelling
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 0 {
		t.Fatalf("target scanned as its own referrer through a symlink: %+v", res.Inbound)
	}
	if len(res.Indeterminate) != 0 {
		t.Fatalf("unexpected indeterminate: %v", res.Indeterminate)
	}
}

// TestScanInboundDoesNotFlagNonPPtrGUIDMention is the fix for the over-eager
// completeness backstop: a target-GUID mention the parser fully recovered but
// that is NOT a fileID-bearing PPtr (an Addressables m_AssetGUID string, or a
// bare guid field) must NOT be reported as indeterminate, and is not inbound.
func TestScanInboundDoesNotFlagNonPPtrGUIDMention(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	body := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_AssetGUID: " + guidA + "\n" + // Addressables-style plain guid string
		"  m_BareRef:\n    guid: " + guidA + "\n    something: 5\n" // guid map, no fileID
	writeAsset(t, root, "B.unity", guidB, body)

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 0 {
		t.Fatalf("non-PPtr guid mention misreported as inbound: %+v", res.Inbound)
	}
	if len(res.Indeterminate) != 0 {
		t.Fatalf("fully-parsed non-PPtr guid mention falsely flagged indeterminate: %v", res.Indeterminate)
	}
}

// TestScanInboundDetectsBOMPrefixedReferrer is the fix for the byte-0 %YAML
// gate: a referrer written with a leading UTF-8 BOM must still be detected as
// inbound, not silently skipped as "not YAML".
func TestScanInboundDetectsBOMPrefixedReferrer(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	bom := string([]byte{0xEF, 0xBB, 0xBF})
	writeAsset(t, root, "B.unity", guidB, bom+refFile(4001, guidA, guidB))

	res, err := ScanInbound(Request{
		ProjectPath: root,
		TargetPath:  filepath.Join(root, "Assets", "A.unity"),
		FileIDs:     []int64{4001},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" {
		t.Fatalf("BOM-prefixed referrer not detected: inbound=%+v indeterminate=%v", res.Inbound, res.Indeterminate)
	}
}

// TestScanInboundDoesNotFlagGUIDOnlyInMapKey is the fix for the backstop counting
// only string VALUES: a target GUID used as a YAML map key (a serialized
// dictionary) is fully parsed and must NOT be flagged indeterminate.
func TestScanInboundDoesNotFlagGUIDOnlyInMapKey(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	body := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Dict:\n    " + guidA + ": 5\n" // target guid as a map KEY, not a PPtr
	writeAsset(t, root, "B.unity", guidB, body)

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 0 {
		t.Fatalf("guid-as-key misreported as inbound: %+v", res.Inbound)
	}
	if len(res.Indeterminate) != 0 {
		t.Fatalf("fully-parsed guid map key falsely flagged indeterminate: %v", res.Indeterminate)
	}
}

// TestScanInboundExcludesTargetViaDifferentlyNamedSymlink is the fix for the
// basename-gated self-exclusion hole: a leaf symlink to the target under a
// DIFFERENT name must still be recognized as the target and not scanned as its
// own referrer.
func TestScanInboundExcludesTargetViaDifferentlyNamedSymlink(t *testing.T) {
	root := t.TempDir()
	selfRef := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Self: {fileID: 4001, guid: " + guidA + ", type: 2}\n"
	writeAsset(t, root, "A.unity", guidA, selfRef)
	alias := filepath.Join(root, "Assets", "Alias.unity")
	if err := os.Symlink(filepath.Join(root, "Assets", "A.unity"), alias); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 0 {
		t.Fatalf("differently-named symlink alias to target scanned as a referrer: %+v", res.Inbound)
	}
}

// TestScanInboundFlagsBinaryUnityAsset is the fix for the silent skip of
// binary-serialized Unity assets: a .prefab that is NOT text-YAML is flagged
// indeterminate (cannot be scanned), while a non-asset binary (.png) is skipped.
func TestScanInboundFlagsBinaryUnityAsset(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	bin := string([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}) // not %YAML
	writeAsset(t, root, "Binary.prefab", guidC, bin)          // known Unity asset ext
	writeAsset(t, root, "Texture.png", guidD, bin)            // non-asset binary

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	hasBinary, hasPNG := false, false
	for _, p := range res.Indeterminate {
		if p == "Assets/Binary.prefab" {
			hasBinary = true
		}
		if p == "Assets/Texture.png" {
			hasPNG = true
		}
	}
	if !hasBinary {
		t.Fatalf("binary .prefab not flagged indeterminate: %v", res.Indeterminate)
	}
	if hasPNG {
		t.Fatalf("non-asset binary .png should be skipped, not flagged: %v", res.Indeterminate)
	}
}

// TestScanInboundDetectsFlowSequenceCrossFileRef is the fix for the cross-file
// flow-sequence blind spot: a referrer pointing at the target via a single-line
// FLOW list (which the parser renders as an opaque string) must be detected as
// inbound, never reported as a silent "no refs".
func TestScanInboundDetectsFlowSequenceCrossFileRef(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	flow := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Targets: [{fileID: 4001, guid: " + guidA + ", type: 3}]\n"
	writeAsset(t, root, "B.unity", guidB, flow)

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" || len(res.Inbound[0].FileIDs) != 1 || res.Inbound[0].FileIDs[0] != 4001 {
		t.Fatalf("flow-sequence cross-file ref not detected: inbound=%+v indeterminate=%v", res.Inbound, res.Indeterminate)
	}
}

// TestScanInboundDetectsFlowSequenceMultiItem covers a multi-entry flow list.
func TestScanInboundDetectsFlowSequenceMultiItem(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	flow := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Refs: [{fileID: 4001, guid: " + guidA + ", type: 3}, {fileID: 4002, guid: " + guidA + ", type: 3}]\n"
	writeAsset(t, root, "B.unity", guidB, flow)

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001, 4002}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || len(res.Inbound[0].FileIDs) != 2 {
		t.Fatalf("multi-item flow-sequence refs not both detected: inbound=%+v", res.Inbound)
	}
}

// TestScanInboundScansPackages: an embedded/local package asset under Packages/
// that references the target must be detected (not a silent pass at the scope
// boundary).
func TestScanInboundScansPackages(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	pkgDir := filepath.Join(root, "Packages", "com.example.foo")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "Foo.asset"), []byte(refFile(4001, guidA, guidC)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "Foo.asset.meta"), []byte("guid: "+guidC+"\n"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Packages/com.example.foo/Foo.asset" {
		t.Fatalf("Packages referrer not detected: %+v", res.Inbound)
	}
}

// TestScanInboundApostropheDoesNotHideRef: a literal apostrophe in an unquoted
// name before a flow-list PPtr must not stop detection (the quote-tracking bug).
func TestScanInboundApostropheDoesNotHideRef(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	body := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!114 &9000\nMonoBehaviour:\n  m_GameObject: {fileID: 0}\n" +
		"  m_Name: Player's Gun\n  m_Targets: [{fileID: 4001, guid: " + guidA + ", type: 3}]\n"
	writeAsset(t, root, "B.unity", guidB, body)

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0].Path != "Assets/B.unity" {
		t.Fatalf("apostrophe-before-ref not detected: %+v indeterminate=%v", res.Inbound, res.Indeterminate)
	}
}

// TestScanInboundFlagsSymlinkedDirIndeterminate: WalkDir does not descend a
// symlinked directory, so its contents are unscanned -> flag it indeterminate
// rather than silently report "no refs".
func TestScanInboundFlagsSymlinkedDirIndeterminate(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "A.unity", guidA, targetScene())
	realDir := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "Assets", "Linked")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	res, err := ScanInbound(Request{ProjectPath: root, TargetPath: filepath.Join(root, "Assets", "A.unity"), FileIDs: []int64{4001}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	found := false
	for _, p := range res.Indeterminate {
		if p == "Assets/Linked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("symlinked dir not flagged indeterminate: %v", res.Indeterminate)
	}
}
