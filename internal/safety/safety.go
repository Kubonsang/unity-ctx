// Package safety adapts the unity-fileid-graph safety kernel for unity-ctx
// write paths. It is the only package in unity-ctx allowed to import
// github.com/Kubonsang/unity-fileid-graph.
package safety

import (
	"fmt"
	"strings"

	fgcheck "github.com/Kubonsang/unity-fileid-graph/pkg/check"
	fgcore "github.com/Kubonsang/unity-fileid-graph/pkg/core"
	fggraph "github.com/Kubonsang/unity-fileid-graph/pkg/graph"
	fgparser "github.com/Kubonsang/unity-fileid-graph/pkg/parser"
)

type Phase string

const (
	PhasePre   Phase = "pre_check"
	PhaseTemp  Phase = "temp_check"
	PhaseFinal Phase = "final_check"
)

const (
	StatusOK    = "OK"
	StatusWarn  = "WARN"
	StatusError = "ERROR"

	CodeParseFailed      = "PARSE_FAILED"
	CodeGraphBuildFailed = "GRAPH_BUILD_FAILED"
)

type Finding struct {
	Severity string
	Code     string
	Detail   string
}

type Report struct {
	Status      string
	Blocks      int
	GameObjects int
	Components  int
	Transforms  int
	Findings    []Finding
}

func (r Report) Blocking() bool {
	return r.Status == StatusError
}

func (r Report) counts() (errors, warnings int) {
	for _, f := range r.Findings {
		if f.Severity == StatusError {
			errors++
		} else {
			warnings++
		}
	}
	return errors, warnings
}

// Lines renders detail lines for a non-OK report:
// "CHECK phase=... status=... errors=N warnings=M" followed by one
// "ERROR code=..."/"WARN code=..." line per finding. Empty for OK.
func (r Report) Lines(phase Phase) []string {
	if r.Status == StatusOK {
		return nil
	}
	errors, warnings := r.counts()
	lines := []string{fmt.Sprintf("CHECK phase=%s status=%s errors=%d warnings=%d", phase, r.Status, errors, warnings)}
	for _, f := range r.Findings {
		line := fmt.Sprintf("%s code=%s", f.Severity, f.Code)
		if f.Detail != "" {
			line += " " + f.Detail
		}
		lines = append(lines, line)
	}
	return lines
}

// CheckBytes runs the fileid-graph safety kernel (parse, graph build,
// integrity check) over raw Unity YAML bytes. It never returns an error:
// parse or build failures are reported as blocking findings.
func CheckBytes(data []byte) Report {
	parsed, err := fgparser.Parse(data)
	if err != nil {
		return Report{
			Status:   StatusError,
			Findings: []Finding{{Severity: StatusError, Code: CodeParseFailed, Detail: fmt.Sprintf("message=%q", err.Error())}},
		}
	}
	g, err := fggraph.Build(parsed)
	if err != nil {
		return Report{
			Status:   StatusError,
			Findings: []Finding{{Severity: StatusError, Code: CodeGraphBuildFailed, Detail: fmt.Sprintf("message=%q", err.Error())}},
		}
	}
	result := fgcheck.Run(g)
	report := Report{
		Status:      result.Status,
		Blocks:      result.BlockCount,
		GameObjects: result.GameObjectCount,
		Components:  result.ComponentCount,
		Transforms:  result.TransformCount,
	}
	for _, finding := range result.Errors {
		report.Findings = append(report.Findings, Finding{
			Severity: StatusError,
			Code:     finding.Code,
			Detail:   errorDetail(finding),
		})
	}
	for _, finding := range result.Warnings {
		report.Findings = append(report.Findings, Finding{
			Severity: StatusWarn,
			Code:     finding.Code,
			Detail:   warningDetail(finding),
		})
	}
	return report
}

// errorDetail mirrors uyaml's check output field rendering so both tools
// emit the same key=value dialect for the same finding codes.
func errorDetail(finding fgcore.CheckFinding) string {
	var b strings.Builder
	switch finding.Code {
	case fgcore.CheckDuplicateFileID:
		fmt.Fprintf(&b, "file_id=%d duplicates=%d", finding.FileID, finding.DuplicateCount)
	case fgcore.CheckMissingComponentBlock:
		fmt.Fprintf(&b, "go=%d component_id=%d reason=%s", finding.GameObjectID, finding.ComponentID, finding.Reason)
	case fgcore.CheckMissingGameObjectBlock:
		fmt.Fprintf(&b, "component=%d m_GameObject=%d reason=%s", finding.ComponentID, finding.GameObjectID, finding.Reason)
	case fgcore.CheckGoComponentBackrefMismatch:
		fmt.Fprintf(&b, "component=%d go=%d reason=%s", finding.ComponentID, finding.GameObjectID, finding.Reason)
	case fgcore.CheckTransformParentChildMismatch:
		if finding.ParentID != 0 {
			fmt.Fprintf(&b, " parent=%d", finding.ParentID)
		}
		if finding.ChildID != 0 {
			fmt.Fprintf(&b, " child=%d", finding.ChildID)
		}
		if finding.TransformID != 0 {
			fmt.Fprintf(&b, " transform=%d", finding.TransformID)
		}
		if finding.Reason != "" {
			fmt.Fprintf(&b, " reason=%s", finding.Reason)
		}
	case fgcore.CheckMissingTransformComponent:
		fmt.Fprintf(&b, "go=%d reason=%s", finding.GameObjectID, finding.Reason)
	case fgcore.CheckSuspiciousMonoBehaviourScript:
		fmt.Fprintf(&b, "component=%d reason=%s", finding.ComponentID, finding.Reason)
	default:
		if finding.FileID != 0 {
			fmt.Fprintf(&b, " file_id=%d", finding.FileID)
		}
		if finding.Reason != "" {
			fmt.Fprintf(&b, " reason=%s", finding.Reason)
		}
	}
	return strings.TrimSpace(b.String())
}

func warningDetail(finding fgcore.CheckFinding) string {
	var b strings.Builder
	if finding.FileID != 0 {
		fmt.Fprintf(&b, " file_id=%d", finding.FileID)
	}
	if finding.ComponentID != 0 {
		fmt.Fprintf(&b, " component=%d", finding.ComponentID)
	}
	if finding.Reason != "" {
		fmt.Fprintf(&b, " reason=%s", finding.Reason)
	}
	if finding.Message != "" {
		fmt.Fprintf(&b, " message=%q", finding.Message)
	}
	return strings.TrimSpace(b.String())
}
