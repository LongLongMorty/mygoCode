package mcp

// Integration test for the MCP stdio client against a minimal local server
// (plain Node, no external dependencies, no network). This covers the
// Connect → ListTools → CallTool chain that the live TestContext7MCP covers
// only when a network + npm are available; it runs in the default test suite
// on any machine with Node installed.
//
// Note: the real context7 server additionally requires Node >= 20 (its undici
// dependency needs the global File API).

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const miniMCPServer = `const readline = require('readline');
const rl = readline.createInterface({ input: process.stdin });
rl.on('line', (line) => {
  const t = line.trim();
  if (!t) return;
  try { handle(JSON.parse(t)); } catch (e) { /* ignore malformed */ }
});
function send(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}
function handle(msg) {
  if (msg.id === undefined || msg.id === null) return; // notification
  switch (msg.method) {
    case 'initialize':
      send({ jsonrpc: '2.0', id: msg.id, result: {
        protocolVersion: msg.params && msg.params.protocolVersion,
        capabilities: { tools: {} },
        serverInfo: { name: 'minitest-mcp', version: '1.0.0' }
      }});
      break;
    case 'notifications/initialized': send({ jsonrpc: '2.0', id: msg.id, result: {} }); break;
    case 'ping': send({ jsonrpc: '2.0', id: msg.id, result: {} }); break;
    case 'tools/list':
      send({ jsonrpc: '2.0', id: msg.id, result: { tools: [{
        name: 'echo', description: 'echo back text',
        inputSchema: { type: 'object', properties: { text: { type: 'string' } } }
      }]}});
      break;
    case 'tools/call':
      const text = (msg.params && msg.params.arguments && msg.params.arguments.text) || '';
      send({ jsonrpc: '2.0', id: msg.id, result: { content: [{ type: 'text', text: 'echo: ' + text }], isError: false } });
      break;
    default:
      send({ jsonrpc: '2.0', id: msg.id, error: { code: -32601, message: 'method not found: ' + msg.method } });
  }
}
`

func TestLocalStdioMCPClient(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed; skipping local MCP stdio test")
	}
	script := filepath.Join(t.TempDir(), "minimcp.js")
	if err := os.WriteFile(script, []byte(miniMCPServer), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(ServerConfig{Name: "minitest", Command: "node", Args: []string{script}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want [echo]", tools)
	}

	text, isErr, err := client.CallTool(ctx, "echo", map[string]any{"text": "hello-mcp"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if isErr || text != "echo: hello-mcp" {
		t.Fatalf("CallTool = (%q, %v), want (\"echo: hello-mcp\", false)", text, isErr)
	}
}
