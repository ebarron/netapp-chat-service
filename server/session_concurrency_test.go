package server

import (
	"bytes"
	"context"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

type stopRaceProvider struct {
	calls   atomic.Int32
	started chan int
}

func (p *stopRaceProvider) Name() string { return "stop-race" }

func (p *stopRaceProvider) ValidateConfig(context.Context) error { return nil }

func (p *stopRaceProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (p *stopRaceProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) iter.Seq2[llm.StreamEvent, error] {
	call := int(p.calls.Add(1))
	return func(yield func(llm.StreamEvent, error) bool) {
		p.started <- call
		if call == 1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				return
			}
		}
		if !yield(llm.StreamEvent{Type: llm.EventText, Delta: "second completed"}, nil) {
			return
		}
		yield(llm.StreamEvent{Type: llm.EventDone}, nil)
	}
}

func TestStopCancelsActiveTurnInsteadOfSameSessionWaiter(t *testing.T) {
	provider := &stopRaceProvider{started: make(chan int, 2)}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	srv := New(deps)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- postSessionMessage(srv, "serialized", "first")
	}()
	if call := waitProviderCall(t, provider.started); call != 1 {
		t.Fatalf("first provider call = %d", call)
	}

	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		secondDone <- postSessionMessage(srv, "serialized", "second")
	}()
	// The second handler must be waiting on the session, not registered as the
	// active cancellation target.
	time.Sleep(20 * time.Millisecond)

	stopReq := httptest.NewRequest(http.MethodPost, "/chat/stop",
		bytes.NewBufferString(`{"session_id":"serialized"}`))
	stopReq.Header.Set("Content-Type", "application/json")
	stopRec := httptest.NewRecorder()
	srv.PostChatStop(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200", stopRec.Code)
	}

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("active turn was not cancelled")
	}
	if call := waitProviderCall(t, provider.started); call != 2 {
		t.Fatalf("resumed provider call = %d, want 2", call)
	}
	select {
	case rec := <-secondDone:
		if !strings.Contains(rec.Body.String(), "second completed") ||
			!strings.Contains(rec.Body.String(), "event: done") {
			t.Fatalf("waiting turn did not complete normally:\n%s", rec.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("same-session waiter did not resume")
	}

	messages := deps.Sessions.Get("serialized").Messages
	if len(messages) != 4 {
		t.Fatalf("history = %+v, want two complete user/assistant pairs", messages)
	}
	if messages[1].Role != llm.RoleAssistant ||
		messages[1].Content != "The previous turn was cancelled." {
		t.Fatalf("cancelled-turn checkpoint = %+v", messages[1])
	}
}

func TestDeleteCancelsAndWaitsBeforeRecreatingSameSessionID(t *testing.T) {
	provider := &stopRaceProvider{started: make(chan int, 2)}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	srv := New(deps)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- postSessionMessage(srv, "delete-active", "first")
	}()
	if call := waitProviderCall(t, provider.started); call != 1 {
		t.Fatalf("first provider call = %d", call)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/chat/session",
		bytes.NewBufferString(`{"session_id":"delete-active"}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	srv.DeleteChatSession(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", deleteRec.Code, deleteRec.Body.String())
	}
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("active turn did not finish before delete returned")
	}
	if deps.Sessions.Get("delete-active") != nil {
		t.Fatal("deleted session is still registered")
	}

	recreated := postSessionMessage(srv, "delete-active", "replacement")
	if call := waitProviderCall(t, provider.started); call != 2 {
		t.Fatalf("replacement provider call = %d, want 2", call)
	}
	if !strings.Contains(recreated.Body.String(), "second completed") ||
		!strings.Contains(recreated.Body.String(), "event: done") {
		t.Fatalf("replacement turn did not complete:\n%s", recreated.Body.String())
	}
}

func postSessionMessage(srv *Server, sessionID, message string) *httptest.ResponseRecorder {
	body := `{"session_id":"` + sessionID + `","message":"` + message + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/message", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.PostChatMessage(rec, req)
	return rec
}

func waitProviderCall(t *testing.T, started <-chan int) int {
	t.Helper()
	select {
	case call := <-started:
		return call
	case <-time.After(time.Second):
		t.Fatal("provider call did not start")
		return 0
	}
}

func TestPostChatMessageResolvesEmptySessionOnlyOnce(t *testing.T) {
	manager := session.NewManager(20)
	srv := New(&ChatDeps{
		Sessions: manager,
		Provider: &llm.MockProvider{Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("hello"),
		}},
		Router: mcpclient.NewMockRouter(nil),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := postSessionMessage(srv, "", "hello")
	if manager.Count() != 1 {
		t.Fatalf("sessions = %d, want exactly 1", manager.Count())
	}
	if !strings.Contains(rec.Body.String(), `"session_id":"`) {
		t.Fatalf("missing resolved session ID in SSE: %s", rec.Body.String())
	}
}

func TestDeleteCancelsAndClosesOldSessionBeforeSameIDRecreation(t *testing.T) {
	provider := &stopRaceProvider{started: make(chan int, 2)}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	srv := New(deps)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- postSessionMessage(srv, "deleted-session", "first")
	}()
	if call := waitProviderCall(t, provider.started); call != 1 {
		t.Fatalf("first provider call = %d", call)
	}

	waiterDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		waiterDone <- postSessionMessage(srv, "deleted-session", "stale waiter")
	}()
	time.Sleep(20 * time.Millisecond)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/chat/session",
		bytes.NewBufferString(`{"session_id":"deleted-session"}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	srv.DeleteChatSession(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", deleteRec.Code)
	}
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("delete did not cancel the active turn")
	}
	select {
	case rec := <-waiterDone:
		if !strings.Contains(rec.Body.String(), "session is no longer available") {
			t.Fatalf("stale waiter response:\n%s", rec.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("delete did not release the stale waiter")
	}
	if deps.Sessions.Get("deleted-session") != nil {
		t.Fatal("old session remains registered after delete")
	}

	replacement := postSessionMessage(srv, "deleted-session", "replacement")
	if call := waitProviderCall(t, provider.started); call != 2 {
		t.Fatalf("replacement provider call = %d, want 2", call)
	}
	if !strings.Contains(replacement.Body.String(), "second completed") {
		t.Fatalf("replacement response:\n%s", replacement.Body.String())
	}
}
