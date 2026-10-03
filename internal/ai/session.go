package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SessionManager handles AI session persistence and context management.
type SessionManager struct {
	db     *sql.DB
	openai *OpenRouterClient
	turns  *sync.Map
}

func NewSessionManager(db *sql.DB, openai *OpenRouterClient) *SessionManager {
	return &SessionManager{db: db, openai: openai, turns: &sync.Map{}}
}

// WithClient retains active conversation locks when credentials are changed or
// restored. Requests already running keep their original client snapshot.
func (m *SessionManager) WithClient(client *OpenRouterClient) *SessionManager {
	return &SessionManager{db: m.db, openai: client, turns: m.turns}
}

// BeginTurn serializes requests for one conversation without holding a database lock.
func (m *SessionManager) BeginTurn(id int) bool {
	_, busy := m.turns.LoadOrStore(id, true)
	return !busy
}
func (m *SessionManager) EndTurn(id int) { m.turns.Delete(id) }

func parseStoredTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Session represents a conversation session.
type Session struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StoredMessage represents a persisted message.
type StoredMessage struct {
	Reasoning        string            `json:"reasoning,omitempty"`
	ReasoningDetails []json.RawMessage `json:"reasoning_details,omitempty"`
	ID               int               `json:"id"`
	SessionID        int               `json:"session_id"`
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ToolCalls        string            `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
	TokenEstimate    int               `json:"token_estimate"`
	CreatedAt        time.Time         `json:"created_at"`
}

// CreateSession creates a new conversation session.
func (m *SessionManager) CreateSession() (*Session, error) {
	res, err := m.db.Exec(`INSERT INTO ai_sessions (title) VALUES (NULL)`)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	id, _ := res.LastInsertId()

	return &Session{
		ID:        int(id),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// ListSessions returns all sessions ordered by most recent.
func (m *SessionManager) ListSessions() ([]Session, error) {
	rows, err := m.db.Query(`SELECT id, COALESCE(title, ''), created_at, updated_at FROM ai_sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		var created, updated string
		if err := rows.Scan(&s.ID, &s.Title, &created, &updated); err != nil {
			continue
		}
		s.CreatedAt = parseStoredTime(created)
		s.UpdatedAt = parseStoredTime(updated)
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetSession returns a session by ID with all messages.
func (m *SessionManager) GetSession(id int) (*Session, []StoredMessage, error) {
	var s Session
	var created, updated string
	var title sql.NullString
	err := m.db.QueryRow(`SELECT id, title, created_at, updated_at FROM ai_sessions WHERE id = ?`, id).Scan(
		&s.ID, &title, &created, &updated)
	if err != nil {
		return nil, nil, fmt.Errorf("get session: %w", err)
	}
	if title.Valid {
		s.Title = title.String
	}
	s.CreatedAt = parseStoredTime(created)
	s.UpdatedAt = parseStoredTime(updated)

	rows, err := m.db.Query(`SELECT id, session_id, role, content, COALESCE(tool_calls, ''), COALESCE(tool_call_id, ''), COALESCE(token_estimate, 0), created_at, COALESCE(reasoning, ''), COALESCE(reasoning_details, '[]')
		FROM ai_messages WHERE session_id = ? ORDER BY id ASC`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get messages: %w", err)
	}
	defer rows.Close()

	var messages []StoredMessage
	for rows.Next() {
		var msg StoredMessage
		var createdStr, details string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolCallID, &msg.TokenEstimate, &createdStr, &msg.Reasoning, &details); err != nil {
			continue
		}
		msg.CreatedAt = parseStoredTime(createdStr)
		if err := json.Unmarshal([]byte(details), &msg.ReasoningDetails); err != nil {
			return nil, nil, fmt.Errorf("read reasoning: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if messages == nil {
		messages = []StoredMessage{}
	}
	return &s, messages, nil
}

// DeleteSession removes a session and all its messages.
func (m *SessionManager) DeleteSession(id int) error {
	_, err := m.db.Exec(`DELETE FROM ai_sessions WHERE id = ?`, id)
	return err
}

// SaveMessage stores a message in the database.
func (m *SessionManager) SaveMessage(sessionID int, role, content, toolCalls, toolCallID string) (*StoredMessage, error) {
	msg := Message{Role: role, Content: content, ToolCallID: toolCallID}
	if toolCalls != "" {
		if err := json.Unmarshal([]byte(toolCalls), &msg.ToolCalls); err != nil {
			return nil, err
		}
	}
	saved, err := m.SaveMessages(sessionID, []Message{msg})
	if err != nil {
		return nil, err
	}
	return &saved[0], nil
}

// SaveMessages atomically persists an assistant tool request together with all
// results, so interrupted or failed turns never leave an invalid tool chain.
func (m *SessionManager) SaveMessages(sessionID int, messages []Message) ([]StoredMessage, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	saved := make([]StoredMessage, 0, len(messages))
	for _, msg := range messages {
		tc, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return nil, err
		}
		details, err := json.Marshal(msg.ReasoningDetails)
		if err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(msg)
		tokenEst := (len(encoded) + 3) / 4
		res, err := tx.Exec(`INSERT INTO ai_messages (session_id,role,content,tool_calls,tool_call_id,token_estimate,reasoning,reasoning_details) VALUES (?,?,?,?,?,?,?,?)`, sessionID, msg.Role, msg.Content, string(tc), msg.ToolCallID, tokenEst, msg.Reasoning, string(details))
		if err != nil {
			return nil, fmt.Errorf("save message: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		saved = append(saved, StoredMessage{ID: int(id), SessionID: sessionID, Role: msg.Role, Content: msg.Content, ToolCalls: string(tc), ToolCallID: msg.ToolCallID, Reasoning: msg.Reasoning, ReasoningDetails: msg.ReasoningDetails, TokenEstimate: tokenEst, CreatedAt: time.Now()})
	}
	if _, err = tx.Exec(`UPDATE ai_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}

// BuildContext assembles messages for an API call, compressing if needed.
func (m *SessionManager) BuildContext(sessionID int, systemPrompt string, modelID string) ([]Message, error) {
	return m.BuildContextContext(context.Background(), sessionID, systemPrompt, modelID)
}

func (m *SessionManager) BuildContextContext(ctx context.Context, sessionID int, systemPrompt string, modelID string) ([]Message, error) {
	_, messages, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Start with system prompt
	apiMessages := []Message{{Role: "system", Content: systemPrompt}}

	// Convert stored messages to API format
	for _, msg := range messages {
		apiMsg := Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Reasoning:  msg.Reasoning, ReasoningDetails: msg.ReasoningDetails,
		}
		if msg.ToolCalls != "" {
			if err := json.Unmarshal([]byte(msg.ToolCalls), &apiMsg.ToolCalls); err != nil {
				return nil, fmt.Errorf("read tool calls: %w", err)
			}
		}
		apiMessages = append(apiMessages, apiMsg)
	}

	// Estimate total tokens
	totalTokens := 0
	for _, msg := range apiMessages {
		encoded, _ := json.Marshal(msg)
		totalTokens += (len(encoded) + 3) / 4
		for _, tc := range msg.ToolCalls {
			totalTokens += len(tc.Function.Arguments) / 4
		}
	}

	// Get model context budget
	contextLength := m.openai.GetModelContextLengthContext(ctx, modelID)
	budget := contextLength - 16384 // reserve for reasoning and response
	if budget < 1024 {
		budget = contextLength / 2
	}
	threshold := int(float64(budget) * 0.7)

	if totalTokens > threshold && len(apiMessages) > 10 {
		apiMessages, err = m.compressContext(ctx, sessionID, apiMessages, systemPrompt, modelID)
		if err != nil {
			// If compression fails, just return what we have
			return apiMessages, nil
		}
	}

	return apiMessages, nil
}

// compressContext summarizes middle messages to fit within the context budget.
func (m *SessionManager) compressContext(ctx context.Context, sessionID int, messages []Message, systemPrompt string, modelID string) ([]Message, error) {
	if len(messages) <= 10 {
		return messages, nil
	}

	// Keep complete recent turns. Splitting a tool call from its results makes
	// the conversation invalid for providers, so only cut at a user message.
	keepStart := 1
	keepEnd := 1
	userTurns := 0
	for i := len(messages) - 1; i > 0; i-- {
		if messages[i].Role == "user" {
			userTurns++
			if userTurns == 2 {
				keepEnd = i
				break
			}
		}
	}
	if keepEnd <= keepStart {
		return messages, nil
	}

	// Extract middle messages for summarization
	var middleParts []string
	for _, msg := range messages[keepStart:keepEnd] {
		if msg.Role == "tool" {
			continue // skip tool results in summary
		}
		middleParts = append(middleParts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}

	if len(middleParts) == 0 {
		return messages, nil
	}

	// Summarize middle
	summaryReq := ChatRequest{
		Model: modelID,
		Messages: []Message{
			{Role: "system", Content: "Summarize this conversation in 300 words. Preserve: all titles recommended, user preferences stated, things the user rejected or disliked, and any specific requests."},
			{Role: "user", Content: fmt.Sprintf("Conversation to summarize:\n\n%s", joinStrings(middleParts, "\n\n"))},
		},
		MaxTokens:   4096,
		Temperature: 0.3,
	}

	summaryMsg, err := m.openai.ChatContext(ctx, summaryReq)
	if err != nil {
		return messages, nil // return uncompressed on failure
	}

	// Build compressed message list
	compressed := make([]Message, 0, keepStart+1+8)
	compressed = append(compressed, messages[:keepStart]...)
	compressed = append(compressed, Message{
		Role:    "system",
		Content: fmt.Sprintf("[Conversation summary: %s]", summaryMsg.Content),
	})
	compressed = append(compressed, messages[keepEnd:]...)

	// The stored transcript remains canonical. Appending a summary without
	// replacing summarized rows duplicates history and grows every subsequent request.

	return compressed, nil
}

// MaybeGenerateTitle generates a title after the 3rd user message.
func (m *SessionManager) MaybeGenerateTitle(sessionID int, modelID string) {
	// Check if title already exists
	var title sql.NullString
	m.db.QueryRow(`SELECT title FROM ai_sessions WHERE id = ?`, sessionID).Scan(&title)
	if title.Valid && title.String != "" {
		return
	}

	// Count user messages
	var count int
	m.db.QueryRow(`SELECT COUNT(*) FROM ai_messages WHERE session_id = ? AND role = 'user'`, sessionID).Scan(&count)
	if count < 3 {
		return
	}

	// Get first few messages for context
	rows, err := m.db.Query(`SELECT role, content FROM ai_messages WHERE session_id = ? AND role IN ('user', 'assistant') ORDER BY id ASC LIMIT 6`, sessionID)
	if err != nil {
		return
	}
	defer rows.Close()

	var contextMsgs []Message
	for rows.Next() {
		var role, content string
		rows.Scan(&role, &content)
		// Truncate long messages
		if len(content) > 200 {
			content = content[:200]
		}
		contextMsgs = append(contextMsgs, Message{Role: role, Content: content})
	}

	titleReq := ChatRequest{
		Model: modelID,
		Messages: append([]Message{
			{Role: "system", Content: "Give this conversation a 5-word title. Respond with ONLY the title, nothing else."},
		}, contextMsgs...),
		MaxTokens:   4096,
		Temperature: 0.5,
	}

	titleMsg, err := m.openai.Chat(titleReq)
	if err != nil {
		return
	}

	newTitle := strings.TrimSpace(titleMsg.Content)
	if newTitle == "" {
		return
	}
	if len(newTitle) > 100 {
		newTitle = newTitle[:100]
	}
	m.db.Exec(`UPDATE ai_sessions SET title = ? WHERE id = ?`, newTitle, sessionID)
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
