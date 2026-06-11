package app

import (
	"fmt"
	"strings"

	"unity-ctx/internal/safety"
)

// phaseCheck pairs a safety report with the write-path phase it ran in.
type phaseCheck struct {
	phase  safety.Phase
	report safety.Report
}

// checkSuffix renders the per-phase status fields appended to summary
// lines, e.g. " pre_check=OK temp_check=WARN".
func checkSuffix(checks []phaseCheck) string {
	var b strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&b, " %s=%s", c.phase, c.report.Status)
	}
	return b.String()
}

// checkDetailLines renders CHECK + finding lines for every non-OK phase,
// prefixed with newlines so the result can be appended to a body line.
func checkDetailLines(checks []phaseCheck) string {
	var b strings.Builder
	for _, c := range checks {
		for _, line := range c.report.Lines(c.phase) {
			b.WriteString("\n")
			b.WriteString(line)
		}
	}
	return b.String()
}

// blockedBody renders the first line and detail lines for a write path
// refused by a blocking graph-check failure.
func blockedBody(kv string, check phaseCheck) string {
	body := fmt.Sprintf("BLOCKED code=GRAPH_CHECK_FAILED phase=%s%s", check.phase, kv)
	return body + checkDetailLines([]phaseCheck{check})
}

type SafetyFinding struct {
	Phase    string `json:"phase"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Detail   string `json:"detail,omitempty"`
}

type SafetyPayload struct {
	PreCheck   string          `json:"pre_check,omitempty"`
	TempCheck  string          `json:"temp_check,omitempty"`
	FinalCheck string          `json:"final_check,omitempty"`
	Findings   []SafetyFinding `json:"findings,omitempty"`
}

func newSafetyPayload(checks []phaseCheck) *SafetyPayload {
	if len(checks) == 0 {
		return nil
	}
	payload := &SafetyPayload{}
	for _, c := range checks {
		switch c.phase {
		case safety.PhasePre:
			payload.PreCheck = c.report.Status
		case safety.PhaseTemp:
			payload.TempCheck = c.report.Status
		case safety.PhaseFinal:
			payload.FinalCheck = c.report.Status
		}
		for _, f := range c.report.Findings {
			payload.Findings = append(payload.Findings, SafetyFinding{
				Phase:    string(c.phase),
				Severity: f.Severity,
				Code:     f.Code,
				Detail:   f.Detail,
			})
		}
	}
	return payload
}
