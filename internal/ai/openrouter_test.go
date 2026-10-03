package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func streamClient(body io.ReadCloser) *OpenRouterClient {
	return NewOpenRouterClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body}, nil
	})}, "test")
}

func TestStreamPreservesReasoningAndSparseToolCalls(t *testing.T) {
	body := strings.Join([]string{
		`data:{"choices":[{"delta":{"reasoning":"Thinking ","reasoning_details":[{"type":"reasoning.text","text":"Thinking ","index":0,"signature":null}]}}]}`,
		`data: {"choices":[{"delta":{"reasoning":"done","reasoning_details":[{"type":"reasoning.text","text":"done","index":0,"signature":"opaque","extra":"preserved"}],"tool_calls":[{"index":3,"id":"call_1","function":{"name":"search","arguments":"{\"q\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":3,"function":{"arguments":"\"film\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")
	var events []StreamEvent
	msg, err := streamClient(io.NopCloser(strings.NewReader(body))).ChatStream(ChatRequest{}, func(e StreamEvent) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if msg.Reasoning != "Thinking done" || len(msg.ReasoningDetails) != 2 {
		t.Fatalf("lost reasoning: %+v", msg)
	}
	if !strings.Contains(string(msg.ReasoningDetails[1]), `"extra":"preserved"`) {
		t.Fatal("opaque fields lost")
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Arguments != `{"q":"film"}` {
		t.Fatalf("tool fragments lost: %+v", msg.ToolCalls)
	}
	if events[len(events)-1].Type != "done" {
		t.Fatal("missing done event")
	}
}

func TestStreamRejectsErrorsAndPrematureEOF(t *testing.T) {
	for _, body := range []string{
		`data: {"error":{"message":"provider unavailable"}}` + "\n",
		`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n",
		`data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n",
		`data: [DONE]` + "\n",
		`data: broken` + "\n",
	} {
		if _, err := streamClient(io.NopCloser(strings.NewReader(body))).ChatStream(ChatRequest{}, nil); err == nil {
			t.Fatalf("accepted broken response: %s", body)
		}
	}
}

func TestStreamAcceptsLargeReasoningAndPassesConfig(t *testing.T) {
	text := strings.Repeat("a", 100000)
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": text}, "finish_reason": "stop"}}})
	client := NewOpenRouterClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Reasoning == nil || req.Reasoning.Effort != "high" || req.MaxTokens < 16000 {
			t.Fatalf("incorrect reasoning budget: %+v", req)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))}, nil
	})}, "test")
	msg, err := client.ChatStream(ChatRequest{Reasoning: &ReasoningConfig{Effort: "high"}}, nil)
	if err != nil || msg.Content != text {
		t.Fatalf("large stream failed: %v", err)
	}
}

func TestStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewOpenRouterClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}, "test")
	if _, err := client.ChatStreamContext(ctx, ChatRequest{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
