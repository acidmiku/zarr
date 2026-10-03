package ai

import (
	"context"
	"encoding/json"
	"io"
	"mediaforge/internal/database"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCompressionPreservesCompleteToolTurns(t *testing.T) {
	client := NewOpenRouterClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"Earlier preferences"},"finish_reason":"stop"}]}`))}, nil
	})}, "test")
	m := NewSessionManager(nil, client)
	messages := []Message{{Role: "system"}, {Role: "user", Content: "old"}, {Role: "assistant", Content: "old"}, {Role: "user", Content: "older"}, {Role: "assistant", Content: "older"}, {Role: "user", Content: "keep this entire turn"}}
	for i := 0; i < 5; i++ {
		messages = append(messages, Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "call", Function: FunctionCall{Name: "search", Arguments: `{}`}}}}, Message{Role: "tool", ToolCallID: "call", Content: "result"})
	}
	messages = append(messages, Message{Role: "user", Content: "latest"})
	compressed, err := m.compressContext(context.Background(), 1, messages, "system", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) != 14 || compressed[2].Content != "keep this entire turn" {
		t.Fatalf("compression split a tool turn: %+v", compressed)
	}
	for i, msg := range compressed {
		if msg.Role == "tool" && (i == 0 || len(compressed[i-1].ToolCalls) == 0) {
			t.Fatal("orphaned tool result after compression")
		}
	}
}

func TestSessionReasoningRoundTripAndAtomicToolChain(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client := NewOpenRouterClient(nil, "test")
	client.modelsCache = []Model{{ID: "test", ContextLength: 100000}}
	client.modelsCachedAt = time.Now()
	m := NewSessionManager(db.DB, client)
	session, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if !m.BeginTurn(session.ID) || m.BeginTurn(session.ID) {
		t.Fatal("concurrent turn accepted")
	}
	reconfigured := m.WithClient(NewOpenRouterClient(nil, "replacement"))
	if reconfigured.BeginTurn(session.ID) {
		t.Fatal("credential reload lost the active conversation lock")
	}
	m.EndTurn(session.ID)
	if !m.BeginTurn(session.ID) {
		t.Fatal("turn lock leaked")
	}
	m.EndTurn(session.ID)
	_, err = m.SaveMessages(session.ID, []Message{
		{Role: "user", Content: "find a movie"},
		{Role: "assistant", Reasoning: "opaque thought", ReasoningDetails: []json.RawMessage{json.RawMessage(`{"type":"reasoning.encrypted","data":"opaque","index":0}`)}, ToolCalls: []ToolCall{{ID: "call", Type: "function", Function: FunctionCall{Name: "search", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "call", Content: `[]`},
	})
	if err != nil {
		t.Fatal(err)
	}
	context, err := m.BuildContext(session.ID, "system", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 4 || context[2].Reasoning != "opaque thought" || len(context[2].ReasoningDetails) != 1 || context[3].ToolCallID != "call" {
		t.Fatalf("context lost tool reasoning: %+v", context)
	}
	saved, messages, err := m.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.CreatedAt.IsZero() || messages[0].CreatedAt.IsZero() {
		t.Fatal("SQLite timestamps parsed as zero")
	}
	if _, err := m.SaveMessages(session.ID, []Message{{Role: "assistant", Content: "first"}, {Role: "invalid", Content: "second"}}); err == nil {
		t.Fatal("invalid role accepted")
	}
	_, messages, _ = m.GetSession(session.ID)
	if len(messages) != 3 {
		t.Fatal("partial message batch committed")
	}
	if err := m.DeleteSession(session.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ai_messages`).Scan(&count); err != nil || count != 0 {
		t.Fatal("session messages not cascaded")
	}
}
