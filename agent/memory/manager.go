package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// MemoryManager gestiona múltiples conversaciones y memoria persistente
type MemoryManager struct {
	storageDir      string
	currentSession  *ConversationMemory
	globalMemory    *GlobalMemory
	contextManager  *ContextManager
	maxSessions     int
	autoSaveEnabled bool
}

// GlobalMemory mantiene información que persiste entre conversaciones
type GlobalMemory struct {
	UserProfiles    map[string]UserProfile `json:"user_profiles"`
	ClubDatabase    map[string]ClubInfo    `json:"club_database"`
	CommonPatterns  []Pattern              `json:"common_patterns"`
	LastUpdated     time.Time              `json:"last_updated"`
	TotalSessions   int                    `json:"total_sessions"`
	PreferredTopics []string               `json:"preferred_topics"`
}

// Pattern representa un patrón común en las conversaciones
type Pattern struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Triggers    []string  `json:"triggers"`
	Response    string    `json:"response"`
	Frequency   int       `json:"frequency"`
	LastSeen    time.Time `json:"last_seen"`
}

// SessionSummary información resumida de una sesión
type SessionSummary struct {
	SessionID   string    `json:"session_id"`
	StartTime   time.Time `json:"start_time"`
	Duration    string    `json:"duration"`
	MessageCount int      `json:"message_count"`
	Topics      []string  `json:"topics"`
	Summary     string    `json:"summary"`
}

// NewMemoryManager crea un nuevo gestor de memoria
func NewMemoryManager(storageDir string) *MemoryManager {
	mm := &MemoryManager{
		storageDir:      storageDir,
		contextManager:  NewContextManager(),
		maxSessions:     50, // Mantener máximo 50 sesiones
		autoSaveEnabled: true,
	}
	
	// Registrar proveedores por defecto
	mm.registerDefaultProviders()
	
	return mm
}

// registerDefaultProviders registra los proveedores de contexto por defecto
func (mm *MemoryManager) registerDefaultProviders() {
	// Sistema: fecha/hora, versión, etc.
	systemProvider := NewSystemInfoProvider(map[string]interface{}{
		"enabled":      true,
		"priority":     100,
		"include_time": true,
		"include_os":   false,
		"version":      "0.2.0",
	})
	mm.contextManager.RegisterProvider(systemProvider)
	
	// Contexto de sesión
	sessionProvider := NewSessionContextProvider(map[string]interface{}{
		"enabled":  true,
		"priority": 80,
	})
	mm.contextManager.RegisterProvider(sessionProvider)
}

// Initialize inicializa el gestor de memoria
func (mm *MemoryManager) Initialize() error {
	// Crear directorio de almacenamiento
	if err := os.MkdirAll(mm.storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Cargar memoria global
	if err := mm.loadGlobalMemory(); err != nil {
		// Si no existe, crear nueva
		mm.globalMemory = &GlobalMemory{
			UserProfiles:    make(map[string]UserProfile),
			ClubDatabase:    make(map[string]ClubInfo),
			CommonPatterns:  make([]Pattern, 0),
			LastUpdated:     time.Now(),
			PreferredTopics: make([]string, 0),
		}
	}

	return nil
}

// StartNewSession inicia una nueva sesión conversacional
func (mm *MemoryManager) StartNewSession() (*ConversationMemory, error) {
	session := NewConversationMemory(mm.storageDir)
	mm.currentSession = session
	mm.globalMemory.TotalSessions++

	// Auto-guardar si está habilitado
	if mm.autoSaveEnabled {
		go mm.autoSave()
	}

	return session, nil
}

// LoadSession carga una sesión existente
func (mm *MemoryManager) LoadSession(sessionID string) (*ConversationMemory, error) {
	session, err := LoadConversationMemory(sessionID, mm.storageDir)
	if err != nil {
		return nil, err
	}

	mm.currentSession = session
	return session, nil
}

// DeleteSession deletes a session by session ID
func (mm *MemoryManager) DeleteSession(sessionID string) error {
	// Clear current session if it's the one being deleted
	if mm.currentSession != nil && mm.currentSession.SessionID == sessionID {
		mm.currentSession = nil
	}
	
	// Delete the session file
	sessionPath := filepath.Join(mm.storageDir, fmt.Sprintf("session_%s.json", sessionID))
	err := os.Remove(sessionPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file: %w", err)
	}
	
	return nil
}

// GetCurrentSession devuelve la sesión actual
func (mm *MemoryManager) GetCurrentSession() *ConversationMemory {
	return mm.currentSession
}

// AddMessageToCurrentSession añade un mensaje a la sesión actual
func (mm *MemoryManager) AddMessageToCurrentSession(message llm.Message) error {
	if mm.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	mm.currentSession.AddMessage(message)

	// Actualizar memoria global con patrones
	if message.Role == "user" {
		mm.updateGlobalPatterns(message.Content)
	}

	return nil
}

// GetContextForQuery obtiene contexto relevante para una consulta
func (mm *MemoryManager) GetContextForQuery(query string, maxTokens int) []llm.Message {
	if mm.currentSession == nil {
		return []llm.Message{}
	}

	// Calcular cuántos mensajes podemos incluir (aproximación: 4 tokens por palabra)
	wordsPerMessage := 50 // Estimación promedio
	tokensPerMessage := wordsPerMessage * 4
	maxMessages := maxTokens / tokensPerMessage

	if maxMessages < 5 {
		maxMessages = 5 // Mínimo 5 mensajes
	}

	// Obtener mensajes contextuales de la conversación
	contextualMessages := mm.currentSession.GetContextualMessages(query, maxMessages)

	// Añadir contexto global si es relevante
	globalContext := mm.getGlobalContextForQuery(query)
	if globalContext != "" {
		systemMessage := llm.Message{
			Role:    "system",
			Content: "Previous context: " + globalContext,
		}
		contextualMessages = append([]llm.Message{systemMessage}, contextualMessages...)
	}

	// Obtener contexto de los proveedores pluggables
	providerContext := mm.contextManager.GetContext(query, mm.currentSession)
	
	// Combinar contexto de proveedores + mensajes de conversación
	allMessages := append(providerContext, contextualMessages...)

	return allMessages
}

// ListSessions devuelve una lista de sesiones disponibles
func (mm *MemoryManager) ListSessions() ([]SessionSummary, error) {
	files, err := filepath.Glob(filepath.Join(mm.storageDir, "session_*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list session files: %w", err)
	}

	var summaries []SessionSummary

	for _, file := range files {
		// Extraer session ID del nombre del archivo
		basename := filepath.Base(file)
		sessionID := strings.TrimPrefix(basename, "session_")
		sessionID = strings.TrimSuffix(sessionID, ".json")

		// Cargar sesión para obtener información
		session, err := LoadConversationMemory(sessionID, mm.storageDir)
		if err != nil {
			continue // Saltar sesiones corruptas
		}

		duration := session.LastAccess.Sub(session.StartTime)
		summary := SessionSummary{
			SessionID:    sessionID,
			StartTime:    session.StartTime,
			Duration:     duration.Truncate(time.Second).String(),
			MessageCount: len(session.Messages),
			Topics:       session.Topics,
			Summary:      session.GetSummary(),
		}

		summaries = append(summaries, summary)
	}

	// Ordenar por fecha (más reciente primero)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartTime.After(summaries[j].StartTime)
	})

	return summaries, nil
}

// SearchConversations busca en el historial de conversaciones
func (mm *MemoryManager) SearchConversations(query string, limit int) ([]SearchResult, error) {
	sessions, err := mm.ListSessions()
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	queryWords := strings.Fields(strings.ToLower(query))

	for _, session := range sessions {
		// Cargar sesión completa para buscar
		fullSession, err := LoadConversationMemory(session.SessionID, mm.storageDir)
		if err != nil {
			continue
		}

		// Buscar en mensajes
		for _, message := range fullSession.Messages {
			content := strings.ToLower(message.Content)
			relevance := 0

			for _, word := range queryWords {
				if strings.Contains(content, word) {
					relevance++
				}
			}

			if relevance > 0 {
				result := SearchResult{
					SessionID: session.SessionID,
					Message:   message,
					Relevance: float64(relevance) / float64(len(queryWords)),
					Timestamp: session.StartTime, // Aproximación
					Context:   mm.extractContext(fullSession.Messages, message),
				}
				results = append(results, result)
			}
		}

		if len(results) >= limit {
			break
		}
	}

	// Ordenar por relevancia
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SearchResult representa un resultado de búsqueda
type SearchResult struct {
	SessionID string      `json:"session_id"`
	Message   llm.Message `json:"message"`
	Relevance float64     `json:"relevance"`
	Timestamp time.Time   `json:"timestamp"`
	Context   string      `json:"context"`
}

// CleanupOldSessions elimina sesiones antiguas para mantener el límite
func (mm *MemoryManager) CleanupOldSessions() error {
	sessions, err := mm.ListSessions()
	if err != nil {
		return err
	}

	if len(sessions) <= mm.maxSessions {
		return nil // No hay nada que limpiar
	}

	// Ordenar por fecha (más antigua primero)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	// Eliminar sesiones más antiguas
	toDelete := len(sessions) - mm.maxSessions
	for i := 0; i < toDelete; i++ {
		filename := filepath.Join(mm.storageDir, fmt.Sprintf("session_%s.json", sessions[i].SessionID))
		if err := os.Remove(filename); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessions[i].SessionID, err)
		}
	}

	return nil
}

// GetGlobalStats devuelve estadísticas globales
func (mm *MemoryManager) GetGlobalStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_sessions":    mm.globalMemory.TotalSessions,
		"storage_dir":       mm.storageDir,
		"current_session":   mm.currentSession != nil,
		"preferred_topics":  mm.globalMemory.PreferredTopics,
		"known_clubs":       len(mm.globalMemory.ClubDatabase),
		"user_profiles":     len(mm.globalMemory.UserProfiles),
		"common_patterns":   len(mm.globalMemory.CommonPatterns),
		"last_updated":      mm.globalMemory.LastUpdated,
		"context_providers": mm.contextManager.ListProviders(),
	}

	if mm.currentSession != nil {
		stats["current_session_id"] = mm.currentSession.SessionID
		stats["current_messages"] = len(mm.currentSession.Messages)
		stats["current_topics"] = mm.currentSession.Topics
	}

	return stats
}

// RegisterContextProvider registra un nuevo proveedor de contexto
func (mm *MemoryManager) RegisterContextProvider(provider ContextProvider) {
	mm.contextManager.RegisterProvider(provider)
}

// AddBusinessContext añade un proveedor de contexto de negocio
func (mm *MemoryManager) AddBusinessContext(domain string, keywords []string, context string) {
	provider := NewBusinessContextProvider(map[string]interface{}{
		"enabled":     true,
		"priority":    90,
		"domain":      domain,
		"keywords":    keywords,
		"context":     context,
		"description": fmt.Sprintf("Business context for %s domain", domain),
	})
	mm.contextManager.RegisterProvider(provider)
}

// AddConfigurableContext añade un proveedor completamente configurable
func (mm *MemoryManager) AddConfigurableContext(name, description string, config map[string]interface{}) {
	config["name"] = name
	config["description"] = description
	provider := NewConfigurableContextProvider(config)
	mm.contextManager.RegisterProvider(provider)
}

// RemoveContextProvider remueve un proveedor por nombre
func (mm *MemoryManager) RemoveContextProvider(name string) bool {
	return mm.contextManager.RemoveProvider(name)
}

// ListContextProviders lista todos los proveedores de contexto
func (mm *MemoryManager) ListContextProviders() []ProviderInfo {
	return mm.contextManager.ListProviders()
}

// SetContextManagerEnabled habilita/deshabilita el gestor de contexto
func (mm *MemoryManager) SetContextManagerEnabled(enabled bool) {
	mm.contextManager.SetEnabled(enabled)
}

// Métodos privados

func (mm *MemoryManager) loadGlobalMemory() error {
	filename := filepath.Join(mm.storageDir, "global_memory.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &mm.globalMemory)
}

func (mm *MemoryManager) saveGlobalMemory() error {
	filename := filepath.Join(mm.storageDir, "global_memory.json")
	mm.globalMemory.LastUpdated = time.Now()

	data, err := json.MarshalIndent(mm.globalMemory, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

func (mm *MemoryManager) updateGlobalPatterns(userMessage string) {
	// Detectar patrones comunes y actualizarlos
	lower := strings.ToLower(userMessage)

	// Patrones básicos para club management
	patterns := map[string][]string{
		"pricing_inquiry":     {"precio", "tarifa", "costo", "cuanto cuesta"},
		"membership_question": {"socio", "miembro", "membresia", "inscripcion"},
		"facility_question":   {"instalacion", "cancha", "piscina", "gimnasio"},
		"booking_issue":       {"reserva", "turno", "horario", "disponible"},
	}

	for patternID, triggers := range patterns {
		for _, trigger := range triggers {
			if strings.Contains(lower, trigger) {
				mm.incrementPattern(patternID, trigger)
				break
			}
		}
	}
}

func (mm *MemoryManager) incrementPattern(patternID, trigger string) {
	for i, pattern := range mm.globalMemory.CommonPatterns {
		if pattern.ID == patternID {
			mm.globalMemory.CommonPatterns[i].Frequency++
			mm.globalMemory.CommonPatterns[i].LastSeen = time.Now()
			return
		}
	}

	// Crear nuevo patrón
	newPattern := Pattern{
		ID:          patternID,
		Description: "Patrón detectado automáticamente",
		Triggers:    []string{trigger},
		Frequency:   1,
		LastSeen:    time.Now(),
	}
	mm.globalMemory.CommonPatterns = append(mm.globalMemory.CommonPatterns, newPattern)
}

func (mm *MemoryManager) getGlobalContextForQuery(query string) string {
	// Buscar patrones relevantes
	lower := strings.ToLower(query)
	var relevantContext []string

	for _, pattern := range mm.globalMemory.CommonPatterns {
		for _, trigger := range pattern.Triggers {
			if strings.Contains(lower, trigger) && pattern.Response != "" {
				relevantContext = append(relevantContext, pattern.Response)
				break
			}
		}
	}

	if len(relevantContext) > 0 {
		return strings.Join(relevantContext, " ")
	}

	return ""
}

func (mm *MemoryManager) extractContext(messages []llm.Message, target llm.Message) string {
	// Encontrar el mensaje objetivo y extraer contexto
	for i, msg := range messages {
		if msg.Content == target.Content && msg.Role == target.Role {
			// Extraer contexto anterior
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 2
			if end >= len(messages) {
				end = len(messages) - 1
			}

			var context []string
			for j := start; j <= end; j++ {
				if j != i {
					preview := messages[j].Content
					if len(preview) > 50 {
						preview = preview[:50] + "..."
					}
					context = append(context, fmt.Sprintf("%s: %s", messages[j].Role, preview))
				}
			}
			return strings.Join(context, " | ")
		}
	}

	return ""
}

func (mm *MemoryManager) autoSave() {
	ticker := time.NewTicker(30 * time.Second) // Auto-guardar cada 30 segundos
	defer ticker.Stop()

	for range ticker.C {
		if mm.currentSession != nil {
			mm.currentSession.Save()
		}
		mm.saveGlobalMemory()
	}
}