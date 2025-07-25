package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InteractionLogger maneja el logging simplificado de interacciones LLM
type InteractionLogger struct {
	mu      sync.RWMutex
	enabled bool
	logDir  string
	files   map[string]*os.File // sessionID -> file handle
}

// LoggerConfig configura el InteractionLogger
type LoggerConfig struct {
	Enabled     bool   `json:"enabled"`
	LogDir      string `json:"log_dir"`
	MaxSessions int    `json:"max_sessions"` // Kept for compatibility but not used
}

// NewInteractionLogger crea un nuevo logger de interacciones simplificado
func NewInteractionLogger(config *LoggerConfig) *InteractionLogger {
	if config == nil {
		config = &LoggerConfig{
			Enabled: false,
			LogDir:  "./logs",
		}
	}

	logger := &InteractionLogger{
		enabled: config.Enabled,
		logDir:  config.LogDir,
		files:   make(map[string]*os.File),
	}

	// Crear directorio de logs si está habilitado
	if logger.enabled {
		if err := os.MkdirAll(logger.logDir, 0755); err != nil {
			logger.enabled = false // Deshabilitar en caso de error
		}
	}

	return logger
}

// SetEnabled habilita/deshabilita el logging
func (l *InteractionLogger) SetEnabled(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = enabled
}

// IsEnabled devuelve si el logging está habilitado
func (l *InteractionLogger) IsEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabled
}

// StartSession inicia una nueva sesión de logging
func (l *InteractionLogger) StartSession(sessionID string, metadata map[string]interface{}) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Abrir archivo para esta sesión con timestamp para orden cronológico
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(l.logDir, fmt.Sprintf("%s_session_%s.txt", timestamp, sessionID))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // Error opening file, skip logging
	}

	l.files[sessionID] = file

	// Escribir cabecera de sesión
	sessionTimestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")
	fmt.Fprintf(file, "=== SESSION START ===\n")
	fmt.Fprintf(file, "Session ID: %s\n", sessionID)
	fmt.Fprintf(file, "Start Time: %s\n", sessionTimestamp)
	fmt.Fprintf(file, "===================\n\n")
	file.Sync() // Flush immediately
}

// EndSession finaliza una sesión
func (l *InteractionLogger) EndSession(sessionID string) error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, exists := l.files[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Escribir pie de sesión
	timestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")
	fmt.Fprintf(file, "\n=== SESSION END ===\n")
	fmt.Fprintf(file, "End Time: %s\n", timestamp)
	fmt.Fprintf(file, "==================\n")

	file.Sync() // Flush
	file.Close()
	delete(l.files, sessionID)
	return nil
}

// LogInteraction registra una interacción de forma simplificada
func (l *InteractionLogger) LogInteraction(ctx context.Context, sessionID, provider string, req *CompletionRequest, resp *CompletionResponse, err error, duration time.Duration) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, exists := l.files[sessionID]
	if !exists {
		// Crear sesión automáticamente si no existe
		l.mu.Unlock()
		l.StartSession(sessionID, nil)
		l.mu.Lock()
		file = l.files[sessionID]
		if file == nil {
			return // Failed to create session
		}
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")

	// Obtener el modelo de la request (commented out as it's not used in new format)
	// model := "unknown"
	// if req != nil && req.Model != "" {
	// 	model = req.Model
	// }

	fmt.Fprintf(file, "\n%s\n", strings.Repeat("█", 80))
	fmt.Fprintf(file, "                           INTERACTION\n")
	fmt.Fprintf(file, "Timestamp: %s | Provider: %s | Duration: %v\n", timestamp, provider, duration)
	fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))

	// Log complete messages array sent to LLM
	if req != nil && len(req.Messages) > 0 {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("█", 80))
		fmt.Fprintf(file, "                        ENVIADO A LA LLM\n")
		fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))
		
		for i, msg := range req.Messages {
			// Simple role header
			fmt.Fprintf(file, "\n[%d] %s:\n", i+1, strings.ToUpper(msg.Role))

			// Log complete content with simple indentation
			lines := strings.Split(msg.Content, "\n")
			for _, line := range lines {
				fmt.Fprintf(file, "    %s\n", line)
			}

			// Log tool calls in messages (if any)
			if len(msg.ToolCalls) > 0 {
				fmt.Fprintf(file, "\n    TOOL CALLS REQUESTED:\n")
				for j, tc := range msg.ToolCalls {
					fmt.Fprintf(file, "    [%d] %s(%s)\n", j+1, tc.Function.Name, tc.Function.Arguments)
				}
			}

			// Log tool responses (if any)
			if msg.ToolCallID != "" {
				fmt.Fprintf(file, "\n    TOOL_CALL_ID: %s\n", msg.ToolCallID)
				fmt.Fprintf(file, "    TOOL_RESULT:\n")
				responseLines := strings.Split(msg.Content, "\n")
				for _, line := range responseLines {
					fmt.Fprintf(file, "        %s\n", line)
				}
			}

			fmt.Fprintf(file, "\n%s\n", strings.Repeat("-", 80))
		}
	}

	// Log assistant response
	if resp != nil && resp.Content != "" {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("█", 80))
		fmt.Fprintf(file, "                       RESPONDIDO POR LA LLM\n")
		fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))

		// Simple indentation for response content
		lines := strings.Split(resp.Content, "\n")
		for _, line := range lines {
			fmt.Fprintf(file, "    %s\n", line)
		}
	}

	// Log tool calls in response - these will be executed by the system
	if resp != nil && len(resp.ToolCalls) > 0 {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("█", 80))
		fmt.Fprintf(file, "                      TOOLS A EJECUTAR\n")
		fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))
		
		for i, toolCall := range resp.ToolCalls {
			fmt.Fprintf(file, "\n[%d] TOOL: %s\n", i+1, toolCall.Function.Name)
			fmt.Fprintf(file, "    ARGS: %s\n", toolCall.Function.Arguments)
		}
	}

	// Log errors with proper formatting
	if err != nil {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("╔", 80))
		fmt.Fprintf(file, "╔══════════════════════════════ ERROR ═══════════════════════════════════╗\n")
		fmt.Fprintf(file, "%s\n", strings.Repeat("╚", 80))
		fmt.Fprintf(file, "│   %s\n", err.Error())
		fmt.Fprintf(file, "└%s\n", strings.Repeat("─", 79))
	}

	// Final separator for interaction end
	fmt.Fprintf(file, "\n%s\n\n", strings.Repeat("▄", 80))
	file.Sync() // Flush immediately after each interaction
}

// LogToolExecution logs when a tool starts executing
func (l *InteractionLogger) LogToolExecution(sessionID, toolName, args string) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, exists := l.files[sessionID]
	if !exists {
		return
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")
	
	fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))
	fmt.Fprintf(file, "                        EJECUTADA TOOL\n")
	fmt.Fprintf(file, "Timestamp: %s | Tool: %s\n", timestamp, toolName)
	fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))
	
	fmt.Fprintf(file, "ARGS:\n")
	argLines := strings.Split(args, "\n")
	for _, line := range argLines {
		fmt.Fprintf(file, "    %s\n", line)
	}
	
	file.Sync()
}

// LogToolResult logs the result of a tool execution
func (l *InteractionLogger) LogToolResult(sessionID, toolName, result string, err error, duration time.Duration) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, exists := l.files[sessionID]
	if !exists {
		return
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")
	
	fmt.Fprintf(file, "\n%s\n", strings.Repeat("█", 80))
	fmt.Fprintf(file, "                        RESULTADO TOOL\n")
	fmt.Fprintf(file, "Timestamp: %s | Tool: %s | Duration: %v\n", timestamp, toolName, duration)
	fmt.Fprintf(file, "%s\n", strings.Repeat("█", 80))
	
	if err != nil {
		fmt.Fprintf(file, "ERROR:\n")
		fmt.Fprintf(file, "    %s\n", err.Error())
	} else {
		fmt.Fprintf(file, "RESULT:\n")
		resultLines := strings.Split(result, "\n")
		for _, line := range resultLines {
			fmt.Fprintf(file, "    %s\n", line)
		}
	}
	
	fmt.Fprintf(file, "\n%s\n", strings.Repeat("-", 80))
	file.Sync()
}

// Compatibility methods - simplified versions
func (l *InteractionLogger) GetSession(sessionID string) (*SessionLog, bool) {
	return nil, false // Not supported in simplified version
}

func (l *InteractionLogger) GetActiveSessions() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	sessions := make([]string, 0, len(l.files))
	for sessionID := range l.files {
		sessions = append(sessions, sessionID)
	}
	return sessions
}

func (l *InteractionLogger) LoadSessionFromDisk(sessionID string) (*SessionLog, error) {
	return nil, fmt.Errorf("not supported in simplified logger")
}

func (l *InteractionLogger) ListSavedSessions() ([]string, error) {
	if !l.enabled {
		return nil, fmt.Errorf("logging disabled")
	}

	files, err := filepath.Glob(filepath.Join(l.logDir, "*session_*.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to list session files: %w", err)
	}

	sessions := make([]string, 0, len(files))
	for _, file := range files {
		filename := filepath.Base(file)
		// Extract session ID from filename (handles both old and new formats)
		if strings.Contains(filename, "session_") && strings.HasSuffix(filename, ".txt") {
			// Find "session_" and extract ID
			start := strings.Index(filename, "session_") + 8
			end := strings.LastIndex(filename, ".txt")
			if start < end {
				sessionID := filename[start:end]
				sessions = append(sessions, sessionID)
			}
		}
	}

	return sessions, nil
}

func (l *InteractionLogger) ExportSession(sessionID string) ([]byte, error) {
	// Find the session file (could have timestamp prefix)
	pattern := filepath.Join(l.logDir, fmt.Sprintf("*session_%s.txt", sessionID))
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("session file not found for ID: %s", sessionID)
	}
	return os.ReadFile(matches[0]) // Use first match
}

// Temporary compatibility types for compilation
type SessionLog struct {
	ID           string
	StartTime    time.Time
	EndTime      time.Time
	Interactions []interface{}
	Metadata     map[string]interface{}
}
