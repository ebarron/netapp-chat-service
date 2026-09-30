package mcpclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ebarron/netapp-chat-service/llm"
)

func objSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSanitizeToolsCapsPerServer(t *testing.T) {
	var tools []*mcp.Tool
	for i := 0; i < 200; i++ {
		tools = append(tools, &mcp.Tool{Name: fmt.Sprintf("t_%d", i), InputSchema: objSchema()})
	}
	got := sanitizeTools(discardLogger(), "big", tools)
	if len(got) != MaxToolsPerServer {
		t.Fatalf("kept %d tools, want %d", len(got), MaxToolsPerServer)
	}
	if got[0].Name != "t_0" || got[len(got)-1].Name != fmt.Sprintf("t_%d", MaxToolsPerServer-1) {
		t.Errorf("expected the first %d tools to be kept in order", MaxToolsPerServer)
	}
}

func TestSanitizeToolsRejectsInvalid(t *testing.T) {
	longDesc := strings.Repeat("x", maxToolDescriptionBytes+100)
	bigSchema := objSchema()
	bigSchema["description"] = strings.Repeat("y", maxToolSchemaBytes)
	tools := []*mcp.Tool{
		nil,
		{Name: "good", InputSchema: objSchema()},
		{Name: "good", InputSchema: objSchema()}, // duplicate within server
		{Name: "", InputSchema: objSchema()},
		{Name: "has space", InputSchema: objSchema()},
		{Name: "dotted.name", InputSchema: objSchema()},
		{Name: strings.Repeat("a", 65), InputSchema: objSchema()},
		{Name: "no_schema"},
		{Name: "string_schema", InputSchema: map[string]any{"type": "string"}},
		{Name: "array_schema", InputSchema: []any{1, 2}},
		{Name: "big_schema", InputSchema: bigSchema},
		{Name: "bad_props", InputSchema: map[string]any{"type": "object", "properties": []any{}}},
		{Name: "bad_prop_val", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"x": "bogus"}}},
		{Name: "bad_required", InputSchema: map[string]any{"type": "object", "required": "x"}},
		{Name: "bad_required_item", InputSchema: map[string]any{"type": "object", "required": []any{1}}},
		{Name: "long_desc", Description: longDesc, InputSchema: objSchema()},
		{Name: "ok-dash_1", InputSchema: objSchema()},
	}
	got := sanitizeTools(discardLogger(), "srv", tools)
	var names []string
	for _, tl := range got {
		names = append(names, tl.Name)
	}
	want := []string{"good", "long_desc", "ok-dash_1"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("kept %v, want %v", names, want)
	}
	if len(got[1].Description) > maxToolDescriptionBytes {
		t.Errorf("description not truncated: %d bytes", len(got[1].Description))
	}
	if len(tools[15].Description) != len(longDesc) {
		t.Error("sanitizeTools must not mutate the input tool")
	}
}

func TestSanitizeToolsAcceptsTypicalSchema(t *testing.T) {
	tools := []*mcp.Tool{{Name: "get_volume", InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}, "any": true},
		"required":   []any{"name"},
	}}, {Name: "null_props", InputSchema: map[string]any{"type": "object", "properties": nil}}}
	if got := sanitizeTools(discardLogger(), "srv", tools); len(got) != 2 {
		t.Fatalf("kept %d tools, want 2", len(got))
	}
}

func TestSanitizeToolsCapsBytesPerServer(t *testing.T) {
	desc := strings.Repeat("d", maxToolDescriptionBytes)
	var tools []*mcp.Tool
	for i := 0; i < MaxToolsPerServer; i++ {
		tools = append(tools, &mcp.Tool{Name: fmt.Sprintf("t_%d", i), Description: desc, InputSchema: objSchema()})
	}
	got := sanitizeTools(discardLogger(), "big", tools)
	total := 0
	for _, tl := range got {
		b, _ := validateToolSchema(tl.InputSchema)
		total += len(tl.Name) + len(tl.Description) + b
	}
	if total > MaxToolBytesPerServer {
		t.Fatalf("kept %d bytes, max %d", total, MaxToolBytesPerServer)
	}
	if len(got) == 0 || len(got) == len(tools) {
		t.Fatalf("kept %d of %d tools; want a byte-capped prefix", len(got), len(tools))
	}
}

// TestConnectCapsOversizedServer connects to an in-process MCP server that
// publishes 200 tools plus one with an invalid name, and checks the router
// exposes at most MaxToolsPerServer valid tools from it while another
// server's tools remain callable.
func TestConnectCapsOversizedServer(t *testing.T) {
	handler := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	}
	newStub := func(names ...string) *httptest.Server {
		srv := mcp.NewServer(&mcp.Implementation{Name: "stub", Version: "1"}, &mcp.ServerOptions{Logger: discardLogger()})
		for _, n := range names {
			srv.AddTool(&mcp.Tool{
				Name:        n,
				InputSchema: objSchema(),
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			}, handler)
		}
		ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
		t.Cleanup(ts.Close)
		return ts
	}
	var hostileNames []string
	for i := 0; i < 200; i++ {
		hostileNames = append(hostileNames, fmt.Sprintf("hostile_%03d", i))
	}
	hostileNames = append(hostileNames, "bad name!")
	hostile := newStub(hostileNames...)
	good := newStub("get_volume")

	r := NewRouter(discardLogger())
	t.Cleanup(func() { _ = r.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.Connect(ctx, ServerConfig{Name: "hostile", Endpoint: hostile.URL}); err != nil {
		t.Fatalf("connect hostile: %v", err)
	}
	if err := r.Connect(ctx, ServerConfig{Name: "good", Endpoint: good.URL}); err != nil {
		t.Fatalf("connect good: %v", err)
	}

	perServer := map[string]int{}
	tm := r.ToolMap()
	for _, td := range r.Tools() {
		if !validToolNameRe.MatchString(td.Name) {
			t.Errorf("invalid tool name published: %q", td.Name)
		}
		perServer[tm[td.Name]]++
	}
	if perServer["hostile"] != MaxToolsPerServer {
		t.Errorf("hostile server exposes %d tools, want %d", perServer["hostile"], MaxToolsPerServer)
	}
	if perServer["good"] != 1 {
		t.Errorf("good server exposes %d tools, want 1", perServer["good"])
	}
	out, err := r.CallTool(ctx, llm.ToolCall{Name: "get_volume"})
	if err != nil || out != "ok" {
		t.Errorf("CallTool(get_volume) = %q, %v; want ok", out, err)
	}
}
