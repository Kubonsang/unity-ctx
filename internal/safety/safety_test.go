package safety

import (
	"strings"
	"testing"
)

const validPrefab = `--- !u!1 &1000
GameObject:
  m_Name: Healthy
  m_Component:
  - component: {fileID: 4000}
--- !u!4 &4000
Transform:
  m_GameObject: {fileID: 1000}
  m_Father: {fileID: 0}
  m_Children: []
`

const duplicateFileIDPrefab = `--- !u!1 &1000
GameObject:
  m_Name: First
  m_Component:
  - component: {fileID: 4000}
--- !u!4 &4000
Transform:
  m_GameObject: {fileID: 1000}
  m_Father: {fileID: 0}
  m_Children: []
--- !u!1 &1000
GameObject:
  m_Name: Duplicate
  m_Component:
  - component: {fileID: 4000}
`

const tabIndentPrefab = "--- !u!1 &1000\nGameObject:\n\tm_Name: Tabbed\n--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Father: {fileID: 0}\n  m_Children: []\n"

func TestCheckBytesValidPrefabIsOK(t *testing.T) {
	report := CheckBytes([]byte(validPrefab))
	if report.Status != StatusOK {
		t.Fatalf("expected OK, got %s findings=%v", report.Status, report.Findings)
	}
	if report.Blocking() {
		t.Fatal("OK report must not block")
	}
	if report.Blocks != 2 || report.GameObjects != 1 || report.Transforms != 1 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if lines := report.Lines(PhasePre); len(lines) != 0 {
		t.Fatalf("OK report must render no lines, got %v", lines)
	}
}

func TestCheckBytesDuplicateFileIDBlocks(t *testing.T) {
	report := CheckBytes([]byte(duplicateFileIDPrefab))
	if report.Status != StatusError {
		t.Fatalf("expected ERROR, got %s", report.Status)
	}
	if !report.Blocking() {
		t.Fatal("ERROR report must block")
	}
	found := false
	for _, f := range report.Findings {
		if f.Severity == StatusError && f.Code == "DUPLICATE_FILE_ID" {
			found = true
			if !strings.Contains(f.Detail, "file_id=1000") {
				t.Fatalf("detail missing file_id: %q", f.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("expected DUPLICATE_FILE_ID finding, got %v", report.Findings)
	}
	lines := report.Lines(PhaseTemp)
	if len(lines) < 2 {
		t.Fatalf("expected CHECK line plus findings, got %v", lines)
	}
	if !strings.HasPrefix(lines[0], "CHECK phase=temp_check status=ERROR errors=") {
		t.Fatalf("unexpected CHECK line: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "ERROR code=DUPLICATE_FILE_ID file_id=1000") {
		t.Fatalf("unexpected finding line: %q", lines[1])
	}
}

func TestCheckBytesTabIndentWarnsWithoutBlocking(t *testing.T) {
	report := CheckBytes([]byte(tabIndentPrefab))
	if report.Status != StatusWarn {
		t.Fatalf("expected WARN, got %s findings=%v", report.Status, report.Findings)
	}
	if report.Blocking() {
		t.Fatal("WARN report must not block")
	}
	found := false
	for _, f := range report.Findings {
		if f.Severity == StatusWarn && f.Code == "TAB_INDENT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TAB_INDENT warning, got %v", report.Findings)
	}
	lines := report.Lines(PhasePre)
	if !strings.HasPrefix(lines[0], "CHECK phase=pre_check status=WARN") {
		t.Fatalf("unexpected CHECK line: %q", lines[0])
	}
}

func TestCheckBytesMalformedHeaderReportsParseFailed(t *testing.T) {
	report := CheckBytes([]byte("--- !u!abc &xyz\nGameObject:\n"))
	if report.Status != StatusError {
		t.Fatalf("expected ERROR, got %s", report.Status)
	}
	if len(report.Findings) != 1 || report.Findings[0].Code != CodeParseFailed {
		t.Fatalf("expected single PARSE_FAILED finding, got %v", report.Findings)
	}
}

const refsPrefab = `--- !u!1 &1000
GameObject:
  m_Name: Enemy
  m_Component:
  - component: {fileID: 4000}
  - component: {fileID: 11400000}
--- !u!4 &4000
Transform:
  m_GameObject: {fileID: 1000}
  m_Father: {fileID: 0}
  m_Children: []
--- !u!114 &11400000
MonoBehaviour:
  m_GameObject: {fileID: 1000}
  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}
  maxHealth: 200
`

func TestExtractRefsReturnsScriptReference(t *testing.T) {
	report, err := ExtractRefs([]byte(refsPrefab), "prefab", "Enemy.prefab")
	if err != nil {
		t.Fatalf("ExtractRefs() error = %v", err)
	}
	if report.Status != StatusOK {
		t.Fatalf("status mismatch: got %s warnings=%v", report.Status, report.Warnings)
	}
	found := false
	for _, ref := range report.Refs {
		if ref.Field == "m_Script" {
			found = true
			if ref.Block != 11400000 || ref.Class != "MonoBehaviour" {
				t.Fatalf("ref owner mismatch: %+v", ref)
			}
			if !ref.HasGUID || ref.GUID != "a1b2c3d4e5f60718293a4b5c6d7e8f90" {
				t.Fatalf("ref guid mismatch: %+v", ref)
			}
			if !ref.HasType || ref.Type != 3 {
				t.Fatalf("ref type mismatch: %+v", ref)
			}
		}
		if ref.Field == "m_GameObject" || ref.Field == "m_Father" {
			t.Fatalf("graph-structural field must be skipped: %+v", ref)
		}
	}
	if !found {
		t.Fatalf("expected m_Script ref, got %+v", report.Refs)
	}
}

func TestExtractRefsErrorsOnUnparseableInput(t *testing.T) {
	if _, err := ExtractRefs([]byte("--- !u!abc &xyz\n"), "prefab", "x.prefab"); err == nil {
		t.Fatal("expected parse error")
	}
}
