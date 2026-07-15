package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/surfacearrangement"
)

func runArrangement(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, "ERROR arrangement requires validate or hash\n")
		return 2
	}
	switch args[0] {
	case "validate":
		return runArrangementValidate(args[1:], stdout, stderr)
	case "hash":
		return runArrangementHash(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR arrangement command %q is not supported\n", args[0])
		return 2
	}
}

func runArrangementValidate(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, ok := parseArrangementFile("validate", args, stderr)
	if !ok {
		return 2
	}
	spec, err := surfacearrangement.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	result := map[string]any{
		"status":                      "OK",
		"file":                        path,
		"surface_arrangement_version": spec.SurfaceArrangementVersion,
		"preset":                      spec.Preset,
		"members":                     len(spec.Members),
		"spec_hash":                   spec.SpecHash,
	}
	if jsonOutput {
		return writeArrangementJSON(stdout, stderr, result)
	}
	_, _ = fmt.Fprintf(stdout, "OK file=%s version=%d preset=%s members=%d spec_hash=%s\n", path, spec.SurfaceArrangementVersion, spec.Preset, len(spec.Members), spec.SpecHash)
	return 0
}

func runArrangementHash(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, ok := parseArrangementFile("hash", args, stderr)
	if !ok {
		return 2
	}
	spec, err := surfacearrangement.LoadForHash(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 1
	}
	result := map[string]any{"status": "OK", "file": path, "spec_hash": spec.SpecHash}
	if jsonOutput {
		return writeArrangementJSON(stdout, stderr, result)
	}
	_, _ = fmt.Fprintf(stdout, "OK file=%s spec_hash=%s\n", path, spec.SpecHash)
	return 0
}

func parseArrangementFile(command string, args []string, stderr io.Writer) (string, bool, bool) {
	flags := flag.NewFlagSet("unity-ctx arrangement "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	// The public syntax is `<file> [--json]`. Go's flag package stops at the
	// first positional argument, so move flags ahead while preserving their
	// relative order and allow the documented order as well as flags-first.
	ordered := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			ordered = append(ordered, arg)
		}
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			ordered = append(ordered, arg)
		}
	}
	if err := flags.Parse(ordered); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return "", false, false
	}
	if flags.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "ERROR arrangement %s requires exactly one spec file\n", command)
		return "", false, false
	}
	return flags.Arg(0), *jsonOutput, true
}

func writeArrangementJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 2
	}
	return 0
}
