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
