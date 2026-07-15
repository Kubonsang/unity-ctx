// Package mcp exposes unity-ctx's read-only commands as a Model Context
// Protocol (MCP) server over stdio, so MCP hosts (Claude Code, etc.) can call
// them as native tools instead of shelling out. It speaks newline-delimited
// JSON-RPC 2.0 and implements initialize / tools/list / tools/call.
//
// Only read-only commands are exposed; mutations stay behind the CLI's
// dry-run-first, --write contract.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/app"
	"github.com/Kubonsang/unity-ctx/internal/core"
	"github.com/Kubonsang/unity-ctx/internal/version"
)

const protocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	handler     func(svc *app.Service, args map[string]any) (string, bool)
}

// Serve runs the MCP server loop, reading newline-delimited JSON-RPC requests
// from in and writing responses to out, until in is exhausted.
func Serve(svc *app.Service, in io.Reader, out io.Writer) error {
	tools := buildTools()
	enc := json.NewEncoder(out)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Can't recover an id; skip malformed input.
			continue
		}
		resp, hasResp := dispatch(svc, tools, req)
		if !hasResp {
			continue // notification
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func dispatch(svc *app.Service, tools []tool, req rpcRequest) (rpcResponse, bool) {
	// Notifications (no id) get no response.
	if len(req.ID) == 0 {
		return rpcResponse{}, false
	}
	base := rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		base.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "unity-ctx", "version": version.Version},
		}
	case "ping":
		base.Result = map[string]any{}
	case "tools/list":
		base.Result = map[string]any{"tools": tools}
	case "tools/call":
		base.Result, base.Error = callTool(svc, tools, req.Params)
	default:
		base.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return base, true
}

func callTool(svc *app.Service, tools []tool, params json.RawMessage) (any, *rpcError) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	for _, t := range tools {
		if t.Name != call.Name {
			continue
		}
		text, isErr := t.handler(svc, call.Arguments)
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isErr,
		}, nil
	}
	return nil, &rpcError{Code: -32602, Message: "unknown tool: " + call.Name}
}

// --- argument helpers ---

func strArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func int64Arg(args map[string]any, key string) (int64, bool) {
	switch v := args[key].(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < -9223372036854775808.0 || v >= 9223372036854775808.0 {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}

func floatArrayArg(args map[string]any, key string, count int) ([]float64, bool) {
	v, ok := args[key].([]any)
	if !ok || len(v) != count {
		return nil, false
	}
	result := make([]float64, count)
	for i, item := range v {
		number, ok := item.(float64)
		if !ok {
			return nil, false
		}
		result[i] = number
	}
	return result, true
}

func buildTools() []tool {
	return []tool{
		{
			Name:        "unity_summarize",
			Description: "Compact overview of a Unity scene/prefab/asset (object count, component types).",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"}},"required":["namespace","file"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				r, code := svc.Summarize(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_validate",
			Description: "Run the fileID graph integrity check on a file (read-only). OK/WARN/ERROR.",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"}},"required":["namespace","file"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				r, code := svc.Validate(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_refs",
			Description: "Extract PPtr/GUID reference evidence from a file (read-only).",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"}},"required":["namespace","file"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				r, code := svc.Refs(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_query",
			Description: "Resolve objects in a file by name, fileID, or component type. Returns FOUND id=...",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"},"name":{"type":"string"},"id":{"type":"integer"},"type":{"type":"string"}},"required":["namespace","file"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				args := app.QueryArgs{}
				if name := strArg(a, "name"); name != "" {
					args.HasName, args.Name = true, name
				}
				if typ := strArg(a, "type"); typ != "" {
					args.HasType, args.Type = true, typ
				}
				if id, ok := int64Arg(a, "id"); ok {
					args.HasID, args.ID = true, id
				}
				r, code := svc.Query(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false, args)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_get",
			Description: "Read a single field value from an object by fileID.",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"},"id":{"type":"integer"},"component":{"type":"string"},"field":{"type":"string"}},"required":["namespace","file","field"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				args := app.GetArgs{Component: strArg(a, "component"), Field: strArg(a, "field")}
				if id, ok := int64Arg(a, "id"); ok {
					args.HasID, args.ID = true, id
				}
				r, code := svc.Get(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false, args)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_deps",
			Description: "List external asset dependencies of a file, resolved to paths under --project.",
			InputSchema: schema(`{"type":"object","properties":{"namespace":{"type":"string","enum":["scene","prefab","asset"]},"file":{"type":"string"},"project":{"type":"string"}},"required":["namespace","file","project"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				r, code := svc.Deps(strArg(a, "namespace"), strArg(a, "file"), core.ViewCompact, false, app.DepsArgs{Project: strArg(a, "project")})
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_impact",
			Description: "Scan which scenes and prefabs reference a prefab (blast radius).",
			InputSchema: schema(`{"type":"object","properties":{"file":{"type":"string"},"project":{"type":"string"}},"required":["file","project"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				r, code := svc.Impact("prefab", strArg(a, "file"), core.ViewCompact, false, app.ImpactArgs{Project: strArg(a, "project")})
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_spatial_check",
			Description: "Prove compound-OBB overlap and all reviewed surface contacts for a proposed prefab transform. Use contact_surfaces for simultaneous requirements. Manifest v1 returns UNKNOWN NEED_GEOMETRY_V2.",
			InputSchema: schema(`{"type":"object","properties":{"file":{"type":"string"},"manifest":{"type":"string"},"prefab":{"type":"string"},"position":{"type":"array","items":{"type":"number"},"minItems":3,"maxItems":3},"rotation":{"type":"array","items":{"type":"number"},"minItems":4,"maxItems":4},"surface_id":{"type":"string"},"contact":{"type":"string","enum":["floor-supported","wall-backed","wall-mounted","ceiling-mounted"]},"contact_surfaces":{"type":"object","minProperties":1,"additionalProperties":{"type":"string"}}},"required":["file","manifest","prefab","position","rotation"],"oneOf":[{"required":["surface_id","contact"]},{"required":["contact_surfaces"]}]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				position, positionOK := floatArrayArg(a, "position", 3)
				rotation, rotationOK := floatArrayArg(a, "rotation", 4)
				if !positionOK || !rotationOK {
					return "ERROR INVALID_ARGUMENT position must have 3 numbers and rotation must have 4 numbers", true
				}
				contactSurfaces, hasContactSurfaces, contactSurfacesOK := contactSurfacesArg(a, "contact_surfaces")
				legacyContact := strArg(a, "surface_id") != "" || strArg(a, "contact") != ""
				if !contactSurfacesOK || hasContactSurfaces == legacyContact {
					return "ERROR INVALID_ARGUMENT provide exactly one of contact_surfaces or surface_id/contact", true
				}
				if legacyContact && (strArg(a, "surface_id") == "" || strArg(a, "contact") == "") {
					return "ERROR INVALID_ARGUMENT surface_id and contact must be provided together", true
				}
				args := app.CheckArgs{
					Manifest:        strArg(a, "manifest"),
					Prefab:          strArg(a, "prefab"),
					HasPosition:     true,
					Position:        [3]float64{position[0], position[1], position[2]},
					HasRotation:     true,
					Rotation:        [4]float64{rotation[0], rotation[1], rotation[2], rotation[3]},
					SurfaceID:       strArg(a, "surface_id"),
					Contact:         strArg(a, "contact"),
					ContactSurfaces: contactSurfaces,
				}
				r, code := svc.Check("scene", strArg(a, "file"), core.ViewCompact, false, args)
				return r.Body, code != 0
			},
		},
		{
			Name:        "unity_suggest_wall",
			Description: "Return deterministic read-only wall-aligned candidate transforms from a reviewed Spatial Manifest v2 surface.",
			InputSchema: schema(`{"type":"object","properties":{"file":{"type":"string"},"manifest":{"type":"string"},"prefab":{"type":"string"},"surface_id":{"type":"string"},"contact":{"type":"string","enum":["wall-backed","wall-mounted"]},"count":{"type":"integer","minimum":1,"maximum":4}},"required":["file","manifest","prefab","surface_id"]}`),
			handler: func(svc *app.Service, a map[string]any) (string, bool) {
				count := 4
				if _, present := a["count"]; present {
					value, ok := int64Arg(a, "count")
					if !ok {
						return "ERROR INVALID_ARGUMENT count must be an integer between 1 and 4", true
					}
					count = int(value)
				}
				if count < 1 || count > 4 {
					return "ERROR INVALID_ARGUMENT count must be between 1 and 4", true
				}
				r, code := svc.Suggest("scene", strArg(a, "file"), core.ViewCompact, false, app.SuggestArgs{
					Manifest:  strArg(a, "manifest"),
					Prefab:    strArg(a, "prefab"),
					SurfaceID: strArg(a, "surface_id"),
					Contact:   strArg(a, "contact"),
					Align:     "wall",
					Count:     count,
				})
				return r.Body, code != 0
			},
		},
	}
}

func contactSurfacesArg(args map[string]any, key string) (string, bool, bool) {
	raw, exists := args[key]
	if !exists {
		return "", false, true
	}
	values, ok := raw.(map[string]any)
	if !ok || len(values) == 0 {
		return "", true, false
	}
	keys := make([]string, 0, len(values))
	normalized := make(map[string]string, len(values))
	for rawRequirementID, rawSurfaceID := range values {
		surfaceID, ok := rawSurfaceID.(string)
		requirementID := strings.TrimSpace(rawRequirementID)
		surfaceID = strings.TrimSpace(surfaceID)
		if !ok || requirementID == "" || surfaceID == "" || strings.ContainsAny(requirementID, "=,") || strings.Contains(surfaceID, ",") {
			return "", true, false
		}
		if _, duplicate := normalized[requirementID]; duplicate {
			return "", true, false
		}
		normalized[requirementID] = surfaceID
		keys = append(keys, requirementID)
	}
	sort.Strings(keys)
	items := make([]string, 0, len(keys))
	for _, requirementID := range keys {
		items = append(items, requirementID+"="+normalized[requirementID])
	}
	return strings.Join(items, ","), true, true
}

func schema(s string) json.RawMessage {
	// Validate at startup so a typo fails loudly rather than shipping bad schema.
	if !json.Valid([]byte(s)) {
		panic(fmt.Sprintf("invalid tool inputSchema: %s", s))
	}
	return json.RawMessage(s)
}
