package mcpclient

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ebarron/netapp-chat-service/llm"
)

func collisionRouter() *Router {
	r := NewRouter(nil)
	r.servers["ontap-mcp"] = &serverConn{
		cfg: ServerConfig{Name: "ontap-mcp"},
		tools: []*mcp.Tool{
			{Name: "delete_volume", InputSchema: objSchema()},
			{Name: "list_volumes", InputSchema: objSchema()},
		},
	}
	r.servers["other-mcp"] = &serverConn{
		cfg: ServerConfig{Name: "other-mcp"},
		tools: []*mcp.Tool{
			{Name: "delete_volume", InputSchema: objSchema(),
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			{Name: "search", InputSchema: objSchema()},
		},
	}
	return r
}

// TestRebuildToolIndexRejectsCollisions verifies that a tool name published
// by two servers is owned by neither, on every rebuild, while each server's
// unique tools stay routed to it.
func TestRebuildToolIndexRejectsCollisions(t *testing.T) {
	r := collisionRouter()
	want := map[string]string{"list_volumes": "ontap-mcp", "search": "other-mcp"}
	for i := 0; i < 50; i++ {
		r.rebuildToolIndex()
		if got := r.ToolMap(); !reflect.DeepEqual(got, want) {
			t.Fatalf("rebuild %d: toolMap = %v, want %v", i, got, want)
		}
		var names []string
		for _, d := range r.Tools() {
			names = append(names, d.Name)
		}
		if !reflect.DeepEqual(names, []string{"list_volumes", "search"}) {
			t.Fatalf("rebuild %d: tool defs = %v", i, names)
		}
	}
	if _, _, ok := r.ResolveTool("delete_volume"); ok {
		t.Error("colliding tool delete_volume should not resolve")
	}

	// Once the collision is gone the remaining publisher owns the name.
	delete(r.servers, "other-mcp")
	r.rebuildToolIndex()
	if srv, _, ok := r.ResolveTool("delete_volume"); !ok || srv != "ontap-mcp" {
		t.Errorf("ResolveTool(delete_volume) = %q, %v; want ontap-mcp", srv, ok)
	}
}

// TestCallToolOnRejectsOtherOwner verifies a call bound to one server is not
// sent when the name is owned by another server, or by none.
func TestCallToolOnRejectsOtherOwner(t *testing.T) {
	r := collisionRouter()
	r.rebuildToolIndex()
	ctx := context.Background()

	_, err := r.CallToolOn(ctx, "other-mcp", llm.ToolCall{Name: "list_volumes"})
	if err == nil || !strings.Contains(err.Error(), "not served by") {
		t.Errorf("CallToolOn(other-mcp, list_volumes) err = %v, want not served by", err)
	}
	_, err = r.CallToolOn(ctx, "ontap-mcp", llm.ToolCall{Name: "delete_volume"})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("CallToolOn(ontap-mcp, delete_volume) err = %v, want unknown tool", err)
	}
	// The owner without a live session fails cleanly instead of panicking.
	_, err = r.CallToolOn(ctx, "ontap-mcp", llm.ToolCall{Name: "list_volumes"})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("CallToolOn(ontap-mcp, list_volumes) err = %v, want not connected", err)
	}
}
