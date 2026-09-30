package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// nonJSONContentTypes are the Content-Types a cross-site page can send without
// a CORS preflight (plus a missing header), i.e. the forgeable ones.
var nonJSONContentTypes = []string{
	"",
	"text/plain",
	"text/plain;charset=UTF-8",
	"application/x-www-form-urlencoded",
	"multipart/form-data; boundary=x",
}

// TestStateChangingEndpointsRejectNonJSON verifies a forged cross-site
// "simple request" (JSON body sent as text/plain or a form) is refused before
// any state is touched: capabilities are not changed and no chat turn runs.
func TestStateChangingEndpointsRejectNonJSON(t *testing.T) {
	for _, ct := range nonJSONContentTypes {
		t.Run("capabilities/"+ct, func(t *testing.T) {
			srv := newTestServer(t, []llm.ToolDef{mcpclient.MockReadOnlyTool("t1", "ro")})
			srv.deps.Capabilities[0].State = capability.StateOff

			req := httptest.NewRequest(http.MethodPost, "/chat/capabilities",
				strings.NewReader(`{"capabilities":{"harvest":"allow"}}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusUnsupportedMediaType {
				t.Errorf("status = %d, want 415", w.Code)
			}
			if got := srv.deps.Capabilities[0].State; got != capability.StateOff {
				t.Errorf("capability state mutated by forged request: %s", got)
			}
		})

		t.Run("message/"+ct, func(t *testing.T) {
			srv, provider := limitsServer()
			req := httptest.NewRequest(http.MethodPost, "/chat/message",
				strings.NewReader(`{"message":"delete volume X","mode":"read-write"}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusUnsupportedMediaType {
				t.Errorf("status = %d, want 415", w.Code)
			}
			if len(provider.Calls) != 0 {
				t.Error("forged request reached the LLM provider")
			}
			if n := srv.deps.Sessions.Count(); n != 0 {
				t.Errorf("forged request created %d session(s)", n)
			}
		})
	}

	// Control endpoints: the session, pending approval and active turn they
	// act on must all survive a forged request.
	const sessionID, approvalID = "csrf-session", "csrf-approval"
	srv, _ := limitsServer()
	srv.deps.Sessions.GetOrCreate(sessionID)
	approval := &PendingApproval{ID: approvalID, resultCh: make(chan bool, 1)}
	pendingApprovals.Store(approvalID, approval)
	defer pendingApprovals.Delete(approvalID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	activeContexts.Store(sessionID, cancel)
	defer activeContexts.Delete(sessionID)

	for _, ct := range nonJSONContentTypes {
		for _, rt := range []struct{ method, path string }{
			{http.MethodDelete, "/chat/session"},
			{http.MethodPost, "/chat/approve"},
			{http.MethodPost, "/chat/deny"},
			{http.MethodPost, "/chat/stop"},
		} {
			t.Run(rt.path+"/"+ct, func(t *testing.T) {
				req := httptest.NewRequest(rt.method, rt.path,
					strings.NewReader(`{"session_id":"`+sessionID+`","approval_id":"`+approvalID+`"}`))
				if ct != "" {
					req.Header.Set("Content-Type", ct)
				}
				w := httptest.NewRecorder()
				srv.Handler().ServeHTTP(w, req)
				if w.Code != http.StatusUnsupportedMediaType {
					t.Errorf("%s %s status = %d, want 415", rt.method, rt.path, w.Code)
				}
			})
		}
	}

	if srv.deps.Sessions.Get(sessionID) == nil {
		t.Error("forged request deleted the session")
	}
	if _, ok := pendingApprovals.Load(approvalID); !ok || len(approval.resultCh) != 0 {
		t.Error("forged request resolved the pending approval")
	}
	if _, ok := activeContexts.Load(sessionID); !ok || ctx.Err() != nil {
		t.Error("forged request stopped the active chat")
	}
}

// TestStateChangingEndpointsAcceptJSON verifies the shipped client's requests
// (Content-Type: application/json, optionally with parameters) still succeed.
func TestStateChangingEndpointsAcceptJSON(t *testing.T) {
	for _, ct := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		t.Run(ct, func(t *testing.T) {
			srv := newTestServer(t, []llm.ToolDef{mcpclient.MockReadOnlyTool("t1", "ro")})
			srv.deps.Capabilities[0].State = capability.StateOff

			req := httptest.NewRequest(http.MethodPost, "/chat/capabilities",
				strings.NewReader(`{"capabilities":{"harvest":"allow"}}`))
			req.Header.Set("Content-Type", ct)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
			if got := srv.deps.Capabilities[0].State; got != capability.StateAllow {
				t.Errorf("state = %s, want allow", got)
			}
		})
	}

	srv, provider := limitsServer()
	req := httptest.NewRequest(http.MethodPost, "/chat/message", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/chat/message status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(provider.Calls) == 0 {
		t.Error("JSON /chat/message did not run the agent")
	}
}
