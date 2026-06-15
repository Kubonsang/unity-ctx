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

	"unity-ctx/internal/app"
	"unity-ctx/internal/core"
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
			"serverInfo":      map[string]any{"name": "unity-ctx", "version": "0.6.0"},
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
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
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
	}
}

func schema(s string) json.RawMessage {
	// Validate at startup so a typo fails loudly rather than shipping bad schema.
	if !json.Valid([]byte(s)) {
		panic(fmt.Sprintf("invalid tool inputSchema: %s", s))
	}
	return json.RawMessage(s)
}
