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
