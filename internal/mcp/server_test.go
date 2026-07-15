package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/app"
)

// run feeds the given JSON-RPC request lines through Serve and returns the
// decoded responses (one per line of output).
func run(t *testing.T, requests ...string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out bytes.Buffer
	if err := Serve(app.New(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("decode response %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestInitializeReportsProtocolAndServerInfo(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	result, ok := resps[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %+v", resps[0])
	}
	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion mismatch: %v", result["protocolVersion"])
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "unity-ctx" {
		t.Fatalf("serverInfo.name mismatch: %v", info)
	}
}

func TestNotificationGetsNoResponse(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if len(resps) != 0 {
		t.Fatalf("notification must not produce a response, got %d", len(resps))
	}
}

func TestToolsListExposesReadOnlyTools(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result := resps[0]["result"].(map[string]any)
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) < 5 {
		t.Fatalf("expected several tools, got %v", result["tools"])
	}
	names := map[string]bool{}
	for _, tRaw := range tools {
		tm := tRaw.(map[string]any)
		names[tm["name"].(string)] = true
		if _, hasSchema := tm["inputSchema"]; !hasSchema {
			t.Fatalf("tool %v missing inputSchema", tm["name"])
		}
	}
	for _, want := range []string{"unity_summarize", "unity_validate", "unity_refs", "unity_deps"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
}

func TestSuggestWallSchemaAndHandlerCapCountAtFour(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := resps[0]["result"].(map[string]any)["tools"].([]any)
	var countSchema map[string]any
	for _, raw := range tools {
		tool := raw.(map[string]any)
		if tool["name"] == "unity_suggest_wall" {
			properties := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
			countSchema = properties["count"].(map[string]any)
			break
		}
	}
	if countSchema == nil || countSchema["maximum"] != float64(4) {
		t.Fatalf("unity_suggest_wall count schema = %#v, want maximum 4", countSchema)
	}

	for _, rawCount := range []string{"5", "4.9", `"5"`} {
		request := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"unity_suggest_wall","arguments":{"file":"room.unity","manifest":"room.json","prefab":"chair.prefab","surface_id":"wall","count":` + rawCount + `}}}`
		resps = run(t, request)
		result := resps[0]["result"].(map[string]any)
		if result["isError"] != true {
			t.Fatalf("count=%s should fail, got %#v", rawCount, result)
		}
		content := result["content"].([]any)
		if text := content[0].(map[string]any)["text"].(string); !strings.Contains(text, "between 1 and 4") {
			t.Fatalf("count=%s unexpected error %q", rawCount, text)
		}
	}
}

func TestSpatialCheckSchemaAndHandlerAcceptStableMultiContactMapping(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := resps[0]["result"].(map[string]any)["tools"].([]any)
	var schema map[string]any
	for _, raw := range tools {
		tool := raw.(map[string]any)
		if tool["name"] == "unity_spatial_check" {
			schema = tool["inputSchema"].(map[string]any)
			break
		}
	}
	properties := schema["properties"].(map[string]any)
	contactSurfaces := properties["contact_surfaces"].(map[string]any)
	if contactSurfaces["type"] != "object" || contactSurfaces["minProperties"] != float64(1) {
		t.Fatalf("contact_surfaces schema = %#v", contactSurfaces)
	}
	if _, ok := schema["oneOf"].([]any); !ok {
		t.Fatalf("spatial check schema must require exactly one contact input form: %#v", schema)
	}

	value, present, ok := contactSurfacesArg(map[string]any{"contact_surfaces": map[string]any{
		"wall": "wall-north", "floor": "floor-main",
	}}, "contact_surfaces")
	if !present || !ok || value != "floor=floor-main,wall=wall-north" {
		t.Fatalf("stable mapping = %q present=%v ok=%v", value, present, ok)
	}

	request := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"unity_spatial_check","arguments":{"file":"missing.unity","manifest":"missing.json","prefab":"asset.prefab","position":[0,0,0],"rotation":[0,0,0,1],"surface_id":"wall","contact":"wall-backed","contact_surfaces":{"wall":"wall"}}}}`
	resps = run(t, request)
	result := resps[0]["result"].(map[string]any)
	content := result["content"].([]any)
	if text := content[0].(map[string]any)["text"].(string); !strings.Contains(text, "provide exactly one") {
		t.Fatalf("mutually exclusive contact forms were not rejected: %q", text)
	}

	request = `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"unity_spatial_check","arguments":{"file":"missing.unity","manifest":"missing.json","prefab":"asset.prefab","position":[0,0,0],"rotation":[0,0,0,1]}}}`
	resps = run(t, request)
	result = resps[0]["result"].(map[string]any)
	content = result["content"].([]any)
	if text := content[0].(map[string]any)["text"].(string); !strings.Contains(text, "provide exactly one") {
		t.Fatalf("omitted contact mapping was not rejected before geometry access: %q", text)
	}
}

func TestToolsCallRunsValidate(t *testing.T) {
	req := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"unity_validate","arguments":{"namespace":"asset","file":"../../testdata/assets/enemy_config.asset"}}}`
	resps := run(t, req)
	result, ok := resps[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %+v", resps[0])
	}
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.HasPrefix(text, "OK validate ") {
		t.Fatalf("unexpected tool text: %q", text)
	}
}

func TestToolsCallBrokenFileIsError(t *testing.T) {
	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"unity_validate","arguments":{"namespace":"scene","file":"../../testdata/broken/duplicate_fileid.unity"}}}`
	resps := run(t, req)
	result := resps[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError=true for broken file, got %v", result["isError"])
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":5,"method":"bogus/method"}`)
	errObj, ok := resps[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error, got %+v", resps[0])
	}
	if errObj["code"].(float64) != -32601 {
		t.Fatalf("expected -32601, got %v", errObj["code"])
	}
}

func TestUnknownToolReturnsError(t *testing.T) {
	resps := run(t, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if _, ok := resps[0]["error"].(map[string]any); !ok {
		t.Fatalf("expected error for unknown tool, got %+v", resps[0])
	}
}
