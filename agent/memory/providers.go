package memory

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// SystemInfoProvider proporciona información del sistema
type SystemInfoProvider struct {
	enabled     bool
	priority    int
	keywords    []string
	includeTime bool
	includeOS   bool
	version     string
}

// NewSystemInfoProvider crea un proveedor de información del sistema
func NewSystemInfoProvider(config map[string]interface{}) *SystemInfoProvider {
	provider := &SystemInfoProvider{
		enabled:     true,
		priority:    100, // Alta prioridad
		includeTime: true,
		includeOS:   true,
		version:     "1.0.0",
		keywords: []string{
			"fecha", "date", "dia", "day", "hoy", "today", "ahora", "now",
			"cuando", "when", "hora", "time", "horario", "schedule",
			"sistema", "system", "version", "plataforma", "platform",
		},
	}

	// Aplicar configuración
	if config != nil {
		if enabled, ok := config["enabled"].(bool); ok {
			provider.enabled = enabled
		}
		if priority, ok := config["priority"].(int); ok {
			provider.priority = priority
		}
		if keywords, ok := config["keywords"].([]string); ok {
			provider.keywords = keywords
		}
		if includeTime, ok := config["include_time"].(bool); ok {
			provider.includeTime = includeTime
		}
		if includeOS, ok := config["include_os"].(bool); ok {
			provider.includeOS = includeOS
		}
		if version, ok := config["version"].(string); ok {
			provider.version = version
		}
	}

	return provider
}

func (s *SystemInfoProvider) GetName() string {
	return "system_info"
}

func (s *SystemInfoProvider) GetDescription() string {
	return "Provides current date/time and system information when relevant"
}

func (s *SystemInfoProvider) ShouldActivate(query string, session *ConversationMemory) bool {
	if !s.enabled {
		return false
	}

	queryLower := strings.ToLower(query)
	for _, keyword := range s.keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

func (s *SystemInfoProvider) GetContext(query string, session *ConversationMemory) []llm.Message {
	var contextParts []string

	if s.includeTime {
		now := time.Now()
		contextParts = append(contextParts, fmt.Sprintf("Current date and time: %s (%s)",
			now.Format("Monday, January 2, 2006 at 3:04 PM"),
			now.Format("MST")))
	}

	if s.includeOS {
		contextParts = append(contextParts, fmt.Sprintf("System: %s %s",
			runtime.GOOS, runtime.GOARCH))
	}

	if s.version != "" {
		contextParts = append(contextParts, fmt.Sprintf("Agent version: %s", s.version))
	}

	if len(contextParts) == 0 {
		return []llm.Message{}
	}

	context := strings.Join(contextParts, "\n")
	return []llm.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("SYSTEM CONTEXT:\n%s", context),
		},
	}
}

func (s *SystemInfoProvider) GetPriority() int {
	return s.priority
}

func (s *SystemInfoProvider) IsEnabled() bool {
	return s.enabled
}

// BusinessContextProvider proporciona contexto específico del dominio de negocio
type BusinessContextProvider struct {
	enabled     bool
	priority    int
	domain      string
	description string
	keywords    []string
	context     string
}

// NewBusinessContextProvider crea un proveedor de contexto de negocio
func NewBusinessContextProvider(config map[string]interface{}) *BusinessContextProvider {
	provider := &BusinessContextProvider{
		enabled:  true,
		priority: 90,
		domain:   "general",
	}

	// Aplicar configuración
	if config != nil {
		if enabled, ok := config["enabled"].(bool); ok {
			provider.enabled = enabled
		}
		if priority, ok := config["priority"].(int); ok {
			provider.priority = priority
		}
		if domain, ok := config["domain"].(string); ok {
			provider.domain = domain
		}
		if description, ok := config["description"].(string); ok {
			provider.description = description
		}
		if keywords, ok := config["keywords"].([]string); ok {
			provider.keywords = keywords
		}
		if context, ok := config["context"].(string); ok {
			provider.context = context
		}
	}

	return provider
}

func (b *BusinessContextProvider) GetName() string {
	return fmt.Sprintf("business_%s", b.domain)
}

func (b *BusinessContextProvider) GetDescription() string {
	if b.description != "" {
		return b.description
	}
	return fmt.Sprintf("Provides business context for %s domain", b.domain)
}

func (b *BusinessContextProvider) ShouldActivate(query string, session *ConversationMemory) bool {
	if !b.enabled || len(b.keywords) == 0 {
		return false
	}

	queryLower := strings.ToLower(query)
	for _, keyword := range b.keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

func (b *BusinessContextProvider) GetContext(query string, session *ConversationMemory) []llm.Message {
	if b.context == "" {
		return []llm.Message{}
	}

	return []llm.Message{
		{
			Role:    "system",
			Content: b.context,
		},
	}
}

func (b *BusinessContextProvider) GetPriority() int {
	return b.priority
}

func (b *BusinessContextProvider) IsEnabled() bool {
	return b.enabled
}

// SessionContextProvider proporciona contexto de la sesión actual
type SessionContextProvider struct {
	enabled  bool
	priority int
}

// NewSessionContextProvider crea un proveedor de contexto de sesión
func NewSessionContextProvider(config map[string]interface{}) *SessionContextProvider {
	provider := &SessionContextProvider{
		enabled:  true,
		priority: 80,
	}

	if config != nil {
		if enabled, ok := config["enabled"].(bool); ok {
			provider.enabled = enabled
		}
		if priority, ok := config["priority"].(int); ok {
			provider.priority = priority
		}
	}

	return provider
}

func (s *SessionContextProvider) GetName() string {
	return "session_context"
}

func (s *SessionContextProvider) GetDescription() string {
	return "Provides context about the current conversation session"
}

func (s *SessionContextProvider) ShouldActivate(query string, session *ConversationMemory) bool {
	if !s.enabled || session == nil {
		return false
	}

	// Activar si hay información relevante en la sesión
	return len(session.Topics) > 0 || 
		   len(session.KeyFacts) > 0 || 
		   session.UserProfile.Name != ""
}

func (s *SessionContextProvider) GetContext(query string, session *ConversationMemory) []llm.Message {
	if session == nil {
		return []llm.Message{}
	}

	var contextParts []string

	if session.UserProfile.Name != "" {
		contextParts = append(contextParts, fmt.Sprintf("User: %s", session.UserProfile.Name))
	}

	if len(session.Topics) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("Conversation topics: %s", 
			strings.Join(session.Topics, ", ")))
	}

	if len(session.KeyFacts) > 0 && len(session.KeyFacts) <= 3 {
		facts := make([]string, len(session.KeyFacts))
		for i, fact := range session.KeyFacts {
			facts[i] = fact.Content
		}
		contextParts = append(contextParts, fmt.Sprintf("Key facts: %s", 
			strings.Join(facts, "; ")))
	}

	if len(contextParts) == 0 {
		return []llm.Message{}
	}

	context := strings.Join(contextParts, "\n")
	return []llm.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("SESSION CONTEXT:\n%s", context),
		},
	}
}

func (s *SessionContextProvider) GetPriority() int {
	return s.priority
}

func (s *SessionContextProvider) IsEnabled() bool {
	return s.enabled
}

// ConfigurableContextProvider proveedor completamente configurable desde JSON
type ConfigurableContextProvider struct {
	name        string
	description string
	enabled     bool
	priority    int
	keywords    []string
	context     string
	activation  string // "always", "keywords", "session_based"
}

// NewConfigurableContextProvider crea un proveedor configurable
func NewConfigurableContextProvider(config map[string]interface{}) *ConfigurableContextProvider {
	provider := &ConfigurableContextProvider{
		name:       "configurable",
		enabled:    true,
		priority:   50,
		activation: "keywords",
	}

	if config != nil {
		if name, ok := config["name"].(string); ok {
			provider.name = name
		}
		if description, ok := config["description"].(string); ok {
			provider.description = description
		}
		if enabled, ok := config["enabled"].(bool); ok {
			provider.enabled = enabled
		}
		if priority, ok := config["priority"].(int); ok {
			provider.priority = priority
		}
		if keywords, ok := config["keywords"].([]string); ok {
			provider.keywords = keywords
		}
		if context, ok := config["context"].(string); ok {
			provider.context = context
		}
		if activation, ok := config["activation"].(string); ok {
			provider.activation = activation
		}
	}

	return provider
}

func (c *ConfigurableContextProvider) GetName() string {
	return c.name
}

func (c *ConfigurableContextProvider) GetDescription() string {
	return c.description
}

func (c *ConfigurableContextProvider) ShouldActivate(query string, session *ConversationMemory) bool {
	if !c.enabled {
		return false
	}

	switch c.activation {
	case "always":
		return true
	case "keywords":
		if len(c.keywords) == 0 {
			return false
		}
		queryLower := strings.ToLower(query)
		for _, keyword := range c.keywords {
			if strings.Contains(queryLower, keyword) {
				return true
			}
		}
		return false
	case "session_based":
		return session != nil && len(session.Messages) > 0
	default:
		return false
	}
}

func (c *ConfigurableContextProvider) GetContext(query string, session *ConversationMemory) []llm.Message {
	if c.context == "" {
		return []llm.Message{}
	}

	return []llm.Message{
		{
			Role:    "system",
			Content: c.context,
		},
	}
}

func (c *ConfigurableContextProvider) GetPriority() int {
	return c.priority
}

func (c *ConfigurableContextProvider) IsEnabled() bool {
	return c.enabled
}