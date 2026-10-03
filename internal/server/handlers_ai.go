package server

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	if !s.beginAITurn(w, id) {
		return
	}
	defer s.aiSession.EndTurn(id)
	if err := s.aiSession.DeleteSession(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleAIChat handles the streaming chat endpoint.
func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil || s.aiSession == nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeError(w, 400, "content is required")
		return
	}

	if !s.beginAITurn(w, id) {
		return
	}
	defer s.aiSession.EndTurn(id)
	if len(req.Content) > 32000 {
		writeError(w, 400, "message is too long (maximum 32000 bytes)")
		return
	}
	if _, err := s.aiSession.SaveMessage(id, "user", req.Content, "", ""); err != nil {
		writeError(w, 500, "failed to save message")
		return
	}

	// Stream the response
	s.streamAIResponse(w, r, id)
}

// handleAIRecommend triggers initial recommendations based on ratings.
func (s *Server) handleAIRecommend(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil || s.aiSession == nil {
		writeError(w, 400, "OpenRouter API key not configured. Go to Settings to add it.")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid session ID")
		return
	}

	if !s.beginAITurn(w, id) {
		return
	}
	defer s.aiSession.EndTurn(id)
	ratingsText := s.buildRatingsMessage()
	if _, err := s.aiSession.SaveMessage(id, "user", ratingsText, "", ""); err != nil {
		writeError(w, 500, "failed to save message")
		return
	}

	// Stream the response
	s.streamAIResponse(w, r, id)
}

func (s *Server) beginAITurn(w http.ResponseWriter, id int) bool {
	if !s.aiSession.BeginTurn(id) {
		writeError(w, 409, "this conversation is already responding")
		return false
	}
	if _, _, err := s.aiSession.GetSession(id); err != nil {
		s.aiSession.EndTurn(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "session not found")
		} else {
			writeError(w, 500, "failed to load session")
		}
		return false
	}
	return true
}

func (s *Server) buildRatingsMessage() string {
	rows, err := s.db.Query(`SELECT r.media_type, r.rating, r.comment,
		COALESCE(NULLIF(r.title, ''), m.title, 'Unknown Title') as title
		FROM user_ratings r
		LEFT JOIN media_items m ON m.tmdb_id = r.tmdb_id AND m.type = r.media_type
		ORDER BY r.rating DESC`)
	if err != nil {
		return "Based on my taste, what should I watch or listen to next?"
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Here are my ratings from my media library:\n\n")
	count := 0
	for rows.Next() {
		var mediaType, title string
		var rating int
		var comment *string
		if err := rows.Scan(&mediaType, &rating, &comment, &title); err != nil {
			continue
		}
		if rating < 1 || rating > 5 {
			continue
		}
		stars := strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
		commentStr := ""
		if comment != nil && *comment != "" {
			commentStr = fmt.Sprintf(` — "%s"`, *comment)
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s%s\n", title, mediaType, stars, commentStr))
		count++
	}

	// Also include music ratings from albums table
	musicRows, err := s.db.Query(`SELECT a.title, ar.name, a.rating, a.rating_comment
		FROM albums a JOIN artists ar ON a.artist_id = ar.id
		WHERE a.rating IS NOT NULL AND a.rating > 0
		ORDER BY a.rating DESC`)
	if err == nil {
		defer musicRows.Close()
		for musicRows.Next() {
			var title, artistName string
			var rating int
			var comment *string
			if err := musicRows.Scan(&title, &artistName, &rating, &comment); err != nil {
				continue
			}
			if rating < 1 || rating > 5 {
				continue
			}
			stars := strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
			commentStr := ""
			if comment != nil && *comment != "" {
				commentStr = fmt.Sprintf(` — "%s"`, *comment)
			}
			sb.WriteString(fmt.Sprintf("- %s - %s (music): %s%s\n", artistName, title, stars, commentStr))
			count++
		}
	}

	if count == 0 {
		return "I haven't rated anything yet, but I'm looking for recommendations. What should I watch or listen to? Ask me about my preferences."
	}

	sb.WriteString("\nBased on my taste, what should I watch or listen to next?")
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
		modelID = "moonshotai/kimi-k3"
	}
	effort, _ := s.db.GetSetting("openrouter_reasoning_effort")
	if effort == "" {
		effort = "high"
	}
	preset, _ := s.db.GetSetting("ai_personality_preset")
	custom, _ := s.db.GetSetting("ai_personality_custom")
	braveKey, _ := s.db.GetSetting("brave_api_key")

	systemPrompt := ai.BuildSystemPrompt(preset, custom)
	braveAvailable := braveKey != ""

	// Build tool executor
	executor := &ai.ToolExecutor{
		Jikan:       s.aiJikan,
		Brave:       s.aiBrave,
		DB:          s.db.DB,
		TMDB:        s.tmdb,
		MusicBrainz: s.musicbrainz,
	}

	// Tool call loop
	for iteration := 0; iteration < 6; iteration++ {
		if r.Context().Err() != nil {
			return
		}
		// Build context for this iteration
		messages, err := s.aiSession.BuildContextContext(r.Context(), sessionID, systemPrompt, modelID)
		if err != nil {
			sendEvent("error", map[string]string{"message": "Failed to build context: " + err.Error()})
			sendEvent("done", map[string]interface{}{})
			return
		}

		chatReq := ai.ChatRequest{
			Model:     modelID,
			Messages:  messages,
			Tools:     ai.ToolDefs(braveAvailable),
			Reasoning: &ai.ReasoningConfig{Effort: effort},
			MaxTokens: 16384,
		}

		// Reserve the last iteration for a final answer instead of ending on tools.
		if iteration == 5 {
			chatReq.Tools = nil
		}

		msg, err := s.aiOpenRouter.ChatStreamContext(r.Context(), chatReq, func(evt ai.StreamEvent) {
			switch evt.Type {
			case "text":
				sendEvent("text", map[string]string{"content": evt.Content})
			case "reasoning":
				sendEvent("reasoning", map[string]string{"content": evt.Content})
			}
		})

		if err != nil {
			slog.Error("openrouter stream error", "error", err)
			sendEvent("error", map[string]string{"message": "AI request failed: " + err.Error()})
			sendEvent("done", map[string]interface{}{})
			return
		}

		if len(msg.ToolCalls) > 0 {
			turnMessages := []ai.Message{*msg}

			// Execute each tool call
			for _, tc := range msg.ToolCalls {
				sendEvent("tool_call_start", map[string]string{"tool": tc.Function.Name})

				result, recs, toolErr := executor.ExecuteTool(tc.Function.Name, tc.Function.Arguments)
				if toolErr != nil {
					payload, _ := json.Marshal(map[string]string{"error": toolErr.Error()})
					result = string(payload)
				}

				sendEvent("tool_call_end", map[string]string{
					"tool":           tc.Function.Name,
					"result_summary": truncate(result, 100),
				})

				// Emit recommendations if any
				if len(recs) > 0 {
					sendEvent("recommendations", map[string]interface{}{"recommendations": recs})
				}

				// Save tool result as a message
				turnMessages = append(turnMessages, ai.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
			}

			if _, err := s.aiSession.SaveMessages(sessionID, turnMessages); err != nil {
				sendEvent("error", map[string]string{"message": "Failed to save AI response"})
				sendEvent("done", map[string]interface{}{})
				return
			}
			// Signal frontend to start a new message bubble for continuation
			sendEvent("new_message", map[string]interface{}{})

			// Continue loop — the AI will see tool results and respond
			continue
		}

		// No tool calls — save the final assistant message and break
		if _, err := s.aiSession.SaveMessages(sessionID, []ai.Message{*msg}); err != nil {
			sendEvent("error", map[string]string{"message": "Failed to save AI response"})
			sendEvent("done", map[string]interface{}{})
			return
		}

		// Generate title in background
		sessionManager := s.aiSession
		go sessionManager.MaybeGenerateTitle(sessionID, modelID)

		break
	}

	sendEvent("done", map[string]interface{}{})
}

// handleAIModels returns available OpenRouter models.
func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	if s.aiOpenRouter == nil || s.aiSession == nil {
		writeError(w, 400, "OpenRouter API key not configured.")
		return
	}
	models, err := s.aiOpenRouter.GetModelsContext(r.Context())
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
