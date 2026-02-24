package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type OpenRouterClient struct {
	client *http.Client
	apiKey string

	modelsMu    sync.RWMutex
	modelsCache []Model
	modelsCachedAt time.Time
}

func NewOpenRouterClient(client *http.Client, apiKey string) *OpenRouterClient {
	return &OpenRouterClient{client: client, apiKey: apiKey}
}

func (c *OpenRouterClient) SetAPIKey(key string) {
	c.apiKey = key
}

// -- Request/Response types --

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []Message     `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
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
		Message Message `json:"message"`
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
	req.Stream = true
	if req.Temperature == 0 {
		req.Temperature = 0.8
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 2000
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost:9876")
	httpReq.Header.Set("X-Title", "Zarr")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(b))
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	toolCallsMap := map[int]*ToolCall{} // accumulate tool calls by index
	var finishReason string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

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

	// Build result message
	msg := &Message{
		Role:    "assistant",
		Content: fullContent.String(),
	}

	if len(toolCallsMap) > 0 {
		for i := 0; i < len(toolCallsMap); i++ {
			if tc, ok := toolCallsMap[i]; ok {
				msg.ToolCalls = append(msg.ToolCalls, *tc)
			}
		}
		onEvent(StreamEvent{Type: "tool_calls", ToolCalls: msg.ToolCalls})
	}

	onEvent(StreamEvent{Type: "done", FinishReason: finishReason})

	return msg, nil
}

// Chat makes a non-streaming chat completion (for summarization, title generation).
func (c *OpenRouterClient) Chat(req ChatRequest) (*Message, error) {
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

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost:9876")
	httpReq.Header.Set("X-Title", "Zarr")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &result.Choices[0].Message, nil
}

// GetModels returns available models, cached for 1 hour.
func (c *OpenRouterClient) GetModels() ([]Model, error) {
	c.modelsMu.RLock()
	if c.modelsCache != nil && time.Since(c.modelsCachedAt) < time.Hour {
		defer c.modelsMu.RUnlock()
		return c.modelsCache, nil
	}
	c.modelsMu.RUnlock()

	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
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
	models, err := c.GetModels()
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
