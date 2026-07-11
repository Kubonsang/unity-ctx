package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func runSpatial(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, "ERROR spatial requires validate, diff, review, or apply\n")
		return 2
	}
	command := args[0]
	switch command {
	case "validate":
		return runSpatialValidate(args[1:], stdout, stderr)
	case "diff":
		return runSpatialDiff(args[1:], stdout, stderr)
	case "apply":
		return runSpatialApply(args[1:], stdout, stderr)
	case "review":
		return runSpatialReview(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR spatial command %q is not supported\n", command)
		return 2
	}
}

func runSpatialReview(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("unity-ctx spatial review", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	draft := flags.String("draft", "", "")
	decision := flags.String("decision", "", "")
	reviewer := flags.String("reviewer", "", "")
	issues := flags.String("issues", "", "")
	comment := flags.String("comment", "", "")
	jsonOutput := flags.Bool("json", false, "")
	write := flags.Bool("write", false, "")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*draft) == "" || strings.TrimSpace(*decision) == "" || strings.TrimSpace(*reviewer) == "" {
		_, _ = io.WriteString(stderr, "ERROR spatial review requires --draft, --decision, and --reviewer\n")
		return 2
	}
	contract, err := spatialcontract.Load(*draft)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	issueList := []string{}
	for _, issue := range strings.Split(*issues, ",") {
		if value := strings.TrimSpace(issue); value != "" {
			issueList = append(issueList, value)
		}
	}
	if err := spatialcontract.Review(&contract, *decision, *reviewer, issueList, *comment); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	status := "DRY_RUN"
	if *write {
		if err := spatialcontract.Save(*draft, contract); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
			return 1
		}
		status = "WRITE"
	}
	result := map[string]any{"status": status, "draft": *draft, "decision": *decision, "reviewer": *reviewer, "contract_hash": spatialcontract.ContentHash(contract), "written": *write}
	if *jsonOutput {
		return writeSpatialJSON(stdout, stderr, result)
	}
	_, _ = fmt.Fprintf(stdout, "%s draft=%s decision=%s reviewer=%s written=%d contract_hash=%s\n", status, *draft, *decision, *reviewer, boolInt(*write), spatialcontract.ContentHash(contract))
	return 0
}

func runSpatialValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("unity-ctx spatial validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 2
	}
	if flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, "ERROR spatial validate requires exactly one contract file\n")
		return 2
	}
	path := flags.Arg(0)
	contract, err := spatialcontract.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	hash := spatialcontract.ContentHash(contract)
	if *jsonOutput {
		return writeSpatialJSON(stdout, stderr, map[string]any{"status": "OK", "file": path, "contract_type": contract.ContractType, "state": contract.State, "contract_hash": hash})
	}
	_, _ = fmt.Fprintf(stdout, "OK file=%s type=%s state=%s contract_hash=%s\n", path, contract.ContractType, contract.State, hash)
	return 0
}

func runSpatialDiff(args []string, stdout, stderr io.Writer) int {
	current, draft, jsonOutput, ok := parseSpatialPairFlags("diff", args, stderr, false)
	if !ok {
		return 2
	}
	result, err := spatialcontract.Diff(current, draft)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	if jsonOutput {
		return writeSpatialJSON(stdout, stderr, result)
	}
	fields := "none"
	if len(result.Fields) > 0 {
		fields = strings.Join(result.Fields, ",")
	}
	_, _ = fmt.Fprintf(stdout, "%s current=%s draft=%s changed=%d fields=%s contract_hash=%s\n", result.Status, result.Current, result.Draft, boolInt(result.Changed), fields, result.ContractHash)
	return 0
}

func runSpatialApply(args []string, stdout, stderr io.Writer) int {
	current, draft, jsonOutput, write, ok := parseSpatialApplyFlags(args, stderr)
	if !ok {
		return 2
	}
	result, err := spatialcontract.Apply(current, draft, write)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	if jsonOutput {
		return writeSpatialJSON(stdout, stderr, result)
	}
	_, _ = fmt.Fprintf(stdout, "%s current=%s draft=%s changed=%d written=%d verified=%d contract_hash=%s", result.Status, result.Current, result.Draft, boolInt(result.Changed), boolInt(result.Written), boolInt(result.Verified), result.ContractHash)
	if result.Backup != "" {
		_, _ = fmt.Fprintf(stdout, " backup=%s", result.Backup)
	}
	_, _ = io.WriteString(stdout, "\n")
	return 0
}

func parseSpatialPairFlags(command string, args []string, stderr io.Writer, allowWrite bool) (string, string, bool, bool) {
	flags := flag.NewFlagSet("unity-ctx spatial "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	current := flags.String("current", "", "")
	draft := flags.String("draft", "", "")
	jsonOutput := flags.Bool("json", false, "")
	write := flags.Bool("write", false, "")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return "", "", false, false
	}
	if flags.NArg() != 0 || strings.TrimSpace(*current) == "" || strings.TrimSpace(*draft) == "" {
		_, _ = fmt.Fprintf(stderr, "ERROR spatial %s requires --current and --draft\n", command)
		return "", "", false, false
	}
	if *write && !allowWrite {
		_, _ = fmt.Fprintf(stderr, "ERROR spatial %s does not accept --write\n", command)
		return "", "", false, false
	}
	return *current, *draft, *jsonOutput, true
}

func parseSpatialApplyFlags(args []string, stderr io.Writer) (string, string, bool, bool, bool) {
	flags := flag.NewFlagSet("unity-ctx spatial apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	current := flags.String("current", "", "")
	draft := flags.String("draft", "", "")
	jsonOutput := flags.Bool("json", false, "")
	write := flags.Bool("write", false, "")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return "", "", false, false, false
	}
	if flags.NArg() != 0 || strings.TrimSpace(*current) == "" || strings.TrimSpace(*draft) == "" {
		_, _ = io.WriteString(stderr, "ERROR spatial apply requires --current and --draft\n")
		return "", "", false, false, false
	}
	return *current, *draft, *jsonOutput, *write, true
}

func writeSpatialJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 2
	}
	return 0
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
