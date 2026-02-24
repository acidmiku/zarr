package ai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SessionManager handles AI session persistence and context management.
type SessionManager struct {
	db     *sql.DB
	openai *OpenRouterClient
}

func NewSessionManager(db *sql.DB, openai *OpenRouterClient) *SessionManager {
	return &SessionManager{db: db, openai: openai}
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
	ID            int       `json:"id"`
	SessionID     int       `json:"session_id"`
	Role          string    `json:"role"`
	Content       string    `json:"content"`
	ToolCalls     string    `json:"tool_calls,omitempty"`
	ToolCallID    string    `json:"tool_call_id,omitempty"`
	TokenEstimate int       `json:"token_estimate"`
	CreatedAt     time.Time `json:"created_at"`
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
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
		sessions = append(sessions, s)
	}
	return sessions, nil
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
	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)

	rows, err := m.db.Query(`SELECT id, session_id, role, content, COALESCE(tool_calls, ''), COALESCE(tool_call_id, ''), COALESCE(token_estimate, 0), created_at
		FROM ai_messages WHERE session_id = ? ORDER BY id ASC`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get messages: %w", err)
	}
	defer rows.Close()

	var messages []StoredMessage
	for rows.Next() {
		var msg StoredMessage
		var createdStr string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolCallID, &msg.TokenEstimate, &createdStr); err != nil {
			continue
		}
		msg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
		messages = append(messages, msg)
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
	tokenEst := len(content) / 4
	if toolCalls != "" {
		tokenEst += len(toolCalls) / 4
	}

	var tcVal, tcidVal interface{}
	if toolCalls != "" {
		tcVal = toolCalls
	}
	if toolCallID != "" {
		tcidVal = toolCallID
	}

	res, err := m.db.Exec(`INSERT INTO ai_messages (session_id, role, content, tool_calls, tool_call_id, token_estimate)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, role, content, tcVal, tcidVal, tokenEst)
	if err != nil {
		return nil, fmt.Errorf("save message: %w", err)
	}
	id, _ := res.LastInsertId()

	// Update session timestamp
	m.db.Exec(`UPDATE ai_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID)

	return &StoredMessage{
		ID:            int(id),
		SessionID:     sessionID,
		Role:          role,
		Content:       content,
		ToolCalls:     toolCalls,
		ToolCallID:    toolCallID,
		TokenEstimate: tokenEst,
		CreatedAt:     time.Now(),
	}, nil
}

// BuildContext assembles messages for an API call, compressing if needed.
func (m *SessionManager) BuildContext(sessionID int, systemPrompt string, modelID string) ([]Message, error) {
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
		}
		if msg.ToolCalls != "" {
			json.Unmarshal([]byte(msg.ToolCalls), &apiMsg.ToolCalls)
		}
		apiMessages = append(apiMessages, apiMsg)
	}

	// Estimate total tokens
	totalTokens := 0
	for _, msg := range apiMessages {
		totalTokens += len(msg.Content) / 4
		for _, tc := range msg.ToolCalls {
			totalTokens += len(tc.Function.Arguments) / 4
		}
	}

	// Get model context budget
	contextLength := m.openai.GetModelContextLength(modelID)
	budget := contextLength - 2000 // reserve for response
	threshold := int(float64(budget) * 0.7)

	if totalTokens > threshold && len(apiMessages) > 10 {
		apiMessages, err = m.compressContext(sessionID, apiMessages, systemPrompt, modelID)
		if err != nil {
			// If compression fails, just return what we have
			return apiMessages, nil
		}
	}

	return apiMessages, nil
}

// compressContext summarizes middle messages to fit within the context budget.
func (m *SessionManager) compressContext(sessionID int, messages []Message, systemPrompt string, modelID string) ([]Message, error) {
	if len(messages) <= 10 {
		return messages, nil
	}

	// Keep: system (index 0), first 2 exchanges (index 1-4), last 8 messages
	keepStart := 5 // system + 2 exchanges
	if keepStart > len(messages)-8 {
		return messages, nil // not enough messages to compress
	}
	keepEnd := len(messages) - 8

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
		MaxTokens:   500,
		Temperature: 0.3,
	}

	summaryMsg, err := m.openai.Chat(summaryReq)
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

	// Save summary as a system message in DB for persistence
	m.SaveMessage(sessionID, "system", fmt.Sprintf("[Conversation summary: %s]", summaryMsg.Content), "", "")

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
		MaxTokens:   20,
		Temperature: 0.5,
	}

	titleMsg, err := m.openai.Chat(titleReq)
	if err != nil {
		return
	}

	newTitle := titleMsg.Content
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
