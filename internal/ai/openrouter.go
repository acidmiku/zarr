package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type OpenRouterClient struct {
	client *http.Client
	apiKey string
	keyMu  sync.RWMutex

	modelsMu       sync.RWMutex
	modelsCache    []Model
	modelsCachedAt time.Time
}

func NewOpenRouterClient(client *http.Client, apiKey string) *OpenRouterClient {
	if client == nil {
		client = &http.Client{}
	}
	// Preserve the transport (including proxy reloads) without inheriting short
	// metadata timeouts: high reasoning may take minutes before the first token.
	cloned := *client
	cloned.Timeout = 5 * time.Minute
	client = &cloned
	return &OpenRouterClient{client: client, apiKey: apiKey}
}

func (c *OpenRouterClient) SetAPIKey(key string) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	c.apiKey = key
}

func (c *OpenRouterClient) key() string { c.keyMu.RLock(); defer c.keyMu.RUnlock(); return c.apiKey }

// -- Request/Response types --
type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type ChatRequest struct {
	Reasoning   *ReasoningConfig `json:"reasoning,omitempty"`
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []Tool           `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

type Message struct {
	Reasoning        string            `json:"reasoning,omitempty"`
	ReasoningDetails []json.RawMessage `json:"reasoning_details,omitempty"`
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Streaming chunk
type streamChunk struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content          string            `json:"content"`
			Reasoning        string            `json:"reasoning"`
			ReasoningContent string            `json:"reasoning_content"`
			ReasoningDetails []json.RawMessage `json:"reasoning_details"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Non-streaming response
type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type Model struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// StreamEvent represents a parsed event from the streaming response
type StreamEvent struct {
	Type         string // "text", "tool_calls", "done", "error"
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// ChatStream streams a chat completion. It calls onEvent for each parsed event.
func (c *OpenRouterClient) ChatStream(req ChatRequest, onEvent func(StreamEvent)) (*Message, error) {
	return c.ChatStreamContext(context.Background(), req, onEvent)
}

func (c *OpenRouterClient) ChatStreamContext(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) (*Message, error) {
	if onEvent == nil {
		onEvent = func(StreamEvent) {}
	}
	req.Stream = true
	if req.Temperature == 0 {
		req.Temperature = 0.8
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 16384
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.key())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost:9876")
	httpReq.Header.Set("X-Title", "Zarr")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(b))
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var fullReasoning strings.Builder
	var reasoningDetails []json.RawMessage
	sawDone := false
	var fullContent strings.Builder
	toolCallsMap := map[int]*ToolCall{} // accumulate tool calls by index
	var finishReason string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			sawDone = true
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("invalid streaming response: %w", err)
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("openrouter stream: %s", chunk.Error.Message)
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		// Preserve every opaque reasoning block in provider order, including signatures.
		// These are sent back on the assistant message after tools complete.
		reasoningDetails = append(reasoningDetails, choice.Delta.ReasoningDetails...)
		reasoning := choice.Delta.Reasoning
		if reasoning == "" {
			reasoning = choice.Delta.ReasoningContent
		}
		fullReasoning.WriteString(reasoning)
		if reasoning != "" {
			onEvent(StreamEvent{Type: "reasoning", Content: reasoning})
		}
		// Some providers only emit structured reasoning, with no plaintext alias.
		if reasoning == "" {
			for _, detail := range choice.Delta.ReasoningDetails {
				var block struct {
					Text    string `json:"text"`
					Summary string `json:"summary"`
				}
				if json.Unmarshal(detail, &block) == nil {
					if block.Text != "" {
						onEvent(StreamEvent{Type: "reasoning", Content: block.Text})
					}
					if block.Summary != "" {
						onEvent(StreamEvent{Type: "reasoning", Content: block.Summary})
					}
				}
			}
		}

		// Text content
		if choice.Delta.Content != "" {
			fullContent.WriteString(choice.Delta.Content)
			onEvent(StreamEvent{Type: "text", Content: choice.Delta.Content})
		}

		// Tool calls (accumulate incrementally)
		for _, tc := range choice.Delta.ToolCalls {
			existing, ok := toolCallsMap[tc.Index]
			if !ok {
				toolCallsMap[tc.Index] = &ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
		}

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read openrouter stream: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !sawDone && finishReason == "" {
		return nil, fmt.Errorf("openrouter stream ended before completion")
	}
	if finishReason == "error" {
		return nil, fmt.Errorf("openrouter provider failed during completion")
	}
	if finishReason == "length" {
		return nil, fmt.Errorf("AI reached its response token limit; try a shorter request")
	}
	// Build result message
	msg := &Message{
		Role:             "assistant",
		Content:          fullContent.String(),
		Reasoning:        fullReasoning.String(),
		ReasoningDetails: reasoningDetails,
	}

	if len(toolCallsMap) > 0 {
		indices := make([]int, 0, len(toolCallsMap))
		for index := range toolCallsMap {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			tc := toolCallsMap[index]
			if tc.ID == "" || tc.Function.Name == "" || !json.Valid([]byte(tc.Function.Arguments)) {
				return nil, fmt.Errorf("incomplete tool call in AI response")
			}
			msg.ToolCalls = append(msg.ToolCalls, *tc)
		}
		onEvent(StreamEvent{Type: "tool_calls", ToolCalls: msg.ToolCalls})
	}

	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil, fmt.Errorf("AI returned no answer; try again")
	}
	onEvent(StreamEvent{Type: "done", FinishReason: finishReason})

	return msg, nil
}

// Chat makes a non-streaming chat completion (for summarization, title generation).
func (c *OpenRouterClient) Chat(req ChatRequest) (*Message, error) {
	return c.ChatContext(context.Background(), req)
}

func (c *OpenRouterClient) ChatContext(ctx context.Context, req ChatRequest) (*Message, error) {
	req.Stream = false
	if req.Temperature == 0 {
		req.Temperature = 0.5
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 500
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.key())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost:9876")
	httpReq.Header.Set("X-Title", "Zarr")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	if result.Choices[0].FinishReason == "length" || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("AI returned an incomplete response")
	}
	return &result.Choices[0].Message, nil
}

// GetModels returns available models, cached for 1 hour.
func (c *OpenRouterClient) GetModels() ([]Model, error) {
	return c.GetModelsContext(context.Background())
}

func (c *OpenRouterClient) GetModelsContext(ctx context.Context) ([]Model, error) {
	c.modelsMu.RLock()
	if c.modelsCache != nil && time.Since(c.modelsCachedAt) < time.Hour {
		defer c.modelsMu.RUnlock()
		return c.modelsCache, nil
	}
	c.modelsMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("models %d: %s", resp.StatusCode, string(b))
	}

	var result modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	c.modelsMu.Lock()
	c.modelsCache = result.Data
	c.modelsCachedAt = time.Now()
	c.modelsMu.Unlock()

	return result.Data, nil
}

// GetModelContextLength returns context length for the given model ID.
func (c *OpenRouterClient) GetModelContextLength(modelID string) int {
	return c.GetModelContextLengthContext(context.Background(), modelID)
}

func (c *OpenRouterClient) GetModelContextLengthContext(ctx context.Context, modelID string) int {
	models, err := c.GetModelsContext(ctx)
	if err != nil {
		return 8192 // safe default
	}
	for _, m := range models {
		if m.ID == modelID {
			return m.ContextLength
		}
	}
	return 8192
}
