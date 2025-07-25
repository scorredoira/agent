package memory

import (
	"github.com/santiagocorredoira/agent/agent/llm"
)

// ContextProvider define la interfaz para proveedores de contexto
type ContextProvider interface {
	// GetName devuelve el nombre único del proveedor
	GetName() string
	
	// GetDescription devuelve una descripción del proveedor
	GetDescription() string
	
	// ShouldActivate determina si este proveedor debe activarse para una query
	ShouldActivate(query string, session *ConversationMemory) bool
	
	// GetContext devuelve el contexto a inyectar
	GetContext(query string, session *ConversationMemory) []llm.Message
	
	// GetPriority devuelve la prioridad del proveedor (mayor = más prioritario)
	GetPriority() int
	
	// IsEnabled verifica si el proveedor está habilitado
	IsEnabled() bool
}

// ContextManager gestiona múltiples proveedores de contexto
type ContextManager struct {
	providers []ContextProvider
	enabled   bool
}

// NewContextManager crea un nuevo gestor de contexto
func NewContextManager() *ContextManager {
	return &ContextManager{
		providers: make([]ContextProvider, 0),
		enabled:   true,
	}
}

// RegisterProvider registra un nuevo proveedor de contexto
func (cm *ContextManager) RegisterProvider(provider ContextProvider) {
	if provider == nil {
		return
	}
	
	// Insertar en orden de prioridad
	inserted := false
	for i, p := range cm.providers {
		if provider.GetPriority() > p.GetPriority() {
			cm.providers = append(cm.providers[:i], append([]ContextProvider{provider}, cm.providers[i:]...)...)
			inserted = true
			break
		}
	}
	
	if !inserted {
		cm.providers = append(cm.providers, provider)
	}
}

// GetContext obtiene contexto de todos los proveedores aplicables
func (cm *ContextManager) GetContext(query string, session *ConversationMemory) []llm.Message {
	if !cm.enabled || session == nil {
		return []llm.Message{}
	}
	
	var contextMessages []llm.Message
	
	for _, provider := range cm.providers {
		if !provider.IsEnabled() {
			continue
		}
		
		if provider.ShouldActivate(query, session) {
			providerContext := provider.GetContext(query, session)
			contextMessages = append(contextMessages, providerContext...)
		}
	}
	
	return contextMessages
}

// ListProviders devuelve información de todos los proveedores
func (cm *ContextManager) ListProviders() []ProviderInfo {
	var info []ProviderInfo
	
	for _, provider := range cm.providers {
		info = append(info, ProviderInfo{
			Name:        provider.GetName(),
			Description: provider.GetDescription(),
			Priority:    provider.GetPriority(),
			Enabled:     provider.IsEnabled(),
		})
	}
	
	return info
}

// ProviderInfo información sobre un proveedor
type ProviderInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
}

// SetEnabled habilita/deshabilita el gestor de contexto
func (cm *ContextManager) SetEnabled(enabled bool) {
	cm.enabled = enabled
}

// IsEnabled verifica si el gestor está habilitado
func (cm *ContextManager) IsEnabled() bool {
	return cm.enabled
}

// GetProviderByName busca un proveedor por nombre
func (cm *ContextManager) GetProviderByName(name string) ContextProvider {
	for _, provider := range cm.providers {
		if provider.GetName() == name {
			return provider
		}
	}
	return nil
}

// RemoveProvider remueve un proveedor por nombre
func (cm *ContextManager) RemoveProvider(name string) bool {
	for i, provider := range cm.providers {
		if provider.GetName() == name {
			cm.providers = append(cm.providers[:i], cm.providers[i+1:]...)
			return true
		}
	}
	return false
}