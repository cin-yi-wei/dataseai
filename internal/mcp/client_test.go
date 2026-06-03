package mcp

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

// fakeMCP is a tiny shell script that loops reading JSON lines and emits a
// canned response. Used to exercise the JSON-RPC framing + handshake without
// pulling in a real MCP server binary.
//
// The script:
//   - on `initialize` → reply ok
//   - on `tools/call` for add_connection → reply ok
//   - swallows everything else
func fakeMCPCmd(t *testing.T) (string, []string) {
	t.Helper()
	// `python3 -u` works on most dev boxes and the CI image; if absent the test
	// will skip itself.
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for fake MCP fixture")
	}
	const script = `
import sys, json
def emit(obj):
    sys.stdout.write(json.dumps(obj)+"\n")
    sys.stdout.flush()
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
    except Exception:
        continue
    rpc_id = msg.get("id")
    method = msg.get("method")
    if rpc_id is None:
        # notification, swallow
        continue
    if method == "initialize":
        emit({"jsonrpc":"2.0","id":rpc_id,"result":{"protocolVersion":"2024-11-05","capabilities":{}}})
    elif method == "tools/call":
        name = (msg.get("params") or {}).get("name","")
        emit({"jsonrpc":"2.0","id":rpc_id,"result":{"content":[{"type":"text","text":"ok:"+name}]}})
    else:
        emit({"jsonrpc":"2.0","id":rpc_id,"error":{"code":-32601,"message":"method not found"}})
`
	return "python3", []string{"-u", "-c", script}
}

func TestClient_HandshakeAndCallTool(t *testing.T) {
	cmd, args := fakeMCPCmd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Spawn(ctx, cmd, args, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer c.Close()
	out, err := c.CallTool(ctx, "add_connection", map[string]any{"dsn_name": "x"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out != "ok:add_connection" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClient_ToolErrorMaps(t *testing.T) {
	cmd, args := fakeMCPCmd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Spawn(ctx, cmd, args, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer c.Close()
	// Method we didn't script — server returns JSON-RPC error.
	_, err = c.call(ctx, "tools/list", nil)
	if err == nil {
		t.Fatal("expected error from unscripted method")
	}
}

func TestRequestEncoding(t *testing.T) {
	// Sanity: request marshals with the fields we expect.
	r := request{JSONRPC: "2.0", ID: 42, Method: "tools/call", Params: map[string]any{"name": "x"}}
	bs, _ := json.Marshal(r)
	var got map[string]any
	_ = json.Unmarshal(bs, &got)
	if got["jsonrpc"] != "2.0" || got["method"] != "tools/call" {
		t.Fatalf("bad marshal: %s", bs)
	}
}
