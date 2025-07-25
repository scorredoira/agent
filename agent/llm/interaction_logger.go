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

	// Abrir archivo para esta sesión
	filename := filepath.Join(l.logDir, fmt.Sprintf("session_%s.txt", sessionID))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // Error opening file, skip logging
	}

	l.files[sessionID] = file

	// Escribir cabecera de sesión
	timestamp := time.Now().Format("2006-01-02T15:04:05.000000-07:00")
	fmt.Fprintf(file, "=== SESSION START ===\n")
	fmt.Fprintf(file, "Session ID: %s\n", sessionID)
	fmt.Fprintf(file, "Start Time: %s\n", timestamp)
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

	fmt.Fprintf(file, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(file, "                           INTERACTION\n")
	fmt.Fprintf(file, "Timestamp: %s | Provider: %s | Duration: %v\n", timestamp, provider, duration)
	fmt.Fprintf(file, "%s\n", strings.Repeat("=", 80))

	// Log complete messages array sent to LLM
	if req != nil && len(req.Messages) > 0 {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("═", 80))
		fmt.Fprintf(file, " %sCONTEXT SENT TO LLM%s\n", strings.Repeat(" ", 30), strings.Repeat(" ", 27))
		fmt.Fprintf(file, "%s\n", strings.Repeat("═", 80))
		for i, msg := range req.Messages {
			// Role header with simple visual separation
			roleHeader := fmt.Sprintf("[%d] %s:", i+1, strings.ToUpper(msg.Role))
			fmt.Fprintf(file, "\n%s\n", roleHeader)

			// Truncate very long content for readability
			content := msg.Content
			if len(content) > 1000 {
				content = content[:1000] + "... [truncated]"
			}

			// Indent content for clarity
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				fmt.Fprintf(file, "    %s\n", line)
			}

			// Log tool calls in messages
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Fprintf(file, "      -> TOOL_CALL: %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
				}
			}

			// Log tool call ID for tool responses
			if msg.ToolCallID != "" {
				fmt.Fprintf(file, "      TOOL_CALL_ID: %s\n", msg.ToolCallID)
			}

			fmt.Fprintf(file, "\n%s\n", strings.Repeat("-", 70))
		}
	}

	// Log assistant response
	if resp != nil && resp.Content != "" {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("═", 80))
		fmt.Fprintf(file, " %sLLM RESPONSE%s\n", strings.Repeat(" ", 33), strings.Repeat(" ", 33))
		fmt.Fprintf(file, "%s\n", strings.Repeat("═", 80))

		// Indent response content
		lines := strings.Split(resp.Content, "\n")
		for _, line := range lines {
			fmt.Fprintf(file, "    %s\n", line)
		}
	}

	// Log tool calls in response
	if resp != nil && len(resp.ToolCalls) > 0 {
		fmt.Fprintf(file, "\n%s\n", strings.Repeat("═", 80))
		fmt.Fprintf(file, " %sTOOL CALLS REQUESTED%s\n", strings.Repeat(" ", 30), strings.Repeat(" ", 28))
		fmt.Fprintf(file, "%s\n", strings.Repeat("═", 80))
		for _, toolCall := range resp.ToolCalls {
			fmt.Fprintf(file, "    → %s: %s\n", toolCall.Function.Name, toolCall.Function.Arguments)
		}
	}

	// Log errors
	if err != nil {
		fmt.Fprintf(file, "\nERROR: %s\n", err.Error())
	}

	fmt.Fprintf(file, "\n")
	file.Sync() // Flush immediately after each interaction
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

	files, err := filepath.Glob(filepath.Join(l.logDir, "session_*.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to list session files: %w", err)
	}

	sessions := make([]string, 0, len(files))
	for _, file := range files {
		filename := filepath.Base(file)
		if len(filename) > 12 && filename[:8] == "session_" && filename[len(filename)-4:] == ".txt" {
			sessionID := filename[8 : len(filename)-4]
			sessions = append(sessions, sessionID)
		}
	}

	return sessions, nil
}

func (l *InteractionLogger) ExportSession(sessionID string) ([]byte, error) {
	filename := filepath.Join(l.logDir, fmt.Sprintf("session_%s.txt", sessionID))
	return os.ReadFile(filename)
}

// Temporary compatibility types for compilation
type SessionLog struct {
	ID           string
	StartTime    time.Time
	EndTime      time.Time
	Interactions []interface{}
	Metadata     map[string]interface{}
}
