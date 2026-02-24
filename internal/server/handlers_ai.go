package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"mediaforge/internal/ai"
)

// handleListAISessions returns all chat sessions.
func (s *Server) handleListAISessions(w http.ResponseWriter, r *http.Request) {
	if s.aiSession == nil {
		writeError(w, 400, "AI assistant not configured. Add an OpenRouter API key in Settings.")
		return
	}
	sessions, err := s.aiSession.ListSessions()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if sessions == nil {
		sessions = []ai.Session{}
	}
	writeJSON(w, 200, sessions)
}

// handleCreateAISession creates a new chat session.
func (s *Server) handleCreateAISession(w http.ResponseWriter, r *http.Request) {
	if s.aiSession == nil {
		writeError(w, 400, "AI assistant not configured. Add an OpenRouter API key in Settings.")
		return
	}
	session, err := s.aiSession.CreateSession()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, session)
}

// handleGetAISession returns a session with all messages.
func (s *Server) handleGetAISession(w http.ResponseWriter, r *http.Request) {
	if s.aiSession == nil {
		writeError(w, 400, "AI assistant not configured.")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid session ID")
		return
	}
	session, messages, err := s.aiSession.GetSession(id)
	if err != nil {
		writeError(w, 404, "session not found")
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"session":  session,
		"messages": messages,
	})
}

// handleDeleteAISession deletes a session.
func (s *Server) handleDeleteAISession(w http.ResponseWriter, r *http.Request) {
	if s.aiSession == nil {
		writeError(w, 400, "AI assistant not configured.")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid session ID")
		return
	}
	if err := s.aiSession.DeleteSession(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleAIChat handles the streaming chat endpoint.
func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil {
		writeError(w, 400, "OpenRouter API key not configured. Go to Settings to add it.")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid session ID")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		writeError(w, 400, "content is required")
		return
	}

	// Save user message
	s.aiSession.SaveMessage(id, "user", req.Content, "", "")

	// Stream the response
	s.streamAIResponse(w, r, id)
}

// handleAIRecommend triggers initial recommendations based on ratings.
func (s *Server) handleAIRecommend(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil {
		writeError(w, 400, "OpenRouter API key not configured. Go to Settings to add it.")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid session ID")
		return
	}

	// Build ratings message
	ratingsText := s.buildRatingsMessage()
	s.aiSession.SaveMessage(id, "user", ratingsText, "", "")

	// Stream the response
	s.streamAIResponse(w, r, id)
}

func (s *Server) buildRatingsMessage() string {
	rows, err := s.db.Query(`SELECT r.media_type, r.rating, r.comment,
		COALESCE(m.title, 'Unknown Title') as title
		FROM user_ratings r
		LEFT JOIN media_items m ON m.tmdb_id = r.tmdb_id AND m.type = r.media_type
		ORDER BY r.rating DESC`)
	if err != nil {
		return "Based on my taste, what should I watch next?"
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Here are my ratings from my media library:\n\n")
	count := 0
	for rows.Next() {
		var mediaType, title string
		var rating int
		var comment *string
		rows.Scan(&mediaType, &rating, &comment, &title)
		stars := strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
		commentStr := ""
		if comment != nil && *comment != "" {
			commentStr = fmt.Sprintf(` — "%s"`, *comment)
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s%s\n", title, mediaType, stars, commentStr))
		count++
	}

	if count == 0 {
		return "I haven't rated anything yet, but I'm looking for recommendations. What should I watch? Ask me about my preferences."
	}

	sb.WriteString("\nBased on my taste, what should I watch next?")
	return sb.String()
}

// streamAIResponse handles the SSE streaming loop with tool execution.
func (s *Server) streamAIResponse(w http.ResponseWriter, r *http.Request, sessionID int) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	sendEvent := func(event string, data interface{}) {
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
		flusher.Flush()
	}

	// Get AI settings
	modelID, _ := s.db.GetSetting("openrouter_model")
	if modelID == "" {
		modelID = "anthropic/claude-sonnet-4-20250514"
	}
	preset, _ := s.db.GetSetting("ai_personality_preset")
	custom, _ := s.db.GetSetting("ai_personality_custom")
	braveKey, _ := s.db.GetSetting("brave_api_key")

	systemPrompt := ai.BuildSystemPrompt(preset, custom)
	braveAvailable := braveKey != ""

	// Build tool executor
	executor := &ai.ToolExecutor{
		Jikan: s.aiJikan,
		Brave: s.aiBrave,
		DB:    s.db.DB,
		TMDB:  s.tmdb,
	}

	// Tool call loop
	for iteration := 0; iteration < 5; iteration++ {
		// Build context for this iteration
		messages, err := s.aiSession.BuildContext(sessionID, systemPrompt, modelID)
		if err != nil {
			sendEvent("error", map[string]string{"message": "Failed to build context: " + err.Error()})
			sendEvent("done", map[string]interface{}{})
			return
		}

		chatReq := ai.ChatRequest{
			Model:    modelID,
			Messages: messages,
			Tools:    ai.ToolDefs(braveAvailable),
		}

		var fullContent strings.Builder
		var toolCalls []ai.ToolCall
		var finishReason string

		msg, err := s.aiOpenRouter.ChatStream(chatReq, func(evt ai.StreamEvent) {
			switch evt.Type {
			case "text":
				fullContent.WriteString(evt.Content)
				sendEvent("text", map[string]string{"content": evt.Content})
			case "tool_calls":
				toolCalls = evt.ToolCalls
			case "done":
				finishReason = evt.FinishReason
			}
		})

		if err != nil {
			slog.Error("openrouter stream error", "error", err)
			sendEvent("error", map[string]string{"message": "AI request failed: " + err.Error()})
			sendEvent("done", map[string]interface{}{})
			return
		}

		_ = finishReason

		if len(toolCalls) > 0 {
			// Save assistant message with tool calls
			tcJSON, _ := json.Marshal(msg.ToolCalls)
			s.aiSession.SaveMessage(sessionID, "assistant", fullContent.String(), string(tcJSON), "")

			// Execute each tool call
			for _, tc := range toolCalls {
				sendEvent("tool_call_start", map[string]string{"tool": tc.Function.Name})

				result, recs, _ := executor.ExecuteTool(tc.Function.Name, tc.Function.Arguments)

				sendEvent("tool_call_end", map[string]string{
					"tool":           tc.Function.Name,
					"result_summary": truncate(result, 100),
				})

				// Emit recommendations if any
				if len(recs) > 0 {
					sendEvent("recommendations", map[string]interface{}{"recommendations": recs})
				}

				// Save tool result as a message
				s.aiSession.SaveMessage(sessionID, "tool", result, "", tc.ID)
			}

			// Signal frontend to start a new message bubble for continuation
			sendEvent("new_message", map[string]interface{}{})

			// Continue loop — the AI will see tool results and respond
			continue
		}

		// No tool calls — save the final assistant message and break
		s.aiSession.SaveMessage(sessionID, "assistant", fullContent.String(), "", "")

		// Generate title in background
		go s.aiSession.MaybeGenerateTitle(sessionID, modelID)

		break
	}

	sendEvent("done", map[string]interface{}{})
}

// handleAIModels returns available OpenRouter models.
func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil {
		writeError(w, 400, "OpenRouter API key not configured.")
		return
	}
	models, err := s.aiOpenRouter.GetModels()
	if err != nil {
		writeError(w, 500, "Failed to fetch models: "+err.Error())
		return
	}
	writeJSON(w, 200, models)
}

// handleJikanImage serves cached Jikan/MAL poster images.
func (s *Server) handleJikanImage(w http.ResponseWriter, r *http.Request) {
	malID, err := parseID(r, "malID")
	if err != nil {
		writeError(w, 400, "invalid MAL ID")
		return
	}

	if s.aiJikan == nil {
		writeError(w, 500, "Jikan client not initialized")
		return
	}

	path, err := s.aiJikan.GetPosterPath(malID)
	if err != nil {
		writeError(w, 502, "Failed to fetch poster: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, path)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
