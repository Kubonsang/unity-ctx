package cli

import (
	"fmt"
	"sort"
	"strings"
)

// knownNamespaces is the set of valid first tokens. meta and mcp are
// pseudo-namespaces handled by dedicated branches in Run.
var knownNamespaces = map[string]bool{
	"scene": true, "prefab": true, "asset": true, "meta": true, "mcp": true,
}

// commandSpec describes a command for --help and error diagnostics. It does not
// drive flag gating (the per-command checks in Run still own that); it only
// feeds human-facing help and better error messages.
type commandSpec struct {
	synopsis string
	flags    string
}

var commandSpecs = map[string]commandSpec{
	"summarize":    {"compact overview of a file (object/component counts)", "[--json] [--view]"},
	"query":        {"find objects by name, fileID, or component type", "--name X | --id N | --type T  [--json]"},
	"inspect":      {"show a component's fields for an object", "[--id N | --name X] [--component C] [--json]"},
	"get":          {"read a single field value", "[--id N | --name X] [--component C] --field F [--json]"},
	"refs":         {"PPtr/GUID reference evidence (read-only)", "[--json]"},
	"validate":     {"fileID graph integrity check (read-only); ERROR exits 1", "[--json]"},
	"changes":      {"structural diff of a file vs its <file>.bak", "[--json]"},
	"restore":      {"recover a file from its <file>.bak", "[--json]"},
	"deps":         {"forward asset dependencies resolved to project paths", "--project DIR [--out FILE.dot] [--json]"},
	"context-pack": {"assemble a token-budgeted context bundle for a task", "--task T | --focus F [--max-tokens N] [--json]"},
	"bench":        {"token-reduction metrics (raw vs summarize vs context-pack)", "[--task T] [--json]"},
	"index":        {"write a block-index snapshot", "--out FILE [--json]"},
	"impact":       {"scenes/prefabs that reference a prefab (prefab namespace)", "--project DIR [--scenes a,b] [--json]"},
	"set":          {"set a field; dry-run first, --write to commit", "--field F --value V [--id N] [--write]  (prefab: --project DIR --id N --ack-impact) [--json]"},
	"reposition":   {"set a Transform's m_LocalPosition; dry-run first, --write to commit (scene)", "--id N --position x,y,z [--write] [--json]"},
	"scan":         {"generate a bounds manifest via the Unity Editor (scene)", "--mode editor --project DIR --out FILE [--prefabs a,b] [--geometry detailed] [--contracts DIR] [--json]"},
	"check":        {"placement overlap/contact check against a manifest (scene)", "--manifest FILE --prefab P --position x,y,z [--rotation x,y,z,w] [--surface-id ID --contact KIND | --contact-surfaces requirement-id=surface-id,...] [--json]"},
	"patch":        {"build a patch plan (scene): place_prefab (v1), reparent or delete (v2 ops[])", "--op place_prefab --manifest FILE --prefab P --position x,y,z [--prefab-guid G] [--project DIR] | --op reparent --id N --new-parent M | --op delete --id N [--cascade]  [--json]"},
	"diff":         {"summarize a persisted patch plan (scene; v1 or v2)", "--patch FILE [--json]"},
	"apply":        {"apply a patch plan; dry-run first, --write to commit (scene)", "--patch FILE [--write] (reparent/delete: --ack-impact; delete --write requires --project; [--project DIR for cross-file report]) [--json]"},
	"suggest":      {"rank prefab placement candidates near an anchor or reviewed wall (scene)", "--manifest FILE --prefab P (--near A [--align floor|grid] | --align wall --surface-id ID [--contact wall-backed|wall-mounted]) [--count N] [--json]"},
}

func knownCommand(name string) bool {
	_, ok := commandSpecs[name]
	return ok
}

// wantsHelp reports whether --help/-h appears anywhere in args.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// helpText returns per-command help when command is known, otherwise the
// general usage overview.
func helpText(namespace, command string) string {
	if namespace == "spatial" {
		switch command {
		case "validate":
			return "unity-ctx spatial validate <file> [--json]\n  strictly validate a Spatial Contract and its embedded contract hash\n"
		case "verify-approved":
			return "unity-ctx spatial verify-approved <file> [--json]\n  verify the exact Approved contract against the external human-approval ledger\n"
		case "diff":
			return "unity-ctx spatial diff --current FILE --draft FILE [--json]\n  compare a stored Spatial Contract with a reviewed draft without writing\n"
		case "review":
			return "unity-ctx spatial review --draft FILE --decision RevisionRequested|UnableToJudge --reviewer ID [--issues a,b] [--comment TEXT] [--write] [--json]\n  record a non-approval review decision; approval is restricted to the trusted local bridge\n"
		case "apply":
			return "unity-ctx spatial apply --current FILE --draft FILE [--json]\n  dry-run a reviewed contract apply; writing is restricted to the trusted local bridge\n"
		}
	}
	if namespace == "arrangement" {
		switch command {
		case "validate":
			return "unity-ctx arrangement validate <file> [--json]\n  strictly validate and normalize a Surface Arrangement v1 spec\n"
		case "hash":
			return "unity-ctx arrangement hash <file> [--json]\n  print the stable normalized Surface Arrangement v1 spec hash\n"
		}
	}
	if spec, ok := commandSpecs[command]; ok {
		ns := namespace
		if ns == "" || !knownNamespaces[ns] {
			ns = "<namespace>"
		}
		return fmt.Sprintf("unity-ctx %s %s <file> [flags]\n  %s\n  flags: %s\n", ns, command, spec.synopsis, spec.flags)
	}
	if command == "guid" {
		return "unity-ctx meta guid <file> [--project DIR] [--json]\n  resolve a prefab/asset GUID from its sibling .meta file\n"
	}
	return generalUsage()
}

func generalUsage() string {
	var read, write []string
	for name, spec := range commandSpecs {
		entry := name
		_ = spec
		if name == "set" || name == "reposition" || name == "scan" || name == "check" || name == "patch" ||
			name == "diff" || name == "apply" || name == "suggest" || name == "restore" || name == "index" {
			write = append(write, entry)
		} else {
			read = append(read, entry)
		}
	}
	sort.Strings(read)
	sort.Strings(write)

	var b strings.Builder
	b.WriteString("unity-ctx — token-safe Unity scene/prefab/asset interface for AI agents\n\n")
	b.WriteString("usage: unity-ctx <namespace> <command> <file> [flags]\n")
	b.WriteString("       unity-ctx meta guid <file> [--project DIR]\n")
	b.WriteString("       unity-ctx spatial validate|verify-approved|diff|review|apply [flags]\n")
	b.WriteString("       unity-ctx arrangement validate|hash <file> [--json]\n")
	b.WriteString("       unity-ctx mcp                          # MCP server over stdio\n\n")
	b.WriteString("namespaces: scene | prefab | asset\n\n")
	b.WriteString("read commands:  " + strings.Join(read, " ") + "\n")
	b.WriteString("write commands: " + strings.Join(write, " ") + "\n")
	b.WriteString("other:          spatial validate|verify-approved|diff|review|apply  arrangement validate|hash  impact (prefab)  meta guid  mcp\n\n")
	b.WriteString("Run 'unity-ctx <namespace> <command> --help' for command-specific usage.\n")
	return b.String()
}

// diagnoseShape returns a precise error message (and exit code) for an
// incomplete or malformed invocation, or stop=false to let Run proceed. It is
// called after the help and mcp branches. The "missing file argument" message
// is preserved for the namespace+command+no-file case.
func diagnoseShape(args []string) (msg string, code int, stop bool) {
	if len(args) == 0 {
		return "ERROR no command\n\n" + generalUsage(), 2, true
	}

	ns := args[0]
	if !knownNamespaces[ns] {
		if knownCommand(ns) {
			return fmt.Sprintf("ERROR %q is a command, not a namespace — did you omit the namespace? e.g. unity-ctx scene %s ...", ns, ns), 2, true
		}
		return fmt.Sprintf("ERROR unknown namespace %q (expected scene, prefab, asset, meta, mcp)", ns), 2, true
	}

	// meta is dispatched separately (meta guid <file>); just require 3 tokens.
	if ns == "meta" {
		if len(args) < 3 {
			return "ERROR missing file argument", 2, true
		}
		return "", 0, false
	}

	if len(args) < 2 {
		return fmt.Sprintf("ERROR missing command for namespace %q", ns), 2, true
	}
	command := args[1]
	if !knownCommand(command) {
		return fmt.Sprintf("ERROR unknown command %q for namespace %q", command, ns), 2, true
	}
	if len(args) < 3 {
		return "ERROR missing file argument", 2, true
	}
	return "", 0, false
}
