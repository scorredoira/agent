package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/santiagocorredoira/agent/agent/llm"
)

// WebSocketMessage represents messages sent over WebSocket
type WebSocketMessage struct {
	Type      string                 `json:"type"`
	Content   string                 `json:"content,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Streaming bool                   `json:"streaming,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// WebSocketHandler handles WebSocket connections for the agent
type WebSocketHandler struct {
	agent    *V3Agent
	upgrader websocket.Upgrader
	sessions sync.Map // sessionID -> *memory.ConversationMemory
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(agent *V3Agent) *WebSocketHandler {
	return &WebSocketHandler{
		agent: agent,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin in development
				// TODO: Restrict this in production
				return true
			},
		},
	}
}

// ServeWebSocket handles WebSocket upgrade and communication
func (a *V3Agent) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	handler := NewWebSocketHandler(a)
	handler.ServeHTTP(w, r)
}

// ServeHTTP implements http.Handler
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Set up ping/pong to keep connection alive
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Create channels for communication
	messageChan := make(chan WebSocketMessage, 10)
	done := make(chan struct{})

	// Start goroutine to handle outgoing messages
	go h.handleOutgoing(conn, messageChan, ticker, done)

	// Handle incoming messages
	h.handleIncoming(conn, messageChan, done)
}

// handleIncoming processes incoming WebSocket messages
func (h *WebSocketHandler) handleIncoming(conn *websocket.Conn, outChan chan<- WebSocketMessage, done chan<- struct{}) {
	defer close(done)

	for {
		var msg WebSocketMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		switch msg.Type {
		case "start_session":
			h.handleStartSession(msg, outChan)
		case "message":
			h.handleMessage(msg, outChan)
		case "load_session":
			h.handleLoadSession(msg, outChan)
		case "list_sessions":
			h.handleListSessions(msg, outChan)
		case "generate_title":
			h.handleGenerateTitle(msg, outChan)
		case "generate_summary":
			h.handleGenerateSummary(msg, outChan)
		case "delete_session":
			h.handleDeleteSession(msg, outChan)
		default:
			outChan <- WebSocketMessage{
				Type:  "error",
				Error: fmt.Sprintf("Unknown message type: %s", msg.Type),
			}
		}
	}
}

// handleOutgoing sends messages to the WebSocket client
func (h *WebSocketHandler) handleOutgoing(conn *websocket.Conn, inChan <-chan WebSocketMessage, ticker *time.Ticker, done <-chan struct{}) {
	for {
		select {
		case msg, ok := <-inChan:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// handleStartSession creates a new conversation session
func (h *WebSocketHandler) handleStartSession(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	// Extract context from message data if provided
	var context *ConversationContext
	if msg.Data != nil {
		contextData, _ := json.Marshal(msg.Data)
		json.Unmarshal(contextData, &context)
	}

	session, err := h.agent.StartConversationWithContext(context)
	if err != nil {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: fmt.Sprintf("Failed to start session: %v", err),
		}
		return
	}

	// Store session
	h.sessions.Store(session.SessionID, session)

	// Start logging session for this conversation
	h.startLoggingSession(session.SessionID)

	outChan <- WebSocketMessage{
		Type:      "session_started",
		SessionID: session.SessionID,
		Data: map[string]interface{}{
			"session_id": session.SessionID,
		},
	}
}

// handleMessage processes a user message
func (h *WebSocketHandler) handleMessage(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	if msg.SessionID == "" {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: "Session ID required",
		}
		return
	}

	// Load or get session
	_, exists := h.sessions.Load(msg.SessionID)
	if !exists {
		// Try to load from storage
		session, err := h.agent.LoadConversation(msg.SessionID)
		if err != nil {
			outChan <- WebSocketMessage{
				Type:  "error",
				Error: fmt.Sprintf("Session not found: %s", msg.SessionID),
			}
			return
		}
		h.sessions.Store(msg.SessionID, session)
	}

	// Log provider info for debugging
	log.Printf("Processing message with LLM provider: %s", h.agent.llmProvider.GetName())

	// Send initial status
	outChan <- WebSocketMessage{
		Type:      "status",
		Content:   "Processing your message...",
		SessionID: msg.SessionID,
	}

	// Create a custom completion handler for streaming
	streamHandler := func(chunk string, isComplete bool) {
		outChan <- WebSocketMessage{
			Type:      "response",
			Content:   chunk,
			SessionID: msg.SessionID,
			Streaming: !isComplete,
		}
	}

	// Process message with streaming support
	go h.processMessageWithStreaming(msg.Content, msg.SessionID, outChan, streamHandler)
}

// processMessageWithStreaming handles message processing with real-time updates
func (h *WebSocketHandler) processMessageWithStreaming(content, sessionID string, outChan chan<- WebSocketMessage, streamHandler func(string, bool)) {
	// Log current provider state before sending message
	log.Printf("🔍 About to send message. Current provider: %s", h.agent.llmProvider.GetName())
	log.Printf("🔍 Provider available: %t", h.agent.llmProvider.IsAvailable(h.agent.ctx))
	
	// Create status callback to send status messages to WebSocket
	statusCallback := func(message string) {
		outChan <- WebSocketMessage{
			Type:      "status",
			Content:   message,
			SessionID: sessionID,
		}
	}
	
	// Create options with status callback
	options := DefaultConversationOptions()
	options.StatusCallback = statusCallback
	
	// Send the message to the agent
	response, err := h.agent.SendMessageWithStreaming(content, options)
	if err != nil {
		outChan <- WebSocketMessage{
			Type:      "error",
			Error:     fmt.Sprintf("Failed to process message: %v", err),
			SessionID: sessionID,
		}
		return
	}
	
	// Log what we got back
	log.Printf("🔍 Response received. Length: %d chars", len(response.Content))

	// Send the complete response
	streamHandler(response.Content, true)

	// Send completion signal
	outChan <- WebSocketMessage{
		Type:      "complete",
		SessionID: sessionID,
		Data: map[string]interface{}{
			"tokens": map[string]interface{}{
				"prompt":     response.Usage.PromptTokens,
				"completion": response.Usage.CompletionTokens,
				"total":      response.Usage.TotalTokens,
			},
		},
	}
}

// handleLoadSession loads an existing session
func (h *WebSocketHandler) handleLoadSession(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	if msg.SessionID == "" {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: "Session ID required",
		}
		return
	}

	session, err := h.agent.LoadConversation(msg.SessionID)
	if err != nil {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: fmt.Sprintf("Failed to load session: %v", err),
		}
		return
	}

	// Store in active sessions
	h.sessions.Store(session.SessionID, session)

	// Note: No need to start logging session here since it's an existing conversation
	// Logging should only start for new sessions, not when loading existing ones

	// Get conversation history
	messages := session.GetRecentMessages(50) // Get last 50 messages
	history := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		history[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	outChan <- WebSocketMessage{
		Type:      "session_loaded",
		SessionID: session.SessionID,
		Data: map[string]interface{}{
			"session_id": session.SessionID,
			"history":    history,
		},
	}
}

// handleListSessions returns available sessions
func (h *WebSocketHandler) handleListSessions(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	sessions, err := h.agent.ListConversations()
	if err != nil {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: fmt.Sprintf("Failed to list sessions: %v", err),
		}
		return
	}

	sessionList := make([]map[string]interface{}, len(sessions))
	for i, session := range sessions {
		sessionList[i] = map[string]interface{}{
			"session_id":     session.SessionID,
			"created_at":     session.StartTime,
			"message_count":  session.MessageCount,
			"topics":         session.Topics,
			"summary":        session.Summary,
		}
	}

	outChan <- WebSocketMessage{
		Type: "sessions_list",
		Data: map[string]interface{}{
			"sessions": sessionList,
		},
	}
}

// handleGenerateTitle generates a title for a conversation
func (h *WebSocketHandler) handleGenerateTitle(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	if msg.SessionID == "" {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: "Session ID required for title generation",
		}
		return
	}

	// Generate title in background to avoid blocking
	go func() {
		title, err := h.agent.GenerateConversationTitle(msg.SessionID)
		if err != nil {
			// Log error but don't send to client - fail silently
			log.Printf("Failed to generate title for session %s: %v", msg.SessionID, err)
			return
		}

		// Send title if we got one (even if it's "New conversation")
		if title != "" {
			outChan <- WebSocketMessage{
				Type:      "title_generated",
				SessionID: msg.SessionID,
				Data: map[string]interface{}{
					"session_id": msg.SessionID,
					"title":      title,
				},
			}
		}
	}()
}

// handleGenerateSummary generates an AI summary for a conversation
func (h *WebSocketHandler) handleGenerateSummary(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	if msg.SessionID == "" {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: "Session ID required for summary generation",
		}
		return
	}

	// Generate summary in background to avoid blocking
	go func() {
		err := h.agent.GenerateConversationSummary(msg.SessionID)
		if err != nil {
			// Log error but don't send to client - fail silently
			log.Printf("Failed to generate summary for session %s: %v", msg.SessionID, err)
			return
		}

		// Send summary generated notification if successful
		outChan <- WebSocketMessage{
			Type:      "summary_generated",
			SessionID: msg.SessionID,
			Data: map[string]interface{}{
				"session_id": msg.SessionID,
			},
		}
	}()
}

// handleDeleteSession deletes a conversation session
func (h *WebSocketHandler) handleDeleteSession(msg WebSocketMessage, outChan chan<- WebSocketMessage) {
	if msg.SessionID == "" {
		outChan <- WebSocketMessage{
			Type:  "error",
			Error: "Session ID required for session deletion",
		}
		return
	}

	// Delete session in background to avoid blocking
	go func() {
		err := h.agent.DeleteConversation(msg.SessionID)
		if err != nil {
			// Log error but don't send to client - fail silently
			log.Printf("Failed to delete session %s: %v", msg.SessionID, err)
			return
		}

		// End logging session for this conversation
		h.endLoggingSession(msg.SessionID)

		// Send session deleted notification if successful
		outChan <- WebSocketMessage{
			Type:      "session_deleted",
			SessionID: msg.SessionID,
			Data: map[string]interface{}{
				"session_id": msg.SessionID,
			},
		}
	}()
}

// StreamingProvider wraps the LLM provider to support streaming
type StreamingProvider struct {
	llm.Provider
	streamHandler func(string, bool)
}

// Complete overrides the Complete method to support streaming
func (p *StreamingProvider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	// For now, we'll use the regular completion
	// In a real implementation, we'd use the streaming API
	resp, err := p.Provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	// Simulate streaming by sending chunks
	// In production, this would use the actual streaming API
	if p.streamHandler != nil {
		words := splitIntoChunks(resp.Content, 5)
		for i, chunk := range words {
			p.streamHandler(chunk, i == len(words)-1)
			time.Sleep(50 * time.Millisecond) // Simulate streaming delay
		}
	}

	return resp, nil
}

// splitIntoChunks splits text into chunks of n words
func splitIntoChunks(text string, wordsPerChunk int) []string {
	// Simple implementation - in production, use proper tokenization
	var chunks []string
	words := []rune(text)
	chunkSize := len(words) / 20 // Approximate chunks

	for i := 0; i < len(words); i += chunkSize {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, string(words[i:end]))
	}

	if len(chunks) == 0 {
		chunks = []string{text}
	}

	return chunks
}

// startLoggingSession starts interaction logging for a WebSocket session
func (h *WebSocketHandler) startLoggingSession(sessionID string) {
	// Get the logger from the agent
	if h.agent.GetLogger() != nil {
		metadata := map[string]interface{}{
			"type":       "websocket_session",
			"session_id": sessionID,
			"timestamp":  time.Now(),
		}
		
		// Start logging session
		h.agent.GetLogger().StartSession(sessionID, metadata)
		
		// Enable interaction logging on the agent for this session
		h.agent.EnableInteractionLogging(h.agent.GetLogger(), sessionID)
		
		log.Printf("Started logging session for WebSocket: %s", sessionID)
	}
}

// endLoggingSession ends interaction logging for a WebSocket session
func (h *WebSocketHandler) endLoggingSession(sessionID string) {
	// Get the logger from the agent
	if h.agent.GetLogger() != nil {
		if err := h.agent.GetLogger().EndSession(sessionID); err != nil {
			log.Printf("Warning: Failed to end logging session %s: %v", sessionID, err)
		} else {
			log.Printf("Ended logging session for WebSocket: %s", sessionID)
		}
	}
}