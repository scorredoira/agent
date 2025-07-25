package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// ConversationMemory gestiona la memoria conversacional del agente
type ConversationMemory struct {
	SessionID     string                `json:"session_id"`
	StartTime     time.Time             `json:"start_time"`
	LastAccess    time.Time             `json:"last_access"`
	Messages      []llm.Message         `json:"messages"`
	Summary       string                `json:"summary"`
	Topics        []string              `json:"topics"`
	KeyFacts      []KeyFact             `json:"key_facts"`
	UserProfile   UserProfile           `json:"user_profile"`
	MaxMessages   int                   `json:"max_messages"`
	CompressAfter int                   `json:"compress_after"`
	StoragePath   string                `json:"-"`
}

// KeyFact representa un hecho importante extraído de la conversación
type KeyFact struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Relevance   float64   `json:"relevance"`
	Timestamp   time.Time `json:"timestamp"`
	Context     string    `json:"context"`
	Category    string    `json:"category"`
}

// UserProfile mantiene información sobre el usuario para personalización
type UserProfile struct {
	Name         string            `json:"name"`
	Preferences  map[string]string `json:"preferences"`
	ClubInfo     ClubInfo          `json:"club_info"`
	LastSeen     time.Time         `json:"last_seen"`
	Interactions int               `json:"interactions"`
}

// ClubInfo información específica del club del usuario
type ClubInfo struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`        // deportivo, social, etc.
	Size        string            `json:"size"`        // pequeño, mediano, grande
	Members     int               `json:"members"`
	Facilities  []string          `json:"facilities"`
	PricingInfo map[string]string `json:"pricing_info"`
}

// NewConversationMemory crea una nueva instancia de memoria conversacional
func NewConversationMemory(storagePath string) *ConversationMemory {
	sessionID := generateSessionID()
	
	return &ConversationMemory{
		SessionID:     sessionID,
		StartTime:     time.Now(),
		LastAccess:    time.Now(),
		Messages:      make([]llm.Message, 0),
		Topics:        make([]string, 0),
		KeyFacts:      make([]KeyFact, 0),
		UserProfile:   UserProfile{
			Preferences: make(map[string]string),
			ClubInfo: ClubInfo{
				PricingInfo: make(map[string]string),
			},
		},
		MaxMessages:   100,  // Máximo de mensajes antes de comprimir
		CompressAfter: 50,   // Comprimir después de este número
		StoragePath:   storagePath,
	}
}

// LoadConversationMemory carga memoria desde archivo
func LoadConversationMemory(sessionID string, storagePath string) (*ConversationMemory, error) {
	filename := filepath.Join(storagePath, fmt.Sprintf("session_%s.json", sessionID))
	
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory file: %w", err)
	}
	
	var memory ConversationMemory
	if err := json.Unmarshal(data, &memory); err != nil {
		return nil, fmt.Errorf("failed to parse memory file: %w", err)
	}
	
	memory.StoragePath = storagePath
	memory.LastAccess = time.Now()
	
	return &memory, nil
}

// AddMessage añade un mensaje a la memoria
func (cm *ConversationMemory) AddMessage(message llm.Message) {
	cm.Messages = append(cm.Messages, message)
	cm.LastAccess = time.Now()
	cm.UserProfile.Interactions++
	
	// Extraer información si es mensaje del usuario
	if message.Role == "user" {
		cm.extractInfoFromUserMessage(message.Content)
	}
	
	// Comprimir si es necesario
	if len(cm.Messages) > cm.CompressAfter {
		cm.compressIfNeeded()
	}
}

// GetRecentMessages obtiene los mensajes más recientes
func (cm *ConversationMemory) GetRecentMessages(count int) []llm.Message {
	if count <= 0 || len(cm.Messages) == 0 {
		return []llm.Message{}
	}
	
	start := len(cm.Messages) - count
	if start < 0 {
		start = 0
	}
	
	return cm.Messages[start:]
}

// GetContextualMessages obtiene mensajes relevantes para una query
func (cm *ConversationMemory) GetContextualMessages(query string, maxCount int) []llm.Message {
	if len(cm.Messages) == 0 {
		return []llm.Message{}
	}
	
	// Estrategia simple: últimos mensajes + búsqueda por palabras clave
	recentMessages := cm.GetRecentMessages(maxCount / 2)
	
	// Buscar mensajes relevantes por palabras clave
	queryWords := strings.Fields(strings.ToLower(query))
	relevantMessages := make([]llm.Message, 0)
	
	for i := len(cm.Messages) - 1; i >= 0 && len(relevantMessages) < maxCount/2; i-- {
		message := cm.Messages[i]
		content := strings.ToLower(message.Content)
		
		relevance := 0
		for _, word := range queryWords {
			if strings.Contains(content, word) {
				relevance++
			}
		}
		
		if relevance > 0 {
			relevantMessages = append([]llm.Message{message}, relevantMessages...)
		}
	}
	
	// Combinar y deduplicar
	combined := combineAndDeduplicate(recentMessages, relevantMessages)
	
	if len(combined) > maxCount {
		return combined[:maxCount]
	}
	
	return combined
}

// GetSummary devuelve un resumen de la conversación
func (cm *ConversationMemory) GetSummary() string {
	if cm.Summary != "" {
		return cm.Summary
	}
	
	if len(cm.Messages) == 0 {
		return "Nueva conversación sin historial."
	}
	
	// Generar resumen básico
	userMessages := 0
	assistantMessages := 0
	
	for _, msg := range cm.Messages {
		if msg.Role == "user" {
			userMessages++
		} else if msg.Role == "assistant" {
			assistantMessages++
		}
	}
	
	topics := strings.Join(cm.Topics, ", ")
	if topics == "" {
		topics = "conversación general"
	}
	
	return fmt.Sprintf("Conversación con %d intercambios sobre: %s", 
		userMessages, topics)
}

// AddKeyFact añade un hecho clave extraído de la conversación
func (cm *ConversationMemory) AddKeyFact(content, category, context string, relevance float64) {
	fact := KeyFact{
		ID:        generateFactID(content),
		Content:   content,
		Relevance: relevance,
		Timestamp: time.Now(),
		Context:   context,
		Category:  category,
	}
	
	cm.KeyFacts = append(cm.KeyFacts, fact)
	
	// Mantener solo los hechos más relevantes
	if len(cm.KeyFacts) > 50 {
		sort.Slice(cm.KeyFacts, func(i, j int) bool {
			return cm.KeyFacts[i].Relevance > cm.KeyFacts[j].Relevance
		})
		cm.KeyFacts = cm.KeyFacts[:40] // Mantener top 40
	}
}

// GetRelevantFacts obtiene hechos relevantes para una consulta
func (cm *ConversationMemory) GetRelevantFacts(query string, maxCount int) []KeyFact {
	if len(cm.KeyFacts) == 0 {
		return []KeyFact{}
	}
	
	queryWords := strings.Fields(strings.ToLower(query))
	
	type factScore struct {
		fact  KeyFact
		score float64
	}
	
	var scored []factScore
	
	for _, fact := range cm.KeyFacts {
		score := fact.Relevance
		content := strings.ToLower(fact.Content + " " + fact.Context)
		
		// Aumentar score por palabras clave
		for _, word := range queryWords {
			if strings.Contains(content, word) {
				score += 0.2
			}
		}
		
		scored = append(scored, factScore{fact, score})
	}
	
	// Ordenar por score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	
	// Devolver top facts
	result := make([]KeyFact, 0, maxCount)
	for i, s := range scored {
		if i >= maxCount {
			break
		}
		result = append(result, s.fact)
	}
	
	return result
}

// Save guarda la memoria en disco
func (cm *ConversationMemory) Save() error {
	if cm.StoragePath == "" {
		return fmt.Errorf("no storage path configured")
	}
	
	// Crear directorio si no existe
	if err := os.MkdirAll(cm.StoragePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	filename := filepath.Join(cm.StoragePath, fmt.Sprintf("session_%s.json", cm.SessionID))
	
	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal memory: %w", err)
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write memory file: %w", err)
	}
	
	return nil
}

// compressIfNeeded comprime la conversación si es muy larga
func (cm *ConversationMemory) compressIfNeeded() {
	if len(cm.Messages) <= cm.MaxMessages {
		return
	}
	
	// Mantener los últimos 30 mensajes
	keepRecent := 30
	if keepRecent > len(cm.Messages) {
		keepRecent = len(cm.Messages)
	}
	
	// Extraer información importante de mensajes antiguos
	oldMessages := cm.Messages[:len(cm.Messages)-keepRecent]
	cm.extractKeyInformation(oldMessages)
	
	// Mantener solo mensajes recientes
	cm.Messages = cm.Messages[len(cm.Messages)-keepRecent:]
	
	// Actualizar resumen
	cm.updateSummary()
}

// extractInfoFromUserMessage extrae información del mensaje del usuario
func (cm *ConversationMemory) extractInfoFromUserMessage(content string) {
	lower := strings.ToLower(content)
	
	// Detectar información del club
	if strings.Contains(lower, "club") || strings.Contains(lower, "instalaciones") {
		cm.Topics = addUniqueString(cm.Topics, "club_management")
	}
	
	if strings.Contains(lower, "precio") || strings.Contains(lower, "tarifa") {
		cm.Topics = addUniqueString(cm.Topics, "pricing")
	}
	
	if strings.Contains(lower, "socio") || strings.Contains(lower, "miembro") {
		cm.Topics = addUniqueString(cm.Topics, "membership")
	}
	
	// Detectar nombre del club
	patterns := []string{"mi club", "nuestro club", "el club"}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			// Intentar extraer nombre después del patrón
			if idx := strings.Index(lower, pattern); idx != -1 {
				after := content[idx+len(pattern):]
				words := strings.Fields(after)
				if len(words) > 0 && len(words[0]) > 3 {
					cm.UserProfile.ClubInfo.Name = strings.Title(words[0])
				}
			}
		}
	}
}

// extractKeyInformation extrae información clave de mensajes antiguos
func (cm *ConversationMemory) extractKeyInformation(messages []llm.Message) {
	for _, message := range messages {
		if message.Role == "user" {
			// Buscar información importante en mensajes del usuario
			if len(message.Content) > 100 {
				cm.AddKeyFact(
					message.Content[:100]+"...",
					"user_context",
					"conversación anterior",
					0.6,
				)
			}
		}
	}
}

// updateSummary actualiza el resumen de la conversación
func (cm *ConversationMemory) updateSummary() {
	topics := strings.Join(cm.Topics, ", ")
	if topics == "" {
		topics = "conversación general"
	}
	
	cm.Summary = fmt.Sprintf("Conversación sobre %s (comprimida)", topics)
}

// Funciones auxiliares

func generateSessionID() string {
	return fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
}

func generateFactID(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)[:8]
}

func addUniqueString(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

func combineAndDeduplicate(slice1, slice2 []llm.Message) []llm.Message {
	seen := make(map[string]bool)
	var result []llm.Message
	
	for _, msg := range slice1 {
		key := msg.Role + ":" + msg.Content
		if !seen[key] {
			result = append(result, msg)
			seen[key] = true
		}
	}
	
	for _, msg := range slice2 {
		key := msg.Role + ":" + msg.Content
		if !seen[key] {
			result = append(result, msg)
			seen[key] = true
		}
	}
	
	return result
}