package memory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
	"github.com/santiagocorredoira/agent/agent/prompts"
)

// ConversationMemory gestiona la memoria conversacional del agente
type ConversationMemory struct {
	SessionID       string                `json:"session_id"`
	StartTime       time.Time             `json:"start_time"`
	LastAccess      time.Time             `json:"last_access"`
	Messages        []llm.Message         `json:"messages"`
	Summary         string                `json:"summary"`
	Topics          []string              `json:"topics"`
	KeyFacts        []KeyFact             `json:"key_facts"`
	UserProfile     UserProfile           `json:"user_profile"`
	MaxMessages     int                   `json:"max_messages"`
	CompressAfter   int                   `json:"compress_after"`
	StoragePath     string                `json:"-"`
	SemanticMemory  *SemanticMemory       `json:"semantic_memory"`  // Enhanced semantic memory
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
		MaxMessages:    100,  // Máximo de mensajes antes de comprimir
		CompressAfter:  50,   // Comprimir después de este número
		StoragePath:    storagePath,
		SemanticMemory: NewSemanticMemory(), // Initialize semantic memory
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
	
	// Initialize semantic memory if it doesn't exist
	if memory.SemanticMemory == nil {
		memory.SemanticMemory = NewSemanticMemory()
	}
	
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
		cm.extractSemanticInformation(message.Content, "user_input")
	} else if message.Role == "assistant" {
		cm.extractSemanticInformation(message.Content, "assistant_response")
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

// GetContextualMessages obtiene mensajes relevantes para una query usando memoria semántica
func (cm *ConversationMemory) GetContextualMessages(query string, maxCount int) []llm.Message {
	if len(cm.Messages) == 0 {
		return []llm.Message{}
	}
	
	// Get recent messages (always include some recent context)
	recentCount := maxCount / 3
	recentMessages := cm.GetRecentMessages(recentCount)
	
	// Get semantically relevant facts
	relevantFacts := cm.SemanticMemory.GetRelevantFacts(query, 10)
	
	// Convert semantic facts back to relevant messages by content matching
	factBasedMessages := make([]llm.Message, 0)
	for _, fact := range relevantFacts {
		// Find messages that contain this fact's content
		for _, message := range cm.Messages {
			if strings.Contains(strings.ToLower(message.Content), strings.ToLower(fact.Content[:min(len(fact.Content), 50)])) {
				factBasedMessages = append(factBasedMessages, message)
				if len(factBasedMessages) >= maxCount/2 {
					break
				}
			}
		}
		if len(factBasedMessages) >= maxCount/2 {
			break
		}
	}
	
	// Fallback to keyword-based search if semantic search didn't yield enough results
	if len(factBasedMessages) < maxCount/4 {
		queryWords := strings.Fields(strings.ToLower(query))
		keywordMessages := make([]llm.Message, 0)
		
		for i := len(cm.Messages) - 1; i >= 0 && len(keywordMessages) < maxCount/2; i-- {
			message := cm.Messages[i]
			content := strings.ToLower(message.Content)
			
			relevance := 0
			for _, word := range queryWords {
				if strings.Contains(content, word) {
					relevance++
				}
			}
			
			if relevance > 0 {
				keywordMessages = append(keywordMessages, message)
			}
		}
		
		factBasedMessages = append(factBasedMessages, keywordMessages...)
	}
	
	// Combine and deduplicate all message sources
	combined := combineAndDeduplicate(recentMessages, factBasedMessages)
	
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
	
	// Fallback to basic summary if AI generation is not available
	cm.Summary = fmt.Sprintf("Conversación sobre %s (comprimida)", topics)
}

// GenerateAISummary generates an AI-powered summary of the conversation
func (cm *ConversationMemory) GenerateAISummary(ctx context.Context, llmProvider llm.Provider) error {
	if len(cm.Messages) < 2 {
		return nil // No need to summarize very short conversations
	}

	// Format messages for the prompt
	var messageTexts []string
	for _, msg := range cm.Messages {
		if msg.Role == "system" {
			continue // Skip system messages in summary
		}
		roleLabel := "Usuario"
		if msg.Role == "assistant" {
			roleLabel = "Asistente"
		}
		messageTexts = append(messageTexts, fmt.Sprintf("%s: %s", roleLabel, msg.Content))
	}

	if len(messageTexts) == 0 {
		return nil
	}

	// Limit messages to avoid token limits (last 10 exchanges)
	if len(messageTexts) > 20 {
		messageTexts = messageTexts[len(messageTexts)-20:]
	}

	messagesText := strings.Join(messageTexts, "\n\n")

	// Create summary prompt
	summaryPrompt := prompts.RenderConversationSummaryPrompt(prompts.PromptData{
		Messages: messagesText,
	})

	// Create completion request
	req := &llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "user", Content: summaryPrompt},
		},
		MaxTokens:   150, // Keep summary concise
		Temperature: 0.3, // Lower temperature for consistent summaries
	}

	// Get AI-generated summary
	resp, err := llmProvider.Complete(ctx, req)
	if err != nil {
		// If AI summary fails, keep the basic summary
		return fmt.Errorf("failed to generate AI summary: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary != "" && len(summary) > 10 { // Sanity check
		cm.Summary = summary
	}

	return nil
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

// extractSemanticInformation extracts semantic information from message content
func (cm *ConversationMemory) extractSemanticInformation(content, category string) {
	if cm.SemanticMemory == nil {
		return
	}
	
	// Simple entity and keyword extraction
	entities := cm.extractEntities(content)
	keywords := cm.extractKeywordsFromContent(content)
	
	// Calculate importance based on content length and category
	importance := 0.5 // Base importance
	if category == "user_input" {
		importance = 0.8 // User input is generally more important
	}
	if len(content) > 200 {
		importance += 0.2 // Longer content might be more important
	}
	
	// Create context map
	context := map[string]string{
		"session_id": cm.SessionID,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	
	// Add to semantic memory
	cm.SemanticMemory.AddSemanticFact(content, category, entities, keywords, importance, context)
}

// extractEntities performs simple entity extraction from text
func (cm *ConversationMemory) extractEntities(content string) []string {
	entities := make([]string, 0)
	lower := strings.ToLower(content)
	
	// Simple pattern-based entity extraction
	entityPatterns := map[string][]string{
		"club":     {"club", "centro", "instalación"},
		"price":    {"precio", "tarifa", "coste", "€", "$"},
		"date":     {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		"service":  {"reserva", "booking", "clase", "entrenamiento"},
		"person":   {"socio", "miembro", "cliente", "usuario"},
	}
	
	for entityType, patterns := range entityPatterns {
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				entities = addUniqueString(entities, entityType)
				break
			}
		}
	}
	
	return entities
}

// extractKeywordsFromContent extracts keywords from content using simple heuristics
func (cm *ConversationMemory) extractKeywordsFromContent(content string) []string {
	words := strings.Fields(strings.ToLower(content))
	keywords := make([]string, 0)
	
	// Filter out common stop words and short words
	stopWords := map[string]bool{
		"el": true, "la": true, "de": true, "que": true, "y": true, "a": true,
		"en": true, "un": true, "es": true, "se": true, "no": true, "te": true,
		"lo": true, "le": true, "da": true, "su": true, "por": true, "son": true,
		"con": true, "para": true, "al": true, "del": true, "las": true, "los": true,
		"the": true, "and": true, "or": true, "but": true, "in": true, "on": true,
		"at": true, "to": true, "for": true, "of": true, "with": true, "by": true,
	}
	
	for _, word := range words {
		// Clean punctuation
		word = strings.Trim(word, ".,!?;:()[]{}\"'")
		
		// Keep words that are longer than 2 characters and not stop words
		if len(word) > 2 && !stopWords[word] {
			keywords = addUniqueString(keywords, word)
		}
	}
	
	// Limit keywords to most relevant ones
	if len(keywords) > 10 {
		keywords = keywords[:10]
	}
	
	return keywords
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}